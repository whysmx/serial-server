package listener

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

const testSerialPort = "/dev/ttyUSB0"

// TestListenerCreation tests creating a new listener
func TestListenerCreation(t *testing.T) {
	l := NewListener(
		"test_device",
		9999,
		testSerialPort,
		115200,
		8,
		1,
		"N",
		FormatHEX,
	)

	if l == nil {
		t.Fatal("NewListener returned nil")
	}

	if l.GetName() != "test_device" {
		t.Errorf("Expected name 'test_device', got '%s'", l.GetName())
	}

	if l.GetListenPort() != 9999 {
		t.Errorf("Expected port 9999, got %d", l.GetListenPort())
	}

	if l.GetSerialPort() != testSerialPort {
		t.Errorf("Expected serial port '%s', got '%s'", testSerialPort, l.GetSerialPort())
	}

	if l.GetBaudRate() != 115200 {
		t.Errorf("Expected baud rate 115200, got %d", l.GetBaudRate())
	}
}

// TestListenerStartStop tests starting and stopping a listener
// Note: This will fail to start the serial port, but tests the TCP listener setup
func TestListenerStartStop(t *testing.T) {
	l := NewListener(
		"test_device",
		9998,               // Use different port to avoid conflicts
		"/dev/nonexistent", // Non-existent serial port
		115200,
		8,
		1,
		"N",
		FormatHEX,
	)

	// Try to start - will fail on serial port but tests the setup
	err := l.Start()
	if err == nil {
		t.Log("Listener started (serial port not available)")
		// Stop it anyway
		l.Stop()
	} else {
		t.Logf("Listener start failed as expected (no serial port): %v", err)
	}
}

// TestTCPClientConnection tests TCP client connection without serial port
func TestTCPClientConnection(t *testing.T) {
	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create a simple TCP server for testing
	serverStarted := make(chan bool)
	serverStopped := make(chan bool)
	dataReceived := make(chan []byte, 10)

	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Errorf("Server listen failed: %v", err)
			return
		}
		defer ln.Close()

		serverStarted <- true

		// Accept one connection
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read data from client
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		if n > 0 {
			dataReceived <- buf[:n]
		}

		// Write response
		_, _ = conn.Write([]byte("ACK"))

		serverStopped <- true
	}()

	// Wait for server to start
	<-serverStarted
	time.Sleep(100 * time.Millisecond)

	// Connect as client
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}
	defer conn.Close()

	// Send data
	testData := []byte("Hello, Server!")
	_, err = conn.Write(testData)
	if err != nil {
		t.Fatalf("Client write failed: %v", err)
	}

	// Read response
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Client read failed: %v", err)
	}

	if string(buf[:n]) != "ACK" {
		t.Errorf("Expected 'ACK', got '%s'", string(buf[:n]))
	}

	// Verify server received data
	select {
	case data := <-dataReceived:
		if string(data) != string(testData) {
			t.Errorf("Server received different data: expected '%s', got '%s'",
				string(testData), string(data))
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not receive data within timeout")
	}

	// Wait for server to stop
	<-serverStopped
}

// TestMultipleClients tests multiple concurrent client connections
func TestMultipleClients(t *testing.T) {
	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	serverStarted := make(chan bool)
	clientsDone := make(chan bool)
	var serverWg sync.WaitGroup

	// Start server
	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Errorf("Server listen failed: %v", err)
			return
		}
		defer ln.Close()

		serverStarted <- true

		// Accept multiple connections
		for i := 0; i < 3; i++ {
			conn, err := ln.Accept()
			if err != nil {
				continue
			}

			serverWg.Add(1)
			go func(c net.Conn) {
				defer c.Close()
				defer serverWg.Done()

				buf := make([]byte, 1024)
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}

				// Echo back
				_, _ = c.Write(buf[:n])
			}(conn)
		}

		serverWg.Wait()
		clientsDone <- true
	}()

	// Wait for server to start
	<-serverStarted
	time.Sleep(100 * time.Millisecond)

	// Connect multiple clients
	var clientWg sync.WaitGroup
	for i := 0; i < 3; i++ {
		clientWg.Add(1)
		go func(clientNum int) {
			defer clientWg.Done()

			conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				t.Errorf("Client %d connect failed: %v", clientNum, err)
				return
			}
			defer conn.Close()

			testData := []byte(fmt.Sprintf("Client %d", clientNum))
			_, err = conn.Write(testData)
			if err != nil {
				t.Errorf("Client %d write failed: %v", clientNum, err)
				return
			}

			buf := make([]byte, 1024)
			n, err := conn.Read(buf)
			if err != nil {
				t.Errorf("Client %d read failed: %v", clientNum, err)
				return
			}

			if string(buf[:n]) != string(testData) {
				t.Errorf("Client %d: expected '%s', got '%s'",
					clientNum, string(testData), string(buf[:n]))
			}
		}(i)
	}

	clientWg.Wait()

	// Wait for server to finish
	select {
	case <-clientsDone:
	case <-time.After(5 * time.Second):
		t.Error("Server did not complete within timeout")
	}
}

