package listener

import (
	"runtime"
	"strings"
	"testing"
)

func TestNewComUsbPair(t *testing.T) {
	pair := NewComUsbPair()
	if pair == nil {
		t.Fatal("NewComUsbPair returned nil")
	}
	if pair.Data == nil {
		t.Error("Data map is not initialized")
	}
}

func TestIsLinux(t *testing.T) {
	result := IsLinux()
	expected := strings.Contains(strings.ToLower(runtime.GOOS), "linux")
	if result != expected {
		t.Errorf("IsLinux() = %v, expected %v", result, expected)
	}
}

func TestIsWindows(t *testing.T) {
	result := IsWindows()
	expected := strings.Contains(strings.ToLower(runtime.GOOS), "windows")
	if result != expected {
		t.Errorf("IsWindows() = %v, expected %v", result, expected)
	}
}

func TestParseComUsbPair(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:  "valid COM entry",
			input: "lrwxrwxrwx 1 root root 7 Jan 1 00:00 COM1 -> /dev/ttyUSB0",
			expected: map[string]string{
				"COM1": "/dev/ttyUSB0",
			},
		},
		{
			name:  "valid RS485 entry",
			input: "lrwxrwxrwx 1 root root 7 Jan 1 00:00 RS485_1 -> /dev/ttyUSB0",
			expected: map[string]string{
				"RS485_1": "/dev/ttyUSB0",
			},
		},
		{
			name: "multiple entries",
			input: `lrwxrwxrwx 1 root root 7 Jan 1 00:00 COM1 -> /dev/ttyUSB0
				lrwxrwxrwx 1 root root 7 Jan 1 00:01 COM2 -> /dev/ttyUSB1`,
			expected: map[string]string{
				"COM1": "/dev/ttyUSB0",
				"COM2": "/dev/ttyUSB1",
			},
		},
		{
			name:     "invalid entry - no arrow",
			input:    "lrwxrwxrwx 1 root root 7 Jan 1 00:00 COM1",
			expected: map[string]string{},
		},
		{
			name:     "invalid entry - wrong device prefix",
			input:    "lrwxrwxrwx 1 root root 7 Jan 1 00:00 ttyUSB0 -> /dev/ttyUSB0",
			expected: map[string]string{},
		},
		{
			name:  "entry without /dev/ prefix on right side",
			input: "lrwxrwxrwx 1 root root 7 Jan 1 00:00 COM1 -> ttyUSB0",
			expected: map[string]string{
				"COM1": "/dev/ttyUSB0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseComUsbPair(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseComUsbPair() returned %d items, expected %d", len(result), len(tt.expected))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("parseComUsbPair()[%s] = %s, expected %s", k, result[k], v)
				}
			}
		})
	}
}

func TestComUsbPair_UpdateComAndUsbPair(t *testing.T) {
	pair := NewComUsbPair()

	// On Windows, should just clear the map
	if IsWindows() {
		pair.Data["test"] = "value"
		err := pair.UpdateComAndUsbPair()
		if err != nil {
			t.Errorf("UpdateComAndUsbPair() failed: %v", err)
		}
		if len(pair.Data) != 0 {
			t.Errorf("UpdateComAndUsbPair() on Windows should clear data, got %d items", len(pair.Data))
		}
	} else {
		// On non-Windows, should parse ls output
		err := pair.UpdateComAndUsbPair()
		// Don't fail on error, as it may fail in container/env
		if err != nil {
			t.Logf("UpdateComAndUsbPair() failed (expected in some environments): %v", err)
		}
		// Just verify it doesn't crash
		if pair.Data == nil {
			t.Error("UpdateComAndUsbPair() left Data as nil")
		}
	}
}

func TestComUsbPair_GetUsbFromCom(t *testing.T) {
	pair := NewComUsbPair()
	pair.Data["COM1"] = "/dev/ttyUSB0"
	pair.Data["COM2"] = "/dev/ttyUSB1"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "existing COM",
			input:    "COM1",
			expected: "/dev/ttyUSB0",
		},
		{
			name:     "another existing COM",
			input:    "COM2",
			expected: "/dev/ttyUSB1",
		},
		{
			name:     "non-existent COM",
			input:    "COM999",
			expected: "COM999",
		},
		{
			name:     "empty COM",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pair.GetUsbFromCom(tt.input)
			if result != tt.expected {
				t.Errorf("GetUsbFromCom(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestComUsbPair_GetAllComNames(t *testing.T) {
	pair := NewComUsbPair()
	pair.Data["COM1"] = "/dev/ttyUSB0"
	pair.Data["COM2"] = "/dev/ttyUSB1"
	pair.Data["COM3"] = "/dev/ttyUSB2"

	names := pair.GetAllComNames()

	if len(names) != 3 {
		t.Errorf("GetAllComNames() returned %d names, expected 3", len(names))
	}

	// Check that all names are present
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}

	for _, expected := range []string{"COM1", "COM2", "COM3"} {
		if !nameMap[expected] {
			t.Errorf("GetAllComNames() missing %s", expected)
		}
	}
}

func TestGetPortName(t *testing.T) {
	tests := []struct {
		name           string
		comName        string
		useOrgPortName bool
		// We can't easily test the exact result since it depends on DefaultComUsb
		// Just verify it doesn't crash and returns something reasonable
	}{
		{
			name:           "Windows COM port",
			comName:        "COM1",
			useOrgPortName: false,
		},
		{
			name:           "already has /dev/ prefix",
			comName:        "/dev/ttyUSB0",
			useOrgPortName: false,
		},
		{
			name:           "simple device name",
			comName:        "ttyUSB0",
			useOrgPortName: false,
		},
		{
			name:           "use original port name",
			comName:        "ttyUSB0",
			useOrgPortName: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPortName(tt.comName, tt.useOrgPortName)
			if result == "" {
				t.Errorf("GetPortName(%s, %v) returned empty string", tt.comName, tt.useOrgPortName)
			}
			// Verify Windows doesn't add /dev/
			if IsWindows() && strings.HasPrefix(tt.comName, "COM") {
				if result != tt.comName {
					t.Errorf("GetPortName(%s, %v) on Windows should return unchanged, got %s", tt.comName, tt.useOrgPortName, result)
				}
			}
			// Verify paths starting with /dev/ keep it
			if strings.HasPrefix(tt.comName, "/dev/") && result != tt.comName {
				t.Errorf("GetPortName(%s, %v) should keep /dev/ prefix, got %s", tt.comName, tt.useOrgPortName, result)
			}
		})
	}
}

func TestScanAvailablePorts(t *testing.T) {
	// This test just verifies it doesn't crash
	ports := ScanAvailablePorts()

	// Verify returned slice is valid (even if empty or nil)
	if ports == nil {
		ports = []string{} // Treat nil as empty slice for further checks
	}

	// If running on Windows, might get COM ports
	// If running on Linux, might get various serial ports
	// Just verify no duplicates
	portMap := make(map[string]bool)
	duplicates := []string{}
	for _, port := range ports {
		if portMap[port] {
			duplicates = append(duplicates, port)
		}
		portMap[port] = true
	}

	if len(duplicates) > 0 {
		t.Errorf("ScanAvailablePorts() returned duplicates: %v", duplicates)
	}

	t.Logf("ScanAvailablePorts() found %d ports: %v", len(ports), ports)
}
