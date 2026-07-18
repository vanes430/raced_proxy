package proxy

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"raced_proxy/internal/logger"
)

// WatchProxyFile watches the proxy file for changes via SHA-256 hash and
// reloads automatically every 3 seconds when content changes.
func WatchProxyFile() {
	var prevHash string
	for {
		time.Sleep(3 * time.Second)
		data, err := os.ReadFile(proxyFile)
		if err != nil {
			continue
		}
		h := fmt.Sprintf("%x", sha256.Sum256(data))
		if h == prevHash {
			continue
		}
		prevHash = h
		LoadProxies()
		logger.Info("Reloaded %d proxies automatically (proxy.txt changed)", len(proxies))
	}
}

// GetRealIP fetches the machine's real public IP from ifconfig.me.
// Returns: IP address string, or error if the request fails.
func GetRealIP() (string, error) {
	resp, err := http.Get("https://ifconfig.me/ip")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(body)), nil
}

// PrintTopWinners prints up to limit top winners with their latency to stdout.
// limit: maximum number of winners to display.
func PrintTopWinners(limit int) {
	mu.RLock()
	defer mu.RUnlock()

	if len(topWinners) == 0 {
		fmt.Println("\n  No winners.")
		return
	}
	n := limit
	if n > len(topWinners) {
		n = len(topWinners)
	}
	fmt.Println("")
	for i := 0; i < n; i++ {
		p := topWinners[i]
		fmt.Printf("  %2d. %-21s %dms\n", i+1, p, winnerMs[p])
	}
	fmt.Println("")
}

// ResetWinners clears all winners and their latency data.
func ResetWinners() {
	mu.Lock()
	defer mu.Unlock()
	topWinners = nil
	winnerMs = make(map[string]int)
}
