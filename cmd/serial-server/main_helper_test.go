package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoveSections tests the removeSections function
func TestRemoveSections(t *testing.T) {
	tests := []struct {
		name             string
		config           string
		sectionsToRemove []string
		expectedContains []string // Substrings that should be in result
		expectedMissing  []string // Substrings that should NOT be in result
	}{
		{
			name: "remove single section",
			config: `[listener1]
serial_port=/dev/ttyUSB0
listen_port=8000

[listener2]
serial_port=/dev/ttyUSB1
listen_port=8001`,
			sectionsToRemove: []string{"listener1"},
			expectedContains: []string{"[listener2]", "ttyUSB1", "8001"},
			expectedMissing:  []string{"[listener1]", "ttyUSB0", "8000"},
		},
		{
			name: "remove multiple sections",
			config: `[listener1]
port=8000

[listener2]
port=8001

[listener3]
port=8002`,
			sectionsToRemove: []string{"listener1", "listener3"},
			expectedContains: []string{"[listener2]", "8001"},
			expectedMissing:  []string{"[listener1]", "[listener3]", "8000", "8002"},
		},
		{
			name: "case insensitive section removal",
			config: `[Listener1]
port=8000

[LISTENER2]
port=8001`,
			sectionsToRemove: []string{"listener1", "listener2"},
			expectedContains: []string{},
			expectedMissing:  []string{"[Listener1]", "[LISTENER2]", "8000", "8001"},
		},
		{
			name:             "empty config",
			config:           "",
			sectionsToRemove: []string{"listener1"},
			expectedContains: []string{},
			expectedMissing:  []string{},
		},
		{
			name: "remove non-existent section",
			config: `[listener1]
port=8000`,
			sectionsToRemove: []string{"listener2"},
			expectedContains: []string{"[listener1]", "8000"},
			expectedMissing:  []string{},
		},
		{
			name: "remove all sections",
			config: `[a]
x=1

[b]
y=2`,
			sectionsToRemove: []string{"a", "b"},
			expectedContains: []string{},
			expectedMissing:  []string{"[a]", "[b]", "x=1", "y=2"},
		},
		{
			name: "preserve empty lines between sections",
			config: `[listener1]
port=8000

[listener2]
port=8001`,
			sectionsToRemove: []string{"listener1"},
			expectedContains: []string{"[listener2]", "8001"},
			expectedMissing:  []string{"[listener1]", "8000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeSections(tt.config, tt.sectionsToRemove)

			// Check expected substrings are present
			for _, expected := range tt.expectedContains {
				if !strings.Contains(result, expected) {
					t.Errorf("Result should contain '%s', but got:\n%s", expected, result)
				}
			}

			// Check expected substrings are missing
			for _, missing := range tt.expectedMissing {
				if strings.Contains(result, missing) {
					t.Errorf("Result should NOT contain '%s', but got:\n%s", missing, result)
				}
			}
		})
	}
}

// TestGetPortDescription tests the getPortDescription function
func TestGetPortDescription(t *testing.T) {
	tests := []struct {
		name     string
		port     string
		expected string
	}{
		{
			name:     "usb serial port (lowercase)",
			port:     "usbserial",
			expected: "USB 串口设备",
		},
		{
			name:     "usb in path",
			port:     "/dev/usb-serial",
			expected: "USB 串口设备",
		},
		{
			name:     "ttyS port (exact case)",
			port:     "/dev/ttyS0",
			expected: "标准串口",
		},
		{
			name:     "ttyS in path",
			port:     "ttyS1",
			expected: "标准串口",
		},
		{
			name:     "ttyACM port (exact case)",
			port:     "/dev/ttyACM0",
			expected: "USB CDC 设备",
		},
		{
			name:     "ttyACM in path",
			port:     "ttyACM1",
			expected: "USB CDC 设备",
		},
		{
			name:     "USB port (uppercase won't match lowercase 'usb')",
			port:     "/dev/ttyUSB0",
			expected: "串口设备", // Won't match 'usb' due to case sensitivity
		},
		{
			name:     "unknown port",
			port:     "/dev/ttyXXX",
			expected: "串口设备",
		},
		{
			name:     "COM port",
			port:     "COM1",
			expected: "串口设备",
		},
		{
			name:     "empty string",
			port:     "",
			expected: "串口设备",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPortDescription(tt.port)
			if result != tt.expected {
				t.Errorf("getPortDescription(%s) = %s, expected %s",
					tt.port, result, tt.expected)
			}
		})
	}
}

// TestMainContains tests the contains helper function in main.go
func TestMainContains(t *testing.T) {
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
			name:     "substring does not exist",
			s:        "hello world",
			substr:   "goodbye",
			expected: false,
		},
		{
			name:     "empty substring",
			s:        "hello",
			substr:   "",
			expected: true,
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
			s:        "Hello",
			substr:   "hello",
			expected: false,
		},
		{
			name:     "special characters",
			s:        "ttyUSB0",
			substr:   "USB",
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

// TestFindConfigFile tests the findConfigFile function
func TestFindConfigFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		setupFile      bool
		filename       string
		expectedExists bool
	}{
		{
			name:           "file exists in current directory",
			setupFile:      true,
			filename:       "test_config.ini",
			expectedExists: true,
		},
		{
			name:           "file does not exist",
			setupFile:      false,
			filename:       "nonexistent.ini",
			expectedExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFile {
				// Create test file in temp directory
				testPath := filepath.Join(tmpDir, tt.filename)
				if err := os.WriteFile(testPath, []byte("test content"), 0600); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}

				// Change to temp directory
				originalDir, _ := os.Getwd()
				defer func() { _ = os.Chdir(originalDir) }()
				if err := os.Chdir(tmpDir); err != nil {
					t.Fatalf("Failed to change directory: %v", err)
				}

				result := findConfigFile(tt.filename)
				if result == "" {
					t.Error("findConfigFile returned empty string")
				}

				// Verify file exists at returned path
				if _, err := os.Stat(result); err != nil {
					t.Errorf("findConfigFile returned non-existent path: %s", result)
				}
			} else {
				result := findConfigFile(tt.filename)
				// Should still return the filename even if it doesn't exist
				if result != tt.filename {
					t.Errorf("findConfigFile(%s) = %s, expected %s",
						tt.filename, result, tt.filename)
				}
			}
		})
	}
}

// TestScanAvailablePorts tests the ScanAvailablePorts wrapper function
func TestScanAvailablePorts(t *testing.T) {
	// This just calls the listener package function
	ports := ScanAvailablePorts()

	// Verify we get a valid slice (nil is ok if no ports found)
	if ports == nil {
		ports = []string{} // Treat nil as empty slice
	}

	// Verify no duplicates (if any ports found)
	portMap := make(map[string]bool)
	for _, port := range ports {
		if portMap[port] {
			t.Errorf("Duplicate port found: %s", port)
		}
		portMap[port] = true
	}

	t.Logf("ScanAvailablePorts found %d ports", len(ports))
}
