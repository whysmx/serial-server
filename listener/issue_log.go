package listener

import (
	"log"
	"os"
	"sync"
	"time"
)

var (
	issueLogger     *log.Logger
	issueLoggerOnce sync.Once

	issueThrottleMu   sync.Mutex
	issueThrottleLast = make(map[string]time.Time)
)

func getIssueLogger() *log.Logger {
	issueLoggerOnce.Do(func() {
		f, err := os.OpenFile("serial-server.issue.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			issueLogger = log.New(os.Stderr, "[ISSUE] ", log.LstdFlags|log.Lmicroseconds)
			issueLogger.Printf("failed to open issue log file: %v", err)
			return
		}
		issueLogger = log.New(f, "[ISSUE] ", log.LstdFlags|log.Lmicroseconds)
	})
	return issueLogger
}

func logIssuef(format string, args ...any) {
	getIssueLogger().Printf(format, args...)
}

func logIssuefThrottled(key string, interval time.Duration, format string, args ...any) {
	if interval <= 0 {
		logIssuef(format, args...)
		return
	}

	now := time.Now()
	issueThrottleMu.Lock()
	last := issueThrottleLast[key]
	if !last.IsZero() && now.Sub(last) < interval {
		issueThrottleMu.Unlock()
		return
	}
	issueThrottleLast[key] = now
	issueThrottleMu.Unlock()

	logIssuef(format, args...)
}
