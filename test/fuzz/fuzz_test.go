package fuzz

import (
	"os"
	"testing"

	"github.com/whysmx/serial-server/config"
)

// Fuzz tests for configuration parsing
func FuzzParseConfig(f *testing.F) {
	// Add seed corpus
	f.Add([]byte("[listener1]\n"))
	f.Add([]byte("[listener1]\nserial_port=/dev/ttyUSB0\n"))
	f.Add([]byte("[listener1]\nserial_port=/dev/ttyUSB0\nlisten_port=8000\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Create temporary config file
		tmpDir := t.TempDir()
		configPath := tmpDir + "/test.ini"

		// Write fuzz data to file
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return
		}

		// Try to load - should not crash
		_, _ = config.Load(configPath)
	})
}

// Fuzz tests for listener configuration
func FuzzListenerConfig(f *testing.F) {
	// Add seed corpus
	f.Add("test_listener")
	f.Add("")
	f.Add("listener-with-dash")
	f.Add("listener_with_underscore")

	f.Fuzz(func(t *testing.T, name string) {
		cfg := &config.Config{
			Listeners: []*config.ListenerConfig{
				{
					Name:          name,
					SerialPort:    "/dev/ttyUSB0",
					ListenPort:    8000,
					BaudRate:      115200,
					DataBits:      8,
					StopBits:      1,
					Parity:        "N",
					DisplayFormat: "HEX",
				},
			},
		}

		// Operations should not crash
		_ = cfg.FindListenerByName(name)
		_ = cfg.FindListenerByPort(8000)
	})
}
