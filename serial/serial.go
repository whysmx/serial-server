// Package serial provides serial port utilities for bridge.
package serial

import (
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/tarm/serial"
)

// Port represents a serial port connection.
type Port struct {
	config *serial.Config
	port   io.ReadWriteCloser
	mu     sync.RWMutex
	name   string
	baud   int
}

// Open opens a serial port with the given configuration.
func Open(portName string, baudRate int, dataBits int, stopBits int, parity string, rtscts bool) (*Port, error) {
	// Note: tarm/serial only supports basic config (Name + Baud)
	// More advanced options would require platform-specific code
	config := &serial.Config{
		Name:        portName,
		Baud:        baudRate,
		ReadTimeout: 0, // Blocking read
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", portName, err)
	}

	log.Printf("[serial] opened %s baud=%d", portName, baudRate)

	return &Port{
		config: config,
		port:   port,
		name:   portName,
		baud:   baudRate,
	}, nil
}

// Close closes the serial port.
func (p *Port) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.port != nil {
		err := p.port.Close()
		p.port = nil
		if err != nil {
			return fmt.Errorf("failed to close serial port %s: %w", p.name, err)
		}
		log.Printf("[serial] closed %s", p.name)
	}
	return nil
}

// Read reads data from the serial port.
func (p *Port) Read(b []byte) (n int, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.port == nil {
		return 0, fmt.Errorf("serial port %s is closed", p.name)
	}

	return p.port.Read(b)
}

// Write writes data to the serial port.
func (p *Port) Write(b []byte) (n int, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.port == nil {
		return 0, fmt.Errorf("serial port %s is closed", p.name)
	}

	return p.port.Write(b)
}

// Name returns the port name.
func (p *Port) Name() string {
	return p.name
}

// Baud returns the baud rate.
func (p *Port) Baud() int {
	return p.baud
}

// IsOpen checks if the port is open.
func (p *Port) IsOpen() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.port != nil
}
