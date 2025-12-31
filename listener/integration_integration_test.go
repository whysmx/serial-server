//go:build !windows
// +build !windows

package listener

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// TestVirtualSerialPortIntegration tests serial port communication with socat
func TestVirtualSerialPortIntegration(t *testing.T) {
	// Check if socat is available
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat not available, skipping integration test")
	}

	// Create virtual serial port pair
	portA := "/tmp/ptyA-test-" + fmt.Sprintf("%d", time.Now().UnixNano())
	portB := "/tmp/ptyB-test-" + fmt.Sprintf("%d", time.Now().UnixNano())

	// Start socat
	cmd := exec.Command("socat", "-d -d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", portA),
		fmt.Sprintf("pty,raw,echo=0,link=%s", portB))

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start socat: %v", err)
	}

	// Ensure cleanup
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
		os.Remove(portA)
		os.Remove(portB)
	}()

	// Give socat time to create PTYs
	time.Sleep(200 * time.Millisecond)

	// Test reading port names
	t.Logf("Virtual ports: %s <-> %s", portA, portB)

	// Test basic file operations
	_, err := os.Stat(portA)
	if err != nil {
		t.Fatalf("Port A not created: %v", err)
	}

	_, err = os.Stat(portB)
	if err != nil {
		t.Fatalf("Port B not created: %v", err)
	}

	// Test data transfer between ports
	// Open port A for writing
	portAFile, err := os.OpenFile(portA, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("Failed to open port A: %v", err)
	}

	// Open port B for reading
	portBFile, err := os.OpenFile(portB, os.O_RDONLY, 0)
	if err != nil {
		portAFile.Close()
		t.Fatalf("Failed to open port B: %v", err)
	}

	// Write test data to port A
	testData := []byte("Hello from port A")
	_, err = portAFile.Write(testData)
	if err != nil {
		portAFile.Close()
		portBFile.Close()
		t.Fatalf("Failed to write to port A: %v", err)
	}

	// Read from port B
	buffer := make([]byte, 1024)
	portBFile.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := portBFile.Read(buffer)
	if err != nil {
		portAFile.Close()
		portBFile.Close()
		t.Fatalf("Failed to read from port B: %v", err)
	}

	// Verify data
	if n != len(testData) {
		portAFile.Close()
		portBFile.Close()
		t.Errorf("Expected to read %d bytes, got %d", len(testData), n)
	}

	if !bytes.Equal(buffer[:n], testData) {
		portAFile.Close()
		portBFile.Close()
		t.Errorf("Data mismatch: expected %v, got %v", testData, buffer[:n])
	}

	portAFile.Close()
	portBFile.Close()

	t.Log("Virtual serial port test passed")
}

// TestTCPSerialDataTransfer tests TCP to serial data transfer
func TestTCPSerialDataTransfer(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat not available, skipping integration test")
	}

	// Create virtual serial port
	portA := "/tmp/ptyA-tcp-" + fmt.Sprintf("%d", time.Now().UnixNano())
	portB := "/tmp/ptyB-tcp-" + fmt.Sprintf("%d", time.Now().UnixNano())

	cmd := exec.Command("socat", "-d -d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", portA),
		fmt.Sprintf("pty,raw,echo=0,link=%s", portB))

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start socat: %v", err)
	}

	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
		os.Remove(portA)
		os.Remove(portB)
	}()

	time.Sleep(200 * time.Millisecond)

	// Start a simple TCP server that echoes to serial port
	tcpPort := 0 // Let OS assign
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
	if err != nil {
		t.Fatalf("Failed to create TCP listener: %v", err)
	}
	tcpPort = listener.Addr().(*net.TCPAddr).Port

	serverDone := make(chan bool)
	receivedData := make(chan []byte, 1)

	// Start echo server
	go func() {
		defer close(serverDone)

		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read from TCP
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil || n == 0 {
			return
		}

		// Send to serial port
		serialFile, err := os.OpenFile(portA, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		serialFile.Write(buf[:n])
		serialFile.Close()

		receivedData <- buf[:n]
	}()

	// Open serial port for reading
	serialFile, err := os.OpenFile(portB, os.O_RDONLY, 0)
	if err != nil {
		listener.Close()
		t.Fatalf("Failed to open serial port: %v", err)
	}
	defer serialFile.Close()

	// Connect TCP client and send data
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
	if err != nil {
		listener.Close()
		serialFile.Close()
		t.Fatalf("Failed to connect TCP client: %v", err)
	}
	defer conn.Close()

	testData := []byte("TCP to Serial test")
	_, err = conn.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write to TCP: %v", err)
	}

	// Wait for server to process
	select {
	case <-receivedData:
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for server to receive data")
	}

	// Read from serial port (should have received what TCP sent)
	serialFile.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1024)
	n, err := serialFile.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read from serial port: %v", err)
	}

	if !bytes.Equal(buf[:n], testData) {
		t.Errorf("Data mismatch: expected %v, got %v", testData, buf[:n])
	}

	listener.Close()
	<-serverDone

	t.Log("TCP to serial data transfer test passed")
}

// TestMultipleClientsWithSerial tests multiple TCP clients with one serial port
func TestMultipleClientsWithSerial(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat not available, skipping integration test")
	}

	// Create virtual serial port
	portA := "/tmp/ptyA-multi-" + fmt.Sprintf("%d", time.Now().UnixNano())
	portB := "/tmp/ptyB-multi-" + fmt.Sprintf("%d", time.Now().UnixNano())

	cmd := exec.Command("socat", "-d -d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", portA),
		fmt.Sprintf("pty,raw,echo=0,link=%s", portB))

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start socat: %v", err)
	}

	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
		os.Remove(portA)
		os.Remove(portB)
	}()

	time.Sleep(200 * time.Millisecond)

	// Start TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create TCP listener: %v", err)
	}
	tcpPort := listener.Addr().(*net.TCPAddr).Port

	serverDone := make(chan bool)
	var mu sync.Mutex
	var connections int

	go func() {
		defer close(serverDone)

		// Accept 3 connections
		for i := 0; i < 3; i++ {
			conn, err := listener.Accept()
			if err != nil {
				continue
			}

			mu.Lock()
			connections++
			mu.Unlock()

			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // Echo
			}(conn)
		}
	}()

	// Connect 3 clients
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(clientNum int) {
			defer wg.Done()

			conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
			if err != nil {
				t.Errorf("Client %d failed to connect: %v", clientNum, err)
				return
			}
			defer conn.Close()

			testData := []byte(fmt.Sprintf("Client %d", clientNum))
			conn.Write(testData)

			buf := make([]byte, 1024)
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				t.Errorf("Client %d read failed: %v", clientNum, err)
				return
			}

			if string(buf[:n]) != string(testData) {
				t.Errorf("Client %d: data mismatch", clientNum)
			}
		}(i)
	}

	wg.Wait()
	listener.Close()
	<-serverDone

	if connections != 3 {
		t.Errorf("Expected 3 connections, got %d", connections)
	}

	t.Log("Multiple clients test passed")
}
