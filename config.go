// Package main - serial-server
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
)

// Config represents the application configuration.
type Config struct {
	Listeners []*ListenerConfig
}

// ListenerConfig represents a single serial listener configuration.
type ListenerConfig struct {
	Name          string
	ListenPort    int
	SerialPort    string
	BaudRate      int
	DataBits      int
	StopBits      int
	Parity        string
	DisplayFormat string
}

// Default values.
const (
	DefaultBaudRate      = 9600
	DefaultDataBits      = 8
	DefaultStopBits      = 1
	DefaultParity        = "N"
	DefaultDisplayFormat = "HEX"
)

// Load loads configuration from the specified file.
func Load(path string) (*Config, error) {
	cfg := &Config{}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil // Return empty config if file doesn't exist
	}

	// Load INI file
	iniCfg, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// Parse each section
	for _, section := range iniCfg.Sections() {
		// Skip default section and empty section names
		if section.Name() == "DEFAULT" || section.Name() == "" {
			continue
		}

		listener, err := parseListenerSection(section)
		if err != nil {
			return nil, fmt.Errorf("error in section [%s]: %w", section.Name(), err)
		}
		if listener != nil {
			cfg.Listeners = append(cfg.Listeners, listener)
		}
	}

	return cfg, nil
}

// parseListenerSection parses a single INI section into ListenerConfig.
func parseListenerSection(section *ini.Section) (*ListenerConfig, error) {
	// Get serial_port (required for serial mode)
	serialPort := section.Key("serial_port").String()
	if serialPort == "" {
		return nil, nil // Skip non-listener sections
	}

	// Get listen_port (required)
	listenPort, err := section.Key("listen_port").Int()
	if err != nil || listenPort <= 0 || listenPort > 65535 {
		return nil, fmt.Errorf("invalid listen_port: %s", section.Key("listen_port").String())
	}

	listener := &ListenerConfig{
		Name:          section.Name(),
		ListenPort:    listenPort,
		SerialPort:    serialPort,
		BaudRate:      DefaultBaudRate,
		DataBits:      DefaultDataBits,
		StopBits:      DefaultStopBits,
		Parity:        DefaultParity,
		DisplayFormat: DefaultDisplayFormat,
	}

	// Optional fields with defaults
	if baudRate, err := section.Key("baud_rate").Int(); err == nil && baudRate > 0 {
		listener.BaudRate = baudRate
	}
	if dataBits, err := section.Key("data_bits").Int(); err == nil && dataBits > 0 {
		listener.DataBits = dataBits
	}
	if stopBits, err := section.Key("stop_bits").Int(); err == nil && stopBits > 0 {
		listener.StopBits = stopBits
	}
	if parity := section.Key("parity").String(); parity != "" {
		listener.Parity = strings.ToUpper(parity)
	}
	if displayFormat := section.Key("display_format").String(); displayFormat != "" {
		listener.DisplayFormat = strings.ToUpper(displayFormat)
	}

	return listener, nil
}

// Save saves configuration to the specified file.
func Save(path string, cfg *Config) error {
	iniCfg := ini.Empty()

	for _, listener := range cfg.Listeners {
		section := iniCfg.Section(listener.Name)
		section.Key("listen_port").SetValue(strconv.Itoa(listener.ListenPort))
		section.Key("serial_port").SetValue(listener.SerialPort)
		section.Key("baud_rate").SetValue(strconv.Itoa(listener.BaudRate))
		if listener.DataBits != DefaultDataBits {
			section.Key("data_bits").SetValue(strconv.Itoa(listener.DataBits))
		}
		if listener.StopBits != DefaultStopBits {
			section.Key("stop_bits").SetValue(strconv.Itoa(listener.StopBits))
		}
		if listener.Parity != DefaultParity {
			section.Key("parity").SetValue(listener.Parity)
		}
		if listener.DisplayFormat != DefaultDisplayFormat {
			section.Key("display_format").SetValue(listener.DisplayFormat)
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	return iniCfg.SaveTo(path)
}

// FindListenerByPort finds a listener by its listen port.
func (c *Config) FindListenerByPort(port int) *ListenerConfig {
	for _, l := range c.Listeners {
		if l.ListenPort == port {
			return l
		}
	}
	return nil
}

// FindListenerByName finds a listener by its section name.
func (c *Config) FindListenerByName(name string) *ListenerConfig {
	for _, l := range c.Listeners {
		if l.Name == name {
			return l
		}
	}
	return nil
}

// RemoveListener removes a listener from the config.
func (c *Config) RemoveListener(name string) {
	newListeners := make([]*ListenerConfig, 0, len(c.Listeners))
	for _, l := range c.Listeners {
		if l.Name != name {
			newListeners = append(newListeners, l)
		}
	}
	c.Listeners = newListeners
}

// AddListener adds a listener to the config.
func (c *Config) AddListener(listener *ListenerConfig) {
	c.Listeners = append(c.Listeners, listener)
}
