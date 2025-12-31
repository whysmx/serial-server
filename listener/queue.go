// Package listener implements the serial server listener with queue and cache.
package listener

import (
	"serial-server/serial"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultCacheTTL  = 5 * time.Second
	requestTimeout   = 3 * time.Second
	respFlushTimeout = 50 * time.Millisecond // 50ms内数据合并显示
)

// cacheEntry represents a cached response with expiration time.
type cacheEntry struct {
	data     []byte
	expireAt time.Time
}

// RequestCache handles caching of request-response pairs with dynamic TTL.
type RequestCache struct {
	cache map[uint64]*cacheEntry
	mu    sync.RWMutex
}

// NewRequestCache creates a new request cache.
func NewRequestCache() *RequestCache {
	return &RequestCache{
		cache: make(map[uint64]*cacheEntry),
	}
}

// Get retrieves a cached response (expired entries are skipped).
func (c *RequestCache) Get(hash uint64) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.cache[hash]
	if !found {
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.expireAt) {
		// Entry expired (cleanup happens in Set or background)
		return nil, false
	}

	return entry.data, true
}

// Set stores a response in cache with default TTL.
func (c *RequestCache) Set(hash uint64, data []byte) {
	c.SetWithTTL(hash, data, defaultCacheTTL)
}

// SetWithTTL stores a response in cache with custom TTL.
func (c *RequestCache) SetWithTTL(hash uint64, data []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[hash] = &cacheEntry{
		data:     data,
		expireAt: time.Now().Add(ttl),
	}
}

// CleanupExpired removes all expired entries from cache.
func (c *RequestCache) CleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for hash, entry := range c.cache {
		if now.After(entry.expireAt) {
			delete(c.cache, hash)
		}
	}
}

// PendingRequest represents a request waiting for serial response.
type PendingRequest struct {
	ID         uint64        // Unique identifier for response matching
	ClientID   string
	DataHash   uint64
	Request    []byte
	ResponseCh chan []byte
	Timestamp  time.Time // Time when request was enqueued
	SentAt     time.Time // Time when request was actually sent to serial
	done       atomic.Bool
}

// finishWithResponse sends response and closes channel (only once).
func (r *PendingRequest) finishWithResponse(data []byte) {
	if r.done.Swap(true) {
		return
	}
	r.ResponseCh <- data
	close(r.ResponseCh)
}

// finishNoResponse closes channel without sending data (only once).
func (r *PendingRequest) finishNoResponse() {
	if r.done.Swap(true) {
		return
	}
	close(r.ResponseCh)
}

// Response state for managing send/receive lifecycle
const (
	respStateIdle    int32 = 0
	respStateSending int32 = 1 // Currently sending to serial
	respStateWaiting int32 = 2 // Waiting for response
)

// WriteQueue serializes writes to serial port and matches responses.
type WriteQueue struct {
	cache   *RequestCache
	pending []*PendingRequest
	mu      sync.Mutex
	serial  *serial.Port

	// ID generator for request matching
	nextReqID atomic.Uint64

	// Index to quickly find pending request by clientID
	clientIndex map[string]int

	// Inflight requests by data hash (main request currently being processed)
	inflight map[uint64]*PendingRequest

	// Waiting requests by data hash (for multi-client same-request handling)
	waiting map[uint64][]*PendingRequest

	// Response accumulation buffer
	respBuf      []byte
	respTimer    *time.Timer
	currentReqID uint64       // ID of request currently waiting for response
	respState    atomic.Int32 // State machine: idle -> sending -> waiting
	dropUntil    time.Time    // Drop responses received before this time (for late response window)

	// Flush loop control
	stopFlushLoop     chan struct{}
	stopFlushLoopOnce sync.Once

	// Cleanup timer control
	stopCleanup     chan struct{}
	stopCleanupOnce sync.Once
}

// NewWriteQueue creates a new write queue.
func NewWriteQueue(sp *serial.Port) *WriteQueue {
	return &WriteQueue{
		cache:         NewRequestCache(),
		pending:       make([]*PendingRequest, 0),
		serial:        sp,
		clientIndex:   make(map[string]int),
		inflight:      make(map[uint64]*PendingRequest),
		waiting:       make(map[uint64][]*PendingRequest),
		stopFlushLoop: make(chan struct{}), // Unbuffered, closed once via sync.Once
		stopCleanup:   make(chan struct{}),
	}
}

