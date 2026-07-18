package proxy

import (
	"os"
	"strings"
	"time"

	"raced_proxy/internal/logger"
)

// ArchiveRateLimited appends a proxy to today's archive file and removes it
// from the active pool. proxyStr: the proxy address (ip:port) to archive.
func ArchiveRateLimited(proxyStr string) {
	today := time.Now().Format("2006-01-02") + ".txt"
	path := archiveDir + "/" + today

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(proxyStr + "\n")
		f.Close()
	}

	DeleteProxy(proxyStr)
	logger.Warn("Archived %s (rate limited)", proxyStr)
}

// IsRateLimitedToday checks whether a proxy was already archived today.
// proxyStr: the proxy address (ip:port) to check.
// Returns: true if the proxy appears in today's archive file.
func IsRateLimitedToday(proxyStr string) bool {
	today := time.Now().Format("2006-01-02") + ".txt"
	path := archiveDir + "/" + today

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == proxyStr {
			return true
		}
	}
	return false
}

// DeleteProxy removes a proxy from both the active pool and the winners list,
// then triggers an async persist to proxy.txt.
// proxyStr: the proxy address (ip:port) to remove.
func DeleteProxy(proxyStr string) {
	mu.Lock()
	idx := -1
	for i, p := range proxies {
		if p == proxyStr {
			idx = i
			break
		}
	}
	if idx >= 0 {
		proxies = append(proxies[:idx], proxies[idx+1:]...)
	}
	wIdx := -1
	for i, p := range topWinners {
		if p == proxyStr {
			wIdx = i
			break
		}
	}
	if wIdx >= 0 {
		topWinners = append(topWinners[:wIdx], topWinners[wIdx+1:]...)
	}
	delete(winnerMs, proxyStr)
	mu.Unlock()

	triggerPersist()
}
