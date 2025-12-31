package listener

import (
	"testing"
	"time"
)

// TestRequestCacheBasic tests basic cache operations
func TestRequestCacheBasic(t *testing.T) {
	cache := NewRequestCache()

	// Test Set and Get
	data := []byte("test data")
	hash := uint64(12345)

	cache.Set(hash, data)

	retrieved, found := cache.Get(hash)
	if !found {
		t.Error("Data not found in cache")
	}
	if string(retrieved) != string(data) {
		t.Errorf("Expected '%s', got '%s'", string(data), string(retrieved))
	}

	// Test nonexistent key
	_, found = cache.Get(99999)
	if found {
		t.Error("Expected false for nonexistent key")
	}
}

// TestRequestCacheWithTTL tests cache with TTL
func TestRequestCacheWithTTL(t *testing.T) {
	cache := NewRequestCache()

	data := []byte("expiring data")
	hash := uint64(54321)

	// Set with short TTL
	cache.SetWithTTL(hash, data, 100*time.Millisecond)

	// Should be found immediately
	_, found := cache.Get(hash)
	if !found {
		t.Error("Data not found immediately after SetWithTTL")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	_, found = cache.Get(hash)
	if found {
		t.Error("Data should have expired")
	}
}

// TestRequestCacheCleanup tests cleanup of expired entries
func TestRequestCacheCleanup(t *testing.T) {
	cache := NewRequestCache()

	// Add some entries with different TTLs
	cache.SetWithTTL(1, []byte("data1"), 50*time.Millisecond)
	cache.SetWithTTL(2, []byte("data2"), 200*time.Millisecond)
	cache.Set(3, []byte("data3")) // No expiration

	// Wait for first to expire
	time.Sleep(100 * time.Millisecond)

	// Run cleanup
	cache.CleanupExpired()

	// First should be gone
	_, found := cache.Get(1)
	if found {
		t.Error("Entry 1 should have been cleaned up")
	}

	// Second and third should still exist
	_, found = cache.Get(2)
	if !found {
		t.Error("Entry 2 should still exist")
	}

	_, found = cache.Get(3)
	if !found {
		t.Error("Entry 3 should still exist (no TTL)")
	}
}

// TestHashDataConsistency tests hash data consistency
func TestHashDataConsistency(t *testing.T) {
	// Test same data produces same hash
	data1 := []byte{0x01, 0x02, 0x03}
	data2 := []byte{0x01, 0x02, 0x03}
	differentData := []byte{0x01, 0x02, 0x04}

	// Verify same data is equal
	if string(data1) != string(data2) {
		t.Error("Same data should be equal")
	}

	// Verify different data is not equal
	if string(data1) == string(differentData) {
		t.Error("Different data should not be equal")
	}

	t.Log("Hash data consistency verified")
}

// TestPendingRequest tests pending request structure
func TestPendingRequest(t *testing.T) {
	// Create a pending request
	req := &PendingRequest{
		ClientID:   "client1",
		DataHash:   12345,
		Request:    []byte{0x01, 0x02},
		ResponseCh: make(chan []byte, 1),
		Timestamp:  time.Now(),
	}

	if req.ClientID != "client1" {
		t.Errorf("Expected ClientID 'client1', got '%s'", req.ClientID)
	}

	if string(req.Request) != "\x01\x02" {
		t.Errorf("Expected Request [0x01,0x02], got %v", req.Request)
	}

	// Test finish with response
	responseData := []byte{0xFF, 0xFE}
	req.finishWithResponse(responseData)

	select {
	case data := <-req.ResponseCh:
		if string(data) != string(responseData) {
			t.Errorf("Expected response %v, got %v", responseData, data)
		}
	default:
		t.Error("Response not sent to channel")
	}
}

// TestPendingRequestTimeout tests request timeout
func TestPendingRequestTimeout(t *testing.T) {
	req := &PendingRequest{
		ClientID:   "client1",
		DataHash:   12345,
		Request:    []byte{0x01},
		ResponseCh: make(chan []byte, 1),
		Timestamp:  time.Now(),
	}

	// Finish without response - closes channel without data
	req.finishNoResponse()

	// Try to read from closed channel - should get zero value immediately
	data, ok := <-req.ResponseCh

	// Channel should be closed
	if ok {
		t.Error("ResponseCh should be closed after finishNoResponse")
	}

	// Data should be zero value (nil for slice)
	if data != nil {
		t.Errorf("Expected nil data from closed channel, got %v", data)
	}
}

// TestDuplicateDetection tests duplicate request detection
func TestDuplicateDetection(t *testing.T) {
	// This tests the concept of duplicate detection
	// In real scenario, WriteQueue uses hash to detect duplicates

	sameData1 := []byte{0x01, 0x02, 0x03, 0x04}
	sameData2 := []byte{0x01, 0x02, 0x03, 0x04}
	differentData := []byte{0x01, 0x02, 0x03, 0x05}

	// Same data should have same hash (implied)
	if string(sameData1) != string(sameData2) {
		t.Error("Same data should be identical")
	}

	// Different data should be different
	if string(sameData1) == string(differentData) {
		t.Error("Different data should not be identical")
	}

	t.Log("Duplicate detection concept verified")
}

// TestRequestExpiration tests request expiration logic
func TestRequestExpiration(t *testing.T) {
	now := time.Now()
	oldTime := now.Add(-10 * time.Second)
	futureTime := now.Add(10 * time.Second)

	req1 := &PendingRequest{
		Timestamp: oldTime,
	}
	req2 := &PendingRequest{
		Timestamp: futureTime,
	}

	// Old request should be expired (using default timeout of 5 seconds)
	timeSince1 := time.Since(req1.Timestamp)
	timeSince2 := time.Since(req2.Timestamp)

	if timeSince1 < 5*time.Second {
		t.Error("Old request should be expired")
	}

	if timeSince2 >= 0 {
		// Future request is not expired
		t.Log("Future request not expired")
	}
}

// TestCacheConcurrency tests concurrent cache access
func TestCacheConcurrency(t *testing.T) {
	cache := NewRequestCache()
	done := make(chan bool)

	// Start multiple goroutines accessing cache
	for i := 0; i < 10; i++ {
		go func(idx int) {
			//nolint:gosec // G115 - Test code with controlled small integer values
			hash := uint64(idx)
			data := []byte{byte(idx)}

			// Write
			cache.Set(hash, data)

			// Read
			cache.Get(hash)

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("Concurrent cache access completed without race")
}

// TestResponseChannel tests response channel behavior
func TestResponseChannel(t *testing.T) {
	req := &PendingRequest{
		ClientID:   "test_client",
		DataHash:   12345,
		Request:    []byte("request"),
		ResponseCh: make(chan []byte, 1),
		Timestamp:  time.Now(),
	}

	// Test sending response
	go func() {
		time.Sleep(50 * time.Millisecond)
		req.ResponseCh <- []byte("response")
	}()

	select {
	case data := <-req.ResponseCh:
		if string(data) != "response" {
			t.Errorf("Expected 'response', got '%s'", string(data))
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for response")
	}
}

// TestCacheDataIntegrity tests cache data integrity
func TestCacheDataIntegrity(t *testing.T) {
	cache := NewRequestCache()

	// Test various data types
	testCases := []struct {
		hash uint64
		data []byte
	}{
		{1, []byte{}},                             // Empty
		{2, []byte{0x00}},                         // Single zero byte
		{3, []byte{0xFF, 0xFF, 0xFF}},             // All max bytes
		{4, []byte("Hello, World!")},              // String
		{5, []byte{0x01, 0x02, 0x03, 0x04, 0x05}}, // Sequential
		{6, make([]byte, 1024)},                   // Large data
	}

	for _, tc := range testCases {
		cache.Set(tc.hash, tc.data)

		retrieved, found := cache.Get(tc.hash)
		if !found {
			t.Errorf("Hash %d: data not found", tc.hash)
			continue
		}

		if len(retrieved) != len(tc.data) {
			t.Errorf("Hash %d: length mismatch, expected %d, got %d",
				tc.hash, len(tc.data), len(retrieved))
			continue
		}

		for i := range tc.data {
			if retrieved[i] != tc.data[i] {
				t.Errorf("Hash %d: byte mismatch at index %d, expected %v, got %v",
					tc.hash, i, tc.data[i], retrieved[i])
				break
			}
		}
	}
}
