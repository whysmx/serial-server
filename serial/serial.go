// Package serial provides serial port utilities for bridge.
package serial

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"

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
	// Map parity string to serial.Parity
	var parityVal serial.Parity
	switch parity {
	case "N", "n", "None", "":
		parityVal = serial.ParityNone
	case "O", "o", "Odd":
		parityVal = serial.ParityOdd
	case "E", "e", "Even":
		parityVal = serial.ParityEven
	default:
		return nil, fmt.Errorf("unsupported parity: %s (supported: N/O/E)", parity)
	}

	// Map stop bits to serial.StopBits
	var stopBitsVal serial.StopBits
	switch stopBits {
	case 1:
		stopBitsVal = serial.Stop1
	case 2:
		stopBitsVal = serial.Stop2
	default:
		return nil, fmt.Errorf("unsupported stop bits: %d (supported: 1 or 2)", stopBits)
	}

	// Validate data bits
	if dataBits < 5 || dataBits > 8 {
		return nil, fmt.Errorf("unsupported data bits: %d (supported: 5-8)", dataBits)
	}

	// Note: rtscts (hardware flow control) is not supported by tarm/serial library
	// If you need RTS/CTS, consider using a different serial library like go-bah
	if rtscts {
		log.Printf("[serial] WARNING: RTS/CTS flow control requested but not supported by current serial library")
	}

	config := &serial.Config{
		Name:        portName,
		Baud:        baudRate,
		ReadTimeout: 50 * time.Millisecond, // Non-blocking with timeout for frame detection
		Size:        byte(dataBits),
		Parity:      parityVal,
		StopBits:    stopBitsVal,
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", portName, err)
	}

	log.Printf("[serial] opened %s baud=%d size=%d parity=%s stop=%d",
		portName, baudRate, dataBits, parity, stopBits)

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
