package proxy

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"raced_proxy/internal/logger"
)

var (
	proxies    []string
	topWinners []string
	winnerMs   = make(map[string]int)
	maxWinners = 20
	vpsIP      string
	mu         sync.RWMutex
	proxyFile  string
	archiveDir = "proxy_bekas"
)

func InitPool(file string, ip string) {
	proxyFile = file
	vpsIP = ip
	LoadProxies()
	_ = os.MkdirAll(archiveDir, 0755)
}

func GetProxies() []string {
	mu.RLock()
	defer mu.RUnlock()
	return proxies
}

func GetTopWinners() []string {
	mu.RLock()
	defer mu.RUnlock()
	return topWinners
}

func LoadProxies() {
	mu.Lock()
	defer mu.Unlock()

	file, err := os.Open(proxyFile)
	if err != nil {
		return
	}
	defer file.Close()

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

	stat, err := os.Stat(proxyFile)
	if err == nil {
		mtime = stat.ModTime()
	}
	logger.Info("Loaded %d proxies from %s", len(list), proxyFile)
}

var mtime time.Time

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

func GetProxiesCount() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(proxies)
}

func GetStats() (int, int) {
	mu.RLock()
	defer mu.RUnlock()
	return len(proxies), len(topWinners)
}

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

func ResetWinners() {
	mu.Lock()
	defer mu.Unlock()
	topWinners = nil
	winnerMs = make(map[string]int)
}

func GetRealIP() (string, error) {
	resp, err := http.Get("https://ifconfig.me/ip")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(body)), nil
}

func GetVPSIP() string {
	mu.RLock()
	defer mu.RUnlock()
	return vpsIP
}
