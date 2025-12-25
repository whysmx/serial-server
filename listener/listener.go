// Package listener implements the serial server listener.
package listener

import (
	"serial-server/serial"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// DisplayFormat represents the data display format.
type DisplayFormat string

const (
	FormatHEX    DisplayFormat = "HEX"
	FormatUTF8   DisplayFormat = "UTF8"
	FormatGB2312 DisplayFormat = "GB2312"
)

// Stats holds statistics for a listener.
type Stats struct {
	TxBytes   uint64 // Bytes sent to serial
	RxBytes   uint64 // Bytes received from serial
	TxPackets uint64
	RxPackets uint64
	Clients   int
}

// Listener represents a serial server listener.
type Listener struct {
	name          string
	listenPort    int
	serialPort    string
	baudRate      int
	displayFormat DisplayFormat
	maxClients    int

	// TCP listener
	tcpListener net.Listener

	// Serial port
	serial *serial.Port

	// Client connections
	clients map[string]net.Conn
	mu      sync.RWMutex

	// Data channels
	rxChan chan []byte // Data received from serial

	// Control channels
	stopChan chan struct{}
	doneChan chan struct{}

	// Stats
	stats Stats

	// Callbacks
	onData func(data []byte, direction string)
}

// NewListener creates a new serial listener.
func NewListener(name string, listenPort int, serialPort string, baudRate int, displayFormat DisplayFormat, maxClients int) *Listener {
	return &Listener{
		name:          name,
		listenPort:    listenPort,
		serialPort:    serialPort,
		baudRate:      baudRate,
		displayFormat: displayFormat,
		maxClients:    maxClients,
		clients:       make(map[string]net.Conn),
		rxChan:        make(chan []byte, 1024),
		stopChan:      make(chan struct{}),
		doneChan:      make(chan struct{}),
	}
}

// Start starts the listener.
func (l *Listener) Start() error {
	var err error

	// Open serial port
	l.serial, err = serial.Open(l.serialPort, l.baudRate, 8, 1, "none", false)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %w", err)
	}

	// Start TCP listener
	l.tcpListener, err = net.Listen("tcp", fmt.Sprintf(":%d", l.listenPort))
	if err != nil {
		l.serial.Close()
		return fmt.Errorf("failed to listen on port %d: %w", l.listenPort, err)
	}

	log.Printf("[listener:%s] listening on :%d -> %s baud=%d (max_clients=%d)",
		l.name, l.listenPort, l.serialPort, l.baudRate, l.maxClients)

	// Start goroutines
	go l.acceptLoop()
	go l.serialReadLoop()

	return nil
}

// Stop stops the listener.
func (l *Listener) Stop() {
	close(l.stopChan)

	if l.tcpListener != nil {
		l.tcpListener.Close()
	}

	l.mu.Lock()
	for _, conn := range l.clients {
		conn.Close()
	}
	l.mu.Unlock()

	if l.serial != nil {
		l.serial.Close()
	}

	<-l.doneChan
	log.Printf("[listener:%s] stopped", l.name)
}

// acceptLoop accepts incoming TCP connections.
func (l *Listener) acceptLoop() {
	defer func() {
		close(l.doneChan)
	}()

	for {
		select {
		case <-l.stopChan:
			return
		default:
		}

		conn, err := l.tcpListener.Accept()
		if err != nil {
			if l.isTemporaryError(err) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if err == net.ErrClosed {
				return
			}
			log.Printf("[listener:%s] accept error: %v", l.name, err)
			return
		}

		l.handleNewConnection(conn)
	}
}

// isTemporaryError checks if the error is temporary.
func (l *Listener) isTemporaryError(err error) bool {
	netErr, ok := err.(net.Error)
	return ok && netErr.Temporary()
}

// handleNewConnection handles a new TCP connection.
func (l *Listener) handleNewConnection(conn net.Conn) {
	addr := conn.RemoteAddr().String()

	l.mu.Lock()
	shouldAccept := true

	// Check max clients
	if l.maxClients > 0 && len(l.clients) >= l.maxClients {
		shouldAccept = false
	}

	if !shouldAccept {
		l.mu.Unlock()
		log.Printf("[listener:%s] client %s rejected (max clients exceeded)", l.name, addr)
		conn.Close()
		return
	}

	// Kick old client if max_clients = 1
	if l.maxClients == 1 {
		for _, oldConn := range l.clients {
			log.Printf("[listener:%s] kicking old client %s", l.name, oldConn.RemoteAddr().String())
			oldConn.Close()
		}
		l.clients = make(map[string]net.Conn)
	}

	l.clients[addr] = conn
	l.mu.Unlock()

	log.Printf("[listener:%s] client connected %s (total: %d)", l.name, addr, l.getClientCount())

	// Handle client
	go l.handleClient(conn, addr)
}

