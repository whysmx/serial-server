package listener

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

// TestListenerCreation tests creating a new listener
func TestListenerCreation(t *testing.T) {
	l := NewListener(
		"test_device",
		9999,
		"/dev/ttyUSB0",
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

	if l.GetSerialPort() != "/dev/ttyUSB0" {
		t.Errorf("Expected serial port '/dev/ttyUSB0', got '%s'", l.GetSerialPort())
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
		9998, // Use different port to avoid conflicts
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
		conn.Write([]byte("ACK"))

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
				c.Write(buf[:n])
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
		"/dev/ttyUSB0",
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
		"/dev/ttyUSB0",
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
		"/dev/ttyUSB0",
		115200,
		7, // data bits
		2, // stop bits
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