// TestListenerStats tests stats tracking
func TestListenerStats(t *testing.T) {
	l := NewListener(
		"test_device",
		9997,
		testSerialPort,
		115200,
		8,
		1,
		"N",
		FormatHEX,
	)

	stats := l.GetStats()

	// Initial stats should be zero
	if stats.TxBytes != 0 {
		t.Errorf("Expected initial TxBytes 0, got %d", stats.TxBytes)
	}
	if stats.RxBytes != 0 {
		t.Errorf("Expected initial RxBytes 0, got %d", stats.RxBytes)
	}
	if stats.TxPackets != 0 {
		t.Errorf("Expected initial TxPackets 0, got %d", stats.TxPackets)
	}
	if stats.RxPackets != 0 {
		t.Errorf("Expected initial RxPackets 0, got %d", stats.RxPackets)
	}
	if stats.Clients != 0 {
		t.Errorf("Expected initial Clients 0, got %d", stats.Clients)
	}
}

// TestOnDataCallback tests OnData callback
func TestOnDataCallback(t *testing.T) {
	l := NewListener(
		"test_device",
		9996,
		testSerialPort,
		115200,
		8,
		1,
		"N",
		FormatHEX,
	)

	received := make(chan bool)
	var receivedData []byte
	var receivedDirection string
	var receivedClientID string

	l.SetOnData(func(data []byte, direction string, clientID string) {
		receivedData = data
		receivedDirection = direction
		receivedClientID = clientID
		close(received)
	})

	// Simulate data callback (normally called internally)
	l.fireOnData([]byte{0x01, 0x02, 0x03}, "tx", "client1")

	select {
	case <-received:
		if string(receivedData) != "\x01\x02\x03" {
			t.Errorf("Expected data [0x01,0x02,0x03], got %v", receivedData)
		}
		if receivedDirection != "tx" {
			t.Errorf("Expected direction 'tx', got '%s'", receivedDirection)
		}
		if receivedClientID != "client1" {
			t.Errorf("Expected clientID 'client1', got '%s'", receivedClientID)
		}
	case <-time.After(1 * time.Second):
		t.Error("OnData callback was not triggered")
	}
}

// TestGetConfigMethods tests various getter methods
func TestGetConfigMethods(t *testing.T) {
	l := NewListener(
		"test_device",
		9995,
		testSerialPort,
		115200,
		7,   // data bits
		2,   // stop bits
		"E", // parity
		FormatUTF8,
	)

	if l.GetDataBits() != 7 {
		t.Errorf("Expected data bits 7, got %d", l.GetDataBits())
	}

	if l.GetStopBits() != 2 {
		t.Errorf("Expected stop bits 2, got %d", l.GetStopBits())
	}

	if l.GetParity() != "E" {
		t.Errorf("Expected parity 'E', got '%s'", l.GetParity())
	}

	if l.GetDisplayFormat() != FormatUTF8 {
		t.Errorf("Expected display format '%s', got '%s'", FormatUTF8, l.GetDisplayFormat())
	}

	if l.GetMaxClients() != 0 {
		t.Errorf("Expected max clients 0 (default), got %d", l.GetMaxClients())
	}
}

// TestContains tests the contains helper function
func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{
			name:     "substring exists",
			s:        "hello world",
			substr:   "world",
			expected: true,
		},
		{
			name:     "substring at start",
			s:        "hello world",
			substr:   "hello",
			expected: true,
		},
		{
			name:     "substring at end",
			s:        "hello world",
			substr:   "world",
			expected: true,
		},
		{
			name:     "substring does not exist",
			s:        "hello world",
			substr:   "goodbye",
			expected: false,
		},
		{
			name:     "empty substring",
			s:        "hello world",
			substr:   "",
			expected: true, // empty string is always contained
		},
		{
			name:     "empty string",
			s:        "",
			substr:   "test",
			expected: false,
		},
		{
			name:     "both empty",
			s:        "",
			substr:   "",
			expected: true,
		},
		{
			name:     "case sensitive",
			s:        "Hello World",
			substr:   "hello",
			expected: false,
		},
		{
			name:     "special characters",
			s:        "use of closed network connection",
			substr:   "closed",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v, expected %v",
					tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

// TestIsClosedError tests the isClosedError method
func TestIsClosedError(t *testing.T) {
	l := NewListener("test", 9999, testSerialPort, 115200, 8, 1, "N", FormatHEX)

	tests := []struct {
		name     string
		msg      string
		expected bool
	}{
		{
			name:     "use of closed connection",
			msg:      "use of closed network connection",
			expected: true,
		},
		{
			name:     "use of closed file",
			msg:      "use of closed file",
			expected: true,
		},
		{
			name:     "connection reset by peer",
			msg:      "connection reset by peer",
			expected: true,
		},
		{
			name:     "connection reset",
			msg:      "read: connection reset",
			expected: true,
		},
		{
			name:     "normal error",
			msg:      "some other error",
			expected: false,
		},
		{
			name:     "empty string",
			msg:      "",
			expected: false,
		},
		{
			name:     "timeout error",
			msg:      "i/o timeout",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := l.isClosedError(tt.msg)
			if result != tt.expected {
				t.Errorf("isClosedError(%q) = %v, expected %v",
					tt.msg, result, tt.expected)
			}
		})
	}
}