// handleClient handles a single client connection.
func (l *Listener) handleClient(conn net.Conn, addr string) {
	defer func() {
		l.mu.Lock()
		if _, ok := l.clients[addr]; ok {
			delete(l.clients, addr)
			log.Printf("[listener:%s] client disconnected %s (remaining: %d)",
				l.name, addr, l.getClientCount())
		}
		l.mu.Unlock()
		conn.Close()
	}()

	buf := make([]byte, 4096)
	for {
		select {
		case <-l.stopChan:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, err := conn.Read(buf)
		if err != nil {
			if l.isTemporaryError(err) {
				continue
			}
			if err == io.EOF || l.isClosedError(err.Error()) {
				return
			}
			return
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			atomic.AddUint64(&l.stats.TxBytes, uint64(n))
			atomic.AddUint64(&l.stats.TxPackets, 1)

			l.fireOnData(data, "tx")

			// Send to serial
			l.sendToSerial(data)
		}
	}
}

// isClosedError checks if the error is due to closed connection.
func (l *Listener) isClosedError(msg string) bool {
	return len(msg) > 0 && (contains(msg, "use of closed") || contains(msg, "connection reset"))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// serialReadLoop reads data from serial port.
func (l *Listener) serialReadLoop() {
	if l.serial == nil {
		return
	}

	buf := make([]byte, 4096)
	for {
		select {
		case <-l.stopChan:
			return
		default:
		}

		n, err := l.serial.Read(buf)
		if err != nil {
			if l.isClosedError(err.Error()) {
				return
			}
			log.Printf("[listener:%s] serial read error: %v", l.name, err)
			continue
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			atomic.AddUint64(&l.stats.RxBytes, uint64(n))
			atomic.AddUint64(&l.stats.RxPackets, 1)

			l.fireOnData(data, "rx")

			// Forward to all clients
			l.broadcastToClients(data)
		}
	}
}

// sendToSerial sends data to serial port.
func (l *Listener) sendToSerial(data []byte) {
	if l.serial == nil {
		return
	}

	_, err := l.serial.Write(data)
	if err != nil {
		log.Printf("[listener:%s] serial write error: %v", l.name, err)
	}
}

// broadcastToClients sends data to all connected clients.
func (l *Listener) broadcastToClients(data []byte) {
	l.mu.RLock()
	clients := make([]net.Conn, 0, len(l.clients))
	for _, conn := range l.clients {
		clients = append(clients, conn)
	}
	l.mu.RUnlock()

	for _, conn := range clients {
		_, err := conn.Write(data)
		if err != nil {
			log.Printf("[listener:%s] client write error: %v", l.name, err)
		}
	}
}

// SetOnData sets the data callback.
func (l *Listener) SetOnData(fn func(data []byte, direction string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onData = fn
}

// fireOnData fires the data callback.
func (l *Listener) fireOnData(data []byte, direction string) {
	l.mu.RLock()
	fn := l.onData
	l.mu.RUnlock()

	if fn != nil {
		fn(data, direction)
	}
}

// GetStats returns current statistics.
func (l *Listener) GetStats() Stats {
	return Stats{
		TxBytes:   atomic.LoadUint64(&l.stats.TxBytes),
		RxBytes:   atomic.LoadUint64(&l.stats.RxBytes),
		TxPackets: atomic.LoadUint64(&l.stats.TxPackets),
		RxPackets: atomic.LoadUint64(&l.stats.RxPackets),
		Clients:   l.getClientCount(),
	}
}

func (l *Listener) getClientCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.clients)
}

// GetName returns the listener name.
func (l *Listener) GetName() string {
	return l.name
}

// GetListenPort returns the listen port.
func (l *Listener) GetListenPort() int {
	return l.listenPort
}

// GetSerialPort returns the serial port path.
func (l *Listener) GetSerialPort() string {
	return l.serialPort
}

// GetBaudRate returns the baud rate.
func (l *Listener) GetBaudRate() int {
	return l.baudRate
}

// GetDisplayFormat returns the display format.
func (l *Listener) GetDisplayFormat() DisplayFormat {
	return l.displayFormat
}

// GetMaxClients returns the max clients setting.
func (l *Listener) GetMaxClients() int {
	return l.maxClients
}

// FormatForDisplay formats data for display.
func FormatForDisplay(data []byte, format DisplayFormat) string {
	switch format {
	case FormatHEX:
		return hex.EncodeToString(data)
	default:
		// Clean non-printable characters
		var buf []byte
		for _, b := range data {
			if b >= 32 && b <= 126 {
				buf = append(buf, b)
			} else if b == 9 || b == 10 || b == 13 {
				buf = append(buf, b)
			} else {
				buf = append(buf, '.')
			}
		}
		return string(buf)
	}
}
