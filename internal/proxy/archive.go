package proxy

import (
	"os"
	"strings"
	"time"

	"raced_proxy/internal/logger"
)

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
