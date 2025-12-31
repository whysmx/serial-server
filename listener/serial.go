// Package listener implements the serial server listener.
package listener

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

	var stopBitsVal serial.StopBits
	switch stopBits {
	case 1:
		stopBitsVal = serial.Stop1
	case 2:
		stopBitsVal = serial.Stop2
	default:
		return nil, fmt.Errorf("unsupported stop bits: %d (supported: 1 or 2)", stopBits)
	}

	if dataBits < 5 || dataBits > 8 {
		return nil, fmt.Errorf("unsupported data bits: %d (supported: 5-8)", dataBits)
	}

	if rtscts {
		log.Printf("[serial] WARNING: RTS/CTS flow control requested but not supported")
	}

	config := &serial.Config{
		Name:        portName,
		Baud:        baudRate,
		ReadTimeout: 50 * time.Millisecond,
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

func (p *Port) Read(b []byte) (n int, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.port == nil {
		return 0, fmt.Errorf("serial port %s is closed", p.name)
	}

	return p.port.Read(b)
}

func (p *Port) Write(b []byte) (n int, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.port == nil {
		return 0, fmt.Errorf("serial port %s is closed", p.name)
	}

	return p.port.Write(b)
}

func (p *Port) Name() string {
	return p.name
}

func (p *Port) Baud() int {
	return p.baud
}

func (p *Port) IsOpen() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.port != nil
}

// ======== Serial Helper Functions ========

type ComUsbPair struct {
	Lock sync.RWMutex
	Data map[string]string
}

var DefaultComUsb = NewComUsbPair()

func NewComUsbPair() *ComUsbPair {
	return &ComUsbPair{
		Data: make(map[string]string),
	}
}

func IsLinux() bool {
	return strings.Contains(strings.ToLower(runtime.GOOS), "linux")
}

func IsWindows() bool {
	return strings.Contains(strings.ToLower(runtime.GOOS), "windows")
}

func (c *ComUsbPair) UpdateComAndUsbPair() error {
	c.Lock.Lock()
	defer c.Lock.Unlock()

	if IsWindows() {
		c.Data = make(map[string]string)
		return nil
	}

	result, err := exec.Command("sh", "-c", "ls -l /dev/").Output()
	if err != nil {
		return err
	}

	c.Data = parseComUsbPair(string(result))
	return nil
}

func (c *ComUsbPair) GetUsbFromCom(comName string) string {
	c.Lock.RLock()
	defer c.Lock.RUnlock()

	if usbPath, ok := c.Data[comName]; ok && usbPath != "" {
		return usbPath
	}
	return comName
}

func (c *ComUsbPair) GetAllComNames() []string {
	c.Lock.RLock()
	defer c.Lock.RUnlock()

	comNames := make([]string, 0, len(c.Data))
	for k := range c.Data {
		comNames = append(comNames, k)
	}
	return comNames
}

func parseComUsbPair(output string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			continue
		}

		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])

		fields := strings.Fields(left)
		if len(fields) == 0 {
			continue
		}
		deviceName := fields[len(fields)-1]

		if !strings.HasPrefix(deviceName, "COM") && !strings.HasPrefix(deviceName, "RS485_") {
			continue
		}

		usbPath := right
		if !strings.HasPrefix(usbPath, "/dev/") {
			usbPath = "/dev/" + usbPath
		}
		result[deviceName] = usbPath
	}
	return result
}

func GetPortName(comName string, useOrgPortName bool) string {
	if IsWindows() {
		return comName
	}

	if strings.HasPrefix(comName, "/dev/") {
		return comName
	}

	if useOrgPortName {
		return "/dev/" + comName
	}

	usbPath := DefaultComUsb.GetUsbFromCom(comName)
	if usbPath != comName {
		return usbPath
	}

	return "/dev/" + comName
}

func ScanAvailablePorts() []string {
	var ports []string

	if IsWindows() {
		for i := 1; i <= 256; i++ {
			portName := fmt.Sprintf("COM%d", i)
			c := &serial.Config{Name: portName, Baud: 9600}
			if s, err := serial.OpenPort(c); err == nil {
				s.Close()
				ports = append(ports, portName)
			}
		}
	} else {
		if matches, err := filepath.Glob("/dev/COM*"); err == nil {
			for _, m := range matches {
				ports = append(ports, strings.TrimPrefix(m, "/dev/"))
			}
		}
		if matches, err := filepath.Glob("/dev/RS485_*"); err == nil {
			for _, m := range matches {
				ports = append(ports, strings.TrimPrefix(m, "/dev/"))
			}
		}

		serialPatterns := []string{
			"/dev/ttyUSB*",
			"/dev/ttyACM*",
			"/dev/ttyS*",
			"/dev/ttyFIQ*",
		}
		for _, pattern := range serialPatterns {
			if matches, err := filepath.Glob(pattern); err == nil {
				ports = append(ports, matches...)
			}
		}
	}
	return ports
}
