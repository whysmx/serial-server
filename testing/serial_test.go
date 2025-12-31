//go:build !windows

// Package testing provides test utilities for serial-server
package testing

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// VirtualSerialPort represents a virtual serial port for testing
// Uses socat on Linux to create connected PTY pairs
type VirtualSerialPort struct {
	portA    string
	portB    string
	cmd      *exec.Cmd
	stopChan chan struct{}
	mu       sync.Mutex
	closed   bool
}

// CreateVirtualSerialPort creates a pair of connected virtual serial ports
// Only works on Linux with socat installed
func CreateVirtualSerialPort() (*VirtualSerialPort, error) {
	// Try to find available PTY devices
	portA := "/tmp/ptyA-" + fmt.Sprintf("%d", time.Now().UnixNano())
	portB := "/tmp/ptyB-" + fmt.Sprintf("%d", time.Now().UnixNano())

	// Check if socat is available
	if _, err := exec.LookPath("socat"); err != nil {
		return nil, fmt.Errorf("socat not found: %w", err)
	}

	// Create socat command for PTY pair
	cmd := exec.Command("socat", "-d -d", "pty,raw,echo=0,link="+portA, "pty,raw,echo=0,link="+portB)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start socat: %w", err)
	}

	// Give socat time to create the PTYs
	time.Sleep(100 * time.Millisecond)

	vsp := &VirtualSerialPort{
		portA:    portA,
		portB:    portB,
		cmd:      cmd,
		stopChan: make(chan struct{}),
	}

	return vsp, nil
}

// PortAName returns the name of port A
func (v *VirtualSerialPort) PortAName() string {
	return v.portA
}

// PortBName returns the name of port B
func (v *VirtualSerialPort) PortBName() string {
	return v.portB
}

// WriteToPortA writes data to port A
func (v *VirtualSerialPort) WriteToPortA(data []byte) error {
	return v.writeToFile(v.portA, data)
}

// WriteToPortB writes data to port B
func (v *VirtualSerialPort) WriteToPortB(data []byte) error {
	return v.writeToFile(v.portB, data)
}

// ReadFromPortA reads data from port A
func (v *VirtualSerialPort) ReadFromPortA(buf []byte) (int, error) {
	return v.readFromFile(v.portA, buf)
}

// ReadFromPortB reads data from port B
func (v *VirtualSerialPort) ReadFromPortB(buf []byte) (int, error) {
	return v.readFromFile(v.portB, buf)
}

func (v *VirtualSerialPort) writeToFile(filename string, data []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return io.ErrClosedPipe
	}

	f, err := os.OpenFile(filename, os.O_WRONLY|unix.O_NONBLOCK, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

func (v *VirtualSerialPort) readFromFile(filename string, buf []byte) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return 0, io.ErrClosedPipe
	}

	f, err := os.OpenFile(filename, os.O_RDONLY|unix.O_NONBLOCK, 0666)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return f.Read(buf)
}

// Close closes the virtual serial port
func (v *VirtualSerialPort) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil
	}
	v.closed = true

	close(v.stopChan)

	if v.cmd != nil && v.cmd.Process != nil {
		v.cmd.Process.Kill()
		v.cmd.Wait()
	}

	return nil
}

// StartEchoServer starts an echo server on port A
func (v *VirtualSerialPort) StartEchoServer() {
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		buf := make([]byte, 1024)
		for {
			select {
			case <-v.stopChan:
				return
			case <-ticker.C:
				n, err := v.ReadFromPortA(buf)
				if err != nil {
					continue
				}
				if n > 0 {
					v.WriteToPortB(buf[:n])
				}
			}
		}
	}()
}

// FindAvailableTCPPort finds an available TCP port
func FindAvailableTCPPort() (int, error) {
	// Try to bind to port 0 and let OS assign
	listener, err := exec.Command("sh", "-c", "python3 -c 'import socket; s=socket.socket(); s.bind(0); print(s.getsockname()[1])'").Output()
	if err != nil {
		// Fallback to a common test port
		return 19999, nil
	}

	var port int
	_, err = fmt.Sscanf(string(listener), "%d", &port)
	if err != nil {
		return 19999, nil
	}
	return port, nil
}
