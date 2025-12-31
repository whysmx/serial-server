// Package listener implements the serial server listener.
package listener

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
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
	// Stats
	stats Stats

	// Synchronization
	mu sync.RWMutex

	// Client connections
	clients        map[string]net.Conn
	clientIndexMap map[string]string // addr -> index (e.g., "127.0.0.1:12345" -> "#1")

	// Configuration fields
	name          string
	listenPort    int
	serialPort    string
	baudRate      int
	dataBits      int
	stopBits      int
	parity        string
	displayFormat DisplayFormat
	maxClients    int
	clientCounter  uint64

	// TCP listener
	tcpListener net.Listener

	// Serial port
	serial *Port

	// Write queue for multi-client serialization
	writeQueue *WriteQueue

	// Data channels
	rxChan chan []byte // Data received from serial

	// Control channels
	stopChan chan struct{}
	doneChan chan struct{}

	// Callbacks
	onData func(data []byte, direction string, clientID string)

	// Serial frame buffer for accumulating incomplete frames
	serialBuffer []byte
}

// NewListener creates a new serial listener.
func NewListener(name string, listenPort int, serialPort string, baudRate int, dataBits int, stopBits int, parity string, displayFormat DisplayFormat) *Listener {
	return &Listener{
		name:           name,
		listenPort:     listenPort,
		serialPort:     serialPort,
		baudRate:       baudRate,
		dataBits:       dataBits,
		stopBits:       stopBits,
		parity:         parity,
		displayFormat:  displayFormat,
		clients:        make(map[string]net.Conn),
		clientIndexMap: make(map[string]string),
		clientCounter:  0,
		rxChan:         make(chan []byte, 1024),
		stopChan:       make(chan struct{}),
		doneChan:       make(chan struct{}),
	}
}

// Start starts the listener.
func (l *Listener) Start() error {
	var err error

	// 更新 COM -> USB 映射
	if err := DefaultComUsb.UpdateComAndUsbPair(); err != nil {
		log.Printf("[listener:%s] warning: failed to update COM-USB mapping: %v", l.name, err)
	}

	// 获取实际串口路径
	actualPort := GetPortName(l.serialPort, false)

	// Convert parity letter to lowercase
	parityLower := strings.ToLower(l.parity)

	// Open serial port
	l.serial, err = Open(actualPort, l.baudRate, l.dataBits, l.stopBits, parityLower, false)
	if err != nil {
		return fmt.Errorf("failed to open serial port %s (设备路径: %s): %w", l.serialPort, actualPort, err)
	}

	// Initialize write queue
	l.writeQueue = NewWriteQueue(l.serial)
	l.writeQueue.StartCleanupTimer()

	// Start TCP listener
	l.tcpListener, err = net.Listen("tcp", fmt.Sprintf(":%d", l.listenPort))
	if err != nil {
		l.writeQueue.StopCleanupTimer()
		l.serial.Close()
		return fmt.Errorf("failed to listen on port %d: %w", l.listenPort, err)
	}

	log.Printf("[listener:%s] listening on :%d -> %s baud=%d (max_clients=%d)",
		l.name, l.listenPort, actualPort, l.baudRate, l.maxClients)

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

	if l.writeQueue != nil {
		l.writeQueue.StopCleanupTimer()
	}

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
	l.clients[addr] = conn
	l.mu.Unlock()

	// Handle client
	go l.handleClient(conn, addr)
}