// TestIsTemporaryError tests the isTemporaryError method
func TestIsTemporaryError(t *testing.T) {
	l := NewListener("test", 9999, testSerialPort, 115200, 8, 1, "N", FormatHEX)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "non-temporary error",
			err:      &testError{msg: "some error", timeout: false},
			expected: false,
		},
		{
			name:     "temporary error (timeout)",
			err:      &testError{msg: "timeout error", timeout: true},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := l.isTemporaryError(tt.err)
			if result != tt.expected {
				t.Errorf("isTemporaryError(%v) = %v, expected %v",
					tt.err, result, tt.expected)
			}
		})
	}
}

// testError implements net.Error for testing
type testError struct {
	msg     string
	timeout bool
}

func (e *testError) Error() string   { return e.msg }
func (e *testError) Timeout() bool   { return e.timeout }
func (e *testError) Temporary() bool { return e.timeout } // Deprecated but kept for compatibility

// TestFormatForDisplayCompact tests the FormatForDisplayCompact function
func TestFormatForDisplayCompact(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		format   DisplayFormat
		notEmpty bool
	}{
		{
			name:     "HEX format with data",
			data:     []byte{0x01, 0x02, 0x03},
			format:   FormatHEX,
			notEmpty: true,
		},
		{
			name:     "UTF8 format with data",
			data:     []byte("Hello"),
			format:   FormatUTF8,
			notEmpty: true,
		},
		{
			name:     "GB2312 format with data",
			data:     []byte{0xC4, 0xE3}, // "你" in GB2312
			format:   FormatGB2312,
			notEmpty: true,
		},
		{
			name:     "empty data",
			data:     []byte{},
			format:   FormatHEX,
			notEmpty: false,
		},
		{
			name:     "nil data",
			data:     nil,
			format:   FormatHEX,
			notEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatForDisplayCompact(tt.data, tt.format)
			if tt.notEmpty && result == "" {
				t.Errorf("FormatForDisplayCompact() returned empty string for %v", tt.data)
			}
			if !tt.notEmpty && result != "" {
				t.Errorf("FormatForDisplayCompact() should return empty for nil/empty data, got %s", result)
			}
		})
	}
}

// ==================== Benchmarks ====================

// BenchmarkFormatForDisplayHEX benchmarks HEX formatting performance
func BenchmarkFormatForDisplayHEX(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FormatForDisplay(data, FormatHEX)
	}
}

// BenchmarkFormatForDisplayHEXSmall benchmarks small data (32 bytes) HEX formatting
func BenchmarkFormatForDisplayHEXSmall(b *testing.B) {
	data := make([]byte, 32)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FormatForDisplay(data, FormatHEX)
	}
}

// BenchmarkFormatForDisplayHEXLarge benchmarks large data (4KB) HEX formatting
func BenchmarkFormatForDisplayHEXLarge(b *testing.B) {
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FormatForDisplay(data, FormatHEX)
	}
}

// BenchmarkFormatForDisplayUTF8 benchmarks UTF8 formatting performance
func BenchmarkFormatForDisplayUTF8(b *testing.B) {
	data := []byte("这是一段中文测试数据，用于测试UTF-8编码的性能表现。This is English text for performance testing. mixed混合内容.")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FormatForDisplay(data, FormatUTF8)
	}
}

// BenchmarkFormatForDisplayGB2312 benchmarks GB2312 formatting performance
func BenchmarkFormatForDisplayGB2312(b *testing.B) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FormatForDisplay(data, FormatGB2312)
	}
}

// BenchmarkFormatForDisplayCompact benchmarks compact formatting
func BenchmarkFormatForDisplayCompact(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FormatForDisplayCompact(data, FormatHEX)
	}
}
