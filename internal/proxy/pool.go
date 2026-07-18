package proxy

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/vanes430/raced_proxy/internal/config"
	"github.com/vanes430/raced_proxy/internal/logger"
)

// proxies holds the current list of loaded proxy addresses (ip:port).
var proxies []string

// topWinners holds the fastest proxies sorted by latency (ascending).
var topWinners []string

// winnerMs maps proxy address to latency in milliseconds.
var winnerMs = make(map[string]int)

// maxWinners is the maximum number of winners kept in memory.
var maxWinners = 20

// hostIP is the real public IP of the host machine.
var hostIP string

// mu serializes all pool and winner state mutations.
var mu sync.RWMutex

// proxyFile is the path to the proxy list file (e.g. proxy.txt).
var proxyFile string

// archiveDir is the directory where rate-limited proxies are archived.
var archiveDir = "proxy_bekas"

// InitPool initializes the proxy pool with the given file path and host IP.
// file: path to the proxy list file. ip: the host's real public IP.
func InitPool(file string, ip string) {
	proxyFile = file
	hostIP = ip
	archiveDir = config.GetEnv("ARCHIVE_DIR", "proxy_bekas")
	LoadProxies()
	_ = os.MkdirAll(archiveDir, 0755) //nolint:gosec // non-sensitive archive dir
}

// GetProxies returns a snapshot of all loaded proxies.
// Returns: slice of proxy address strings (ip:port).
func GetProxies() []string {
	mu.RLock()
	defer mu.RUnlock()
	return proxies
}

// GetTopWinners returns a snapshot of the current top winners.
// Returns: slice of winning proxy address strings sorted by latency.
func GetTopWinners() []string {
	mu.RLock()
	defer mu.RUnlock()
	return topWinners
}

// LoadProxies reads the proxy file and replaces the in-memory proxy list.
// Parses lines matching ip:port format; updates mtime.
func LoadProxies() {
	mu.Lock()
	defer mu.Unlock()

	file, err := os.Open(proxyFile) //nolint:gosec // proxy file path from env
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	ipRe := regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+:\d+$`)
	var list []string
	scan := bufio.NewScanner(file)
	for scan.Scan() {
		l := strings.TrimSpace(scan.Text())
		if ipRe.MatchString(l) {
			list = append(list, l)
		}
	}
	proxies = list

	logger.Info("Loaded %d proxies from %s", len(list), proxyFile)
}

// GetProxiesCount returns the number of loaded proxies.
// Returns: integer count of proxies.
func GetProxiesCount() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(proxies)
}

// GetStats returns the total proxy count and current winner count.
// Returns: (total proxies, total winners).
func GetStats() (int, int) {
	mu.RLock()
	defer mu.RUnlock()
	return len(proxies), len(topWinners)
}

// GetHostIP returns the host's real public IP address.
// Returns: IP address string.
func GetHostIP() string {
	mu.RLock()
	defer mu.RUnlock()
	return hostIP
}
