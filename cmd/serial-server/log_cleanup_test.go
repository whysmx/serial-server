package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestCopyRetainedLogLinesKeepsLastWeek(t *testing.T) {
	input := strings.Join([]string{
		"2026/08/01 10:00:00.000001 old entry",
		"old detail line",
		"2026/08/15 10:00:00.000001 new entry",
		"new detail line",
		"",
	}, "\n")

	var out bytes.Buffer
	changed, err := copyRetainedLogLines(strings.NewReader(input), &out, time.Date(2026, 8, 9, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("copyRetainedLogLines failed: %v", err)
	}
	if !changed {
		t.Fatal("expected log cleanup to report changes")
	}

	got := out.String()
	if strings.Contains(got, "old entry") || strings.Contains(got, "old detail line") {
		t.Fatalf("old log entry was retained:\n%s", got)
	}
	if !strings.Contains(got, "new entry") || !strings.Contains(got, "new detail line") {
		t.Fatalf("new log entry was removed:\n%s", got)
	}
}

func TestCopyRetainedLogLinesParsesIssuePrefix(t *testing.T) {
	input := "[ISSUE] 2026/08/01 10:00:00.000001 old issue\n[ISSUE] 2026/08/15 10:00:00.000001 new issue\n"

	var out bytes.Buffer
	changed, err := copyRetainedLogLines(strings.NewReader(input), &out, time.Date(2026, 8, 9, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("copyRetainedLogLines failed: %v", err)
	}
	if !changed {
		t.Fatal("expected issue log cleanup to report changes")
	}

	got := out.String()
	if strings.Contains(got, "old issue") {
		t.Fatalf("old issue entry was retained:\n%s", got)
	}
	if !strings.Contains(got, "new issue") {
		t.Fatalf("new issue entry was removed:\n%s", got)
	}
}

func TestCopyRetainedLogLinesKeepsUnparseableHeader(t *testing.T) {
	input := "manual header\n2026/08/15 10:00:00 recent entry\n"

	var out bytes.Buffer
	changed, err := copyRetainedLogLines(strings.NewReader(input), &out, time.Date(2026, 8, 9, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("copyRetainedLogLines failed: %v", err)
	}
	if changed {
		t.Fatal("did not expect cleanup changes")
	}
	if got := out.String(); !strings.Contains(got, "manual header") || !strings.Contains(got, "recent entry") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}