// hashData computes FNV-1a 64-bit hash.
func hashData(data []byte) uint64 {
	const (
		offset64 uint64 = 14695981039346656037
		prime64  uint64 = 1099511628211
	)

	hash := offset64
	for _, b := range data {
		hash ^= uint64(b)
		hash *= prime64
	}
	return hash
}

// Send enqueues a client request and returns response channel.
func (q *WriteQueue) Send(clientID string, data []byte) <-chan []byte {
	respCh := make(chan []byte, 1)

	hash := hashData(data)

	// Check cache first
	if cached, found := q.cache.Get(hash); found {
		respCh <- cached
		close(respCh)
		return respCh
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Double check cache after acquiring lock
	if cached, found := q.cache.Get(hash); found {
		respCh <- cached
		close(respCh)
		return respCh
	}

	// Check if there's already an inflight request with the same hash
	if _, found := q.inflight[hash]; found {
		// Same request is being processed, add to waiting list
		req := &PendingRequest{
			ClientID:   clientID,
			DataHash:   hash,
			Request:    data,
			ResponseCh: respCh,
			Timestamp:  time.Now(),
		}
		q.waiting[hash] = append(q.waiting[hash], req)
		q.clientIndex[clientID] = -1 // Mark as waiting
		return respCh
	}

	// Create new pending request with unique ID
	req := &PendingRequest{
		ID:         q.nextReqID.Add(1),
		ClientID:   clientID,
		DataHash:   hash,
		Request:    data,
		ResponseCh: respCh,
		Timestamp:  time.Now(),
	}

	// Add to inflight map (so subsequent same requests can find it)
	q.inflight[hash] = req

	// Append to queue
	q.pending = append(q.pending, req)
	q.clientIndex[clientID] = len(q.pending) - 1

	// If this is the only request, send immediately
	if len(q.pending) == 1 {
		go q.sendToSerial(req)
	}

	return respCh
}

// sendToSerial sends data to serial port.
// CRITICAL: Must verify request is still at head before sending.
// SentAt is set AFTER successful write to avoid premature timeout.
func (q *WriteQueue) sendToSerial(req *PendingRequest) {
	if q.serial == nil {
		return
	}

	// Step 1: Verify request is still at head AND state is idle
	// This prevents sending if timeout cleanup already processed it
	q.mu.Lock()
	if len(q.pending) == 0 || q.pending[0] != req {
		q.mu.Unlock()
		return
	}
	if q.respState.Load() != respStateIdle {
		// State not idle (e.g., cleanup already processed), skip
		q.mu.Unlock()
		return
	}

	// Mark as sending (no response expected yet)
	q.respState.Store(respStateSending)
	q.mu.Unlock()

	// Step 2: Write to serial port (without lock)
	_, err := q.serial.Write(req.Request)

	// Step 3: Handle write result with lock
	q.mu.Lock()

	// Re-verify request is still at head (could have been removed during write)
	if len(q.pending) == 0 || q.pending[0] != req {
		q.mu.Unlock()
		return
	}

	if err != nil {
		// Write failed: drop this request immediately
		hash := req.DataHash
		reqID := req.ID
		clientID := req.ClientID

		// Remove from pending and indexes
		q.pending = q.pending[1:]
		delete(q.clientIndex, req.ClientID)
		delete(q.inflight, hash)

		// Get and remove waiting list
		waitingList := q.waiting[hash]
		delete(q.waiting, hash)
		for _, w := range waitingList {
			delete(q.clientIndex, w.ClientID)
		}

		// Reset state and set drop window for late responses
		q.currentReqID = 0
		q.respState.Store(respStateIdle)
		q.dropUntil = time.Now().Add(150 * time.Millisecond)

		// Capture next request after removal
		var nextReq *PendingRequest
		if len(q.pending) > 0 {
			nextReq = q.pending[0]
		}

		q.mu.Unlock()

		logIssuef("serial write failed: req_id=%d client=%s hash=%d err=%v", reqID, clientID, hash, err)

		// Finish all requests without response
		req.finishNoResponse()
		for _, w := range waitingList {
			w.finishNoResponse()
		}

		// Trigger next request if any
		if nextReq != nil {
			go q.sendToSerial(nextReq)
		}
		return
	}

	// Write successful: set SentAt and transition to waiting for response
	req.SentAt = time.Now()
	q.currentReqID = req.ID
	q.respState.Store(respStateWaiting)
	q.mu.Unlock()
}

// OnSerialData handles data received from serial port.
// Data is accumulated and flushed after 50ms of inactivity.
func (q *WriteQueue) OnSerialData(data []byte) {
	// Quick check: must be in waiting state (not sending or idle)
	state := q.respState.Load()
	if state != respStateWaiting {
		logIssuefThrottled("drop_state", time.Second, "drop rx: state=%d bytes=%d", state, len(data))
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Must have pending request
	if len(q.pending) == 0 {
		logIssuefThrottled("drop_no_pending", time.Second, "drop rx: no pending bytes=%d", len(data))
		return
	}

	// Must be the current request (not a late response after timeout/next send)
	req := q.pending[0]
	if req.ID != q.currentReqID {
		// Not current request - check if we're in the "late response" window
		// Only drop if we're between requests (currentReqID is stale)
		if time.Now().Before(q.dropUntil) {
			logIssuefThrottled("drop_until", time.Second, "drop rx: drop_until=%s bytes=%d", q.dropUntil.Format(time.RFC3339Nano), len(data))
			return
		}
		logIssuefThrottled("drop_id_mismatch", time.Second, "drop rx: current_id=%d pending_id=%d bytes=%d", q.currentReqID, req.ID, len(data))
		return
	}

	// Must have been sent (SentAt is set)
	if req.SentAt.IsZero() {
		logIssuefThrottled("drop_unsent", time.Second, "drop rx: req_id=%d bytes=%d", req.ID, len(data))
		return
	}

	// Accumulate data
	q.respBuf = append(q.respBuf, data...)

	// Reset or create flush timer
	if q.respTimer != nil {
		// Stop() returns false if timer already fired, drain the channel
		// to prevent the old callback from firing after reset
		if !q.respTimer.Stop() {
			select {
			case <-q.respTimer.C: // Drain fired timer
			default:
			}
		}
		q.respTimer.Reset(respFlushTimeout)
	} else {
		// Start a new flush loop
		q.respTimer = time.NewTimer(respFlushTimeout)
		go q.flushResponseLoop()
	}
}

// flushResponseLoop listens for flush signals and processes responses.
// Runs in a separate goroutine started by OnSerialData.
func (q *WriteQueue) flushResponseLoop() {
	for {
		select {
		case <-q.stopFlushLoop:
			return
		case <-q.stopCleanup:
			return
		case <-func() <-chan time.Time {
			q.mu.Lock()
			timer := q.respTimer
			q.mu.Unlock()
			if timer == nil {
				// Timer was cleaned up, exit
				return nil
			}
			return timer.C
		}():
			// Timer fired: lock and check if there's data to flush.
			q.mu.Lock()
			if len(q.pending) > 0 && len(q.respBuf) > 0 {
				q.flushResponseLocked()
			} else {
				// No data to flush (likely a stale C from old timer)
				q.respBuf = nil
				q.mu.Unlock()
			}
		}
	}
}

// flushResponseLocked processes the accumulated response data (must hold q.mu).
func (q *WriteQueue) flushResponseLocked() {
	// No pending request or empty buffer
	if len(q.pending) == 0 || len(q.respBuf) == 0 {
		q.respBuf = nil
		return
	}

	// Get first request
	req := q.pending[0]

	// Calculate RTT: from request send to complete response (timer fires)
	// Use SentAt if available, otherwise fall back to Timestamp
	sendTime := req.SentAt
	if sendTime.IsZero() {
		sendTime = req.Timestamp
	}
	rtt := time.Now().Sub(sendTime)
	if rtt < 0 {
		rtt = 0
	}

	// Calculate cache TTL: RTT * 2 (min 1s, max 30s)
	cacheTTL := rtt * 2
	if cacheTTL < time.Second {
		cacheTTL = time.Second
	} else if cacheTTL > 30*time.Second {
		cacheTTL = 30 * time.Second
	}

	// Store in cache with dynamic TTL
	q.cache.SetWithTTL(req.DataHash, q.respBuf, cacheTTL)

	// Get all waiting requests with the same hash
	hash := req.DataHash
	waitingList := q.waiting[hash]
	delete(q.waiting, hash)

	// Remove main request and all waiting clients from index
	delete(q.clientIndex, req.ClientID)
	for _, w := range waitingList {
		delete(q.clientIndex, w.ClientID)
	}

	// Remove from inflight map (so new requests can be enqueued)
	delete(q.inflight, hash)

	// Remove from queue and reset state
	// State will be set by sendToSerial when next request is actually sent
	q.pending = q.pending[1:]
	q.currentReqID = 0
	q.respState.Store(respStateIdle)

	// Clear response buffer
	responseData := q.respBuf
	q.respBuf = nil

	// Capture next request head (if any) after removing current
	var nextReq *PendingRequest
	if len(q.pending) > 0 {
		nextReq = q.pending[0]
	}

	// Finish main request and all waiting requests (unlock first)
	q.mu.Unlock()
	req.finishWithResponse(responseData)
	for _, w := range waitingList {
		w.finishWithResponse(responseData)
	}

	// Process next request if any
	if nextReq != nil {
		go q.sendToSerial(nextReq)
	}
}

// CleanupExpired removes timed-out requests and expired cache entries.
func (q *WriteQueue) CleanupExpired() {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Clean up expired cache entries
	q.cache.CleanupExpired()

	now := time.Now()
	active := make([]*PendingRequest, 0)
	newIndex := make(map[string]int)
	expired := make([]*PendingRequest, 0)
	firstWasExpired := false

	for i, req := range q.pending {
		// Calculate timeout from send time if sent, otherwise from enqueue time
		timeoutBase := req.SentAt
		if timeoutBase.IsZero() {
			timeoutBase = req.Timestamp
		}
		if now.Sub(timeoutBase) < requestTimeout {
			active = append(active, req)
			newIndex[req.ClientID] = len(active) - 1
		} else {
			// Timeout: mark as expired and remove
			expired = append(expired, req)
			delete(q.clientIndex, req.ClientID)
			delete(q.inflight, req.DataHash) // Remove from inflight so new requests can be enqueued
			if waitingList, found := q.waiting[req.DataHash]; found {
				delete(q.waiting, req.DataHash)
				for _, w := range waitingList {
					expired = append(expired, w)
					delete(q.clientIndex, w.ClientID)
				}
			}
			if i == 0 {
				firstWasExpired = true
			}
		}
	}

	q.pending = active
	q.clientIndex = newIndex

	// Clean up expired waiting requests
	for hash, waitingList := range q.waiting {
		activeWaiting := make([]*PendingRequest, 0)
		for _, req := range waitingList {
			if now.Sub(req.Timestamp) < requestTimeout {
				activeWaiting = append(activeWaiting, req)
			} else {
				// Timeout: mark as expired
				expired = append(expired, req)
				delete(q.clientIndex, req.ClientID)
			}
		}
		if len(activeWaiting) > 0 {
			q.waiting[hash] = activeWaiting
		} else {
			delete(q.waiting, hash)
		}
	}

	// If first request was expired, reset state and set drop window for late responses
	// Note: We don't stop the timer here to avoid goroutine leak.
	// The old loop will eventually fire, see empty respBuf, and return.
	if firstWasExpired {
		q.respBuf = nil
		q.currentReqID = 0
		q.respState.Store(respStateIdle)
		// Set drop window: discard responses received within 150ms after timeout
		// This absorbs late responses from the timed-out request
		q.dropUntil = now.Add(150 * time.Millisecond)
	}

	// Finish all expired requests (unlock first)
	q.mu.Unlock()
	for _, req := range expired {
		logIssuef("request timeout: req_id=%d client=%s hash=%d sent_at=%v queued_at=%v", req.ID, req.ClientID, req.DataHash, req.SentAt, req.Timestamp)
		req.finishNoResponse()
	}
	q.mu.Lock()

	// If first request was expired and we have pending requests, trigger next send
	if firstWasExpired && len(q.pending) > 0 {
		nextReq := q.pending[0]
		go q.sendToSerial(nextReq)
	}
}

// StartCleanupTimer starts periodic cleanup of expired requests.
func (q *WriteQueue) StartCleanupTimer() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				q.CleanupExpired()
			case <-q.stopCleanup:
				return
			}
		}
	}()
}

// StopCleanupTimer stops the cleanup timer and cleans up resources.
func (q *WriteQueue) StopCleanupTimer() {
	q.stopCleanupOnce.Do(func() {
		close(q.stopCleanup)
	})

	// Signal flush loop to stop by closing the channel (idempotent)
	q.stopFlushLoopOnce.Do(func() {
		close(q.stopFlushLoop)
	})

	// Clean up response timer and buffer
	q.mu.Lock()
	if q.respTimer != nil {
		q.respTimer.Stop()
		q.respTimer = nil
	}
	q.respBuf = nil
	q.mu.Unlock()
}
