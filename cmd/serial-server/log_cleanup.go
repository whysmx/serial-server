package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const logRetention = 7 * 24 * time.Hour

func cleanupLogFiles() {
	cutoff := time.Now().Add(-logRetention)
	for _, name := range []string{logFile, "serial-server.issue.log"} {
		if name == "" {
			continue
		}
		if err := cleanupLogFile(name, cutoff); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] 清理日志失败: %s: %v\n", name, err)
		}
	}
}

func cleanupLogFile(name string, cutoff time.Time) error {
	info, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}

	src, err := os.Open(name)
	if err != nil {
		return err
	}
	defer src.Close()

	tmp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keepTmp := false
	defer func() {
		if !keepTmp {
			_ = os.Remove(tmpName)
		}
	}()

	changed, err := copyRetainedLogLines(src, tmp, cutoff)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Rename(tmpName, name); err != nil {
		return err
	}
	keepTmp = true
	return nil
}

func copyRetainedLogLines(src io.Reader, dst io.Writer, cutoff time.Time) (bool, error) {
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	changed := false
	keepCurrentEntry := true
	for scanner.Scan() {
		line := scanner.Text()
		if ts, ok := parseLogTimestamp(line); ok {
			keepCurrentEntry = !ts.Before(cutoff)
		}

		if keepCurrentEntry {
			if _, err := fmt.Fprintln(dst, line); err != nil {
				return changed, err
			}
		} else {
			changed = true
		}
	}
	if err := scanner.Err(); err != nil {
		return changed, err
	}
	return changed, nil
}

func parseLogTimestamp(line string) (time.Time, bool) {
	line = strings.TrimPrefix(line, "[ISSUE] ")
	if len(line) < len("2006/01/02 15:04:05") {
		return time.Time{}, false
	}

	end := len("2006/01/02 15:04:05")
	if len(line) > end && line[end] == '.' {
		end++
		for end < len(line) && line[end] >= '0' && line[end] <= '9' {
			end++
		}
	}

	ts, err := time.ParseInLocation("2006/01/02 15:04:05.999999", line[:end], time.Local)
	if err == nil {
		return ts, true
	}
	ts, err = time.ParseInLocation("2006/01/02 15:04:05", line[:len("2006/01/02 15:04:05")], time.Local)
	return ts, err == nil
}