// handleClient handles a single client connection.
func (l *Listener) handleClient(conn net.Conn, addr string) {
	// Assign client index
	l.mu.Lock()
	l.clientCounter++
	clientIndex := fmt.Sprintf("#%d", l.clientCounter)
	l.clientIndexMap[addr] = clientIndex
	clientCount := len(l.clients) // Get count BEFORE releasing lock
	l.mu.Unlock()

	// Log AFTER releasing lock to avoid deadlock
	log.Printf("[listener:%s] client connected %s -> %s (total: %d)",
		l.name, addr, clientIndex, clientCount)

	defer func() {
		l.mu.Lock()
		if _, ok := l.clients[addr]; ok {
			delete(l.clients, addr)
			delete(l.clientIndexMap, addr)
			remaining := len(l.clients) // Get count BEFORE releasing lock
			l.mu.Unlock()
			log.Printf("[listener:%s] client disconnected %s (remaining: %d)",
				l.name, clientIndex, remaining)
		} else {
			l.mu.Unlock()
		}
		conn.Close()
	}()

	buf := make([]byte, 65536) // 64KB buffer for better performance
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

			l.mu.RLock()
			clientIndex := l.clientIndexMap[addr]
			l.mu.RUnlock()

			l.fireOnData(data, "tx", clientIndex)

			// Send to serial via queue (for multi-client)
			respCh := l.writeQueue.Send(addr, data)

			// Handle response in separate goroutine
			// Capture clientIndex to avoid locking in goroutine
			go func(idx string) {
				resp, ok := <-respCh
				if ok && len(resp) > 0 {
					// Send response back to this client only
					conn.Write(resp)

					atomic.AddUint64(&l.stats.RxBytes, uint64(len(resp)))
					atomic.AddUint64(&l.stats.RxPackets, 1)

					l.fireOnData(resp, "rx", idx)
				}
			}(clientIndex)
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

// serialReadLoop reads data from serial port with frame buffering.
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
			// 超时或 EOF（带 ReadTimeout 时）是正常的，用于检测帧结束
			if err.Error() == "timeout" || err.Error() == "i/o timeout" || err == io.EOF {
				// 超时说明帧间隔到达，如果有缓冲数据则提交完整帧
				if len(l.serialBuffer) > 0 {
					frame := make([]byte, len(l.serialBuffer))
					copy(frame, l.serialBuffer)
					l.writeQueue.OnSerialData(frame)
					l.serialBuffer = nil
				}
				continue
			}
			if l.isClosedError(err.Error()) {
				return
			}
			log.Printf("[listener:%s] serial read error: %v", l.name, err)
			continue
		}

		if n > 0 {
			// 追加到缓冲区
			l.serialBuffer = append(l.serialBuffer, buf[:n]...)
		}
	}
}

// SetOnData sets the data callback.
func (l *Listener) SetOnData(fn func(data []byte, direction string, clientID string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onData = fn
}

// fireOnData fires the data callback.
func (l *Listener) fireOnData(data []byte, direction string, clientID string) {
	l.mu.RLock()
	fn := l.onData
	l.mu.RUnlock()

	if fn != nil {
		fn(data, direction, clientID)
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

// GetDataBits returns the data bits.
func (l *Listener) GetDataBits() int {
	return l.dataBits
}

// GetStopBits returns the stop bits.
func (l *Listener) GetStopBits() int {
	return l.stopBits
}

// GetParity returns the parity setting.
func (l *Listener) GetParity() string {
	return l.parity
}

// FormatForDisplay formats data for display.
func FormatForDisplay(data []byte, format DisplayFormat) string {
	switch format {
	case FormatHEX:
		// 紧凑显示：每 16 字节一行，格式为：xx xx xx ...
		var result []string
		for i := 0; i < len(data); i += 16 {
			end := i + 16
			if end > len(data) {
				end = len(data)
			}
			line := ""
			for j := i; j < end; j++ {
				if j > i {
					line += " "
				}
				line += fmt.Sprintf("%02x", data[j])
			}
			result = append(result, line)
		}
		return strings.Join(result, "\n")
	default:
		return cleanNonPrintable(data)
	}
}

// cleanNonPrintable cleans non-printable characters from byte array.
func cleanNonPrintable(data []byte) string {
	var buf []byte
	for _, b := range data {
		switch {
		case b >= 32 && b <= 126:
			buf = append(buf, b)
		case b == 9 || b == 10 || b == 13:
			buf = append(buf, b)
		default:
			buf = append(buf, '.')
		}
	}
	return string(buf)
}

// FormatForDisplayCompact formats data for display in compact mode (no line breaks).
func FormatForDisplayCompact(data []byte, format DisplayFormat) string {
	switch format {
	case FormatHEX:
		// 紧凑显示：所有字节在一行，格式为：xx xx xx ...
		var result []string
		for _, b := range data {
			result = append(result, fmt.Sprintf("%02x", b))
		}
		return strings.Join(result, " ")
	default:
		return cleanNonPrintable(data)
	}
}
