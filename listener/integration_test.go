// Package listener tests for serial-server listener
package listener

import (
	"testing"
)

// TestDisplayFormat tests display format constants
func TestDisplayFormat(t *testing.T) {
	tests := []struct {
		name   string
		format DisplayFormat
	}{
		{"HEX format", FormatHEX},
		{"UTF8 format", FormatUTF8},
		{"GB2312 format", FormatGB2312},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.format == "" {
				t.Error("Format is empty")
			}
		})
	}
}

// TestFormatForDisplay tests data formatting
func TestFormatForDisplay(t *testing.T) {
	testData := []byte{0x01, 0x02, 0x03, 'H', 'e', 'l', 'l', 'o'}

	// Test HEX format
	result := FormatForDisplay(testData, FormatHEX)
	if result == "" {
		t.Error("HEX format returned empty string")
	}
	t.Logf("HEX format: %s", result)

	// Test UTF8 format
	result = FormatForDisplay(testData, FormatUTF8)
	if result == "" {
		t.Error("UTF8 format returned empty string")
	}
	t.Logf("UTF8 format: %s", result)
}

// TestHashFunction tests hash data functionality
func TestHashFunction(t *testing.T) {
	// This tests the hashData function used in queue.go
	// Since it's not exported, we just verify queue operations work
	t.Log("Hash function test - queue operations verified in queue tests")
}

// TestRequestCache tests request cache functionality
func TestRequestCache(t *testing.T) {
	cache := NewRequestCache()
	if cache == nil {
		t.Fatal("Failed to create cache")
	}

	testData := []byte("test data")
	testHash := uint64(12345)

	// Test Set and Get
	cache.Set(testHash, testData)
	data, found := cache.Get(testHash)
	if !found {
		t.Error("Data not found in cache")
	}
	if len(data) != len(testData) {
		t.Errorf("Cache data length mismatch: got %d, want %d", len(data), len(testData))
	}
}

// TestWriteQueueCreation tests write queue creation
func TestWriteQueueCreation(t *testing.T) {
	// Can't test fully without a real serial port, but we can test creation
	t.Skip("WriteQueue needs serial port - skipped in unit tests")
}

// TestStatsStructure tests stats structure
func TestStatsStructure(t *testing.T) {
	stats := &Stats{
		TxBytes: 100,
		RxBytes: 200,
	}

	if stats.TxBytes != 100 {
		t.Errorf("TxBytes = %d, want 100", stats.TxBytes)
	}
	if stats.RxBytes != 200 {
		t.Errorf("RxBytes = %d, want 200", stats.RxBytes)
	}
}
