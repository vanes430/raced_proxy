package proxy

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"raced_proxy/internal/logger"
)

var (
	proxies   []string
	vpsIP     string
	mu        sync.RWMutex
	failures  = make(map[string]int)
	winners   = make(map[string]int)
	usage     = make(map[string]int)
	cooldown  = make(map[string]int)
	mtime     time.Time
	proxyFile string
)

func InitPool(file string, ip string) {
	proxyFile = file
	vpsIP = ip
	LoadProxies()
}

func GetProxies() []string {
	mu.RLock()
	defer mu.RUnlock()
	return proxies
}

func LoadProxies() {
	mu.Lock()
	defer mu.Unlock()

	file, err := os.Open(proxyFile)
	if err != nil {
		return
	}
	defer file.Close()

	var list []string
	scan := bufio.NewScanner(file)
	for scan.Scan() {
		l := strings.TrimSpace(scan.Text())
		if regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+:\d+$`).MatchString(l) {
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

func PickProxies(n int, exclude map[string]bool) []string {
	mu.RLock()
	defer mu.RUnlock()

	var alive []string
	for _, p := range proxies {
		if exclude[p] {
			continue
		}
		if failures[p] >= 5 {
			continue
		}
		if cooldown[p] > 0 {
			continue
		}
		alive = append(alive, p)
	}

	pool := alive
	if len(pool) == 0 {
		for _, p := range proxies {
			if !exclude[p] {
				pool = append(pool, p)
			}
		}
	}

	if len(pool) == 0 {
		return nil
	}

	sortList := append([]string{}, pool...)
	sort.Slice(sortList, func(i, j int) bool {
		return winners[sortList[i]] > winners[sortList[j]]
	})

	limit := n
	if len(sortList) < limit {
		limit = len(sortList)
	}
	top := sortList[:limit]

	pinCount := int(math.Ceil(float64(len(top)) * 0.2))
	pinned := top[:pinCount]
	rest := top[pinCount:]

	rand.Shuffle(len(rest), func(i, j int) {
		rest[i], rest[j] = rest[j], rest[i]
	})

	return append(pinned, rest...)
}

func RemoveSlowProxies(candidates []string, attempts int, totalMs int, maxLatency int) {
	if totalMs < maxLatency {
		return
	}
	if attempts <= 1 {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	slow := candidates[:attempts-1]
	for _, p := range slow {
		failures[p] += 2
	}
	logger.Warn("Slow proxies penalized: %d proxies (+2 failures each)", len(slow))
	logger.Warn("Total race time: %dms (max: %dms) — proxies slower than winner penalized", totalMs, maxLatency)
}

func RecordWin(proxyStr string, attempts int, winnerTTL int, winnerCooldown int) {
	mu.Lock()
	defer mu.Unlock()

	bonus := 50 - attempts + 1
	if bonus < 1 {
		bonus = 1
	} else if bonus > 10 {
		bonus = 10
	}

	winners[proxyStr] += bonus
	if winners[proxyStr] > 100 {
		winners[proxyStr] = 100
	}

	logger.Info("Winner score update: %s +%d points (total: %d)", proxyStr, bonus, winners[proxyStr])

	for p, score := range winners {
		if p != proxyStr {
			prev := winners[p]
			winners[p] = int(float64(score) * 0.5)
			logger.Info("Score decay: %s %d → %d (halved)", p, prev, winners[p])
		}
	}

	usage[proxyStr]++
	logger.Info("Usage count: %s used %d/%d times", proxyStr, usage[proxyStr], winnerTTL)
	if usage[proxyStr] >= winnerTTL {
		usage[proxyStr] = 0
		cooldown[proxyStr] = winnerCooldown
		logger.Warn("Winner cooldown: %s reached TTL → cooling for %d requests", proxyStr, winnerCooldown)
	}
}

func RecordFail(proxyStr string) {
	mu.Lock()
	defer mu.Unlock()
	failures[proxyStr]++
	logger.Warn("Failure recorded: %s (total: %d)", proxyStr, failures[proxyStr])
}

func TickCooldowns() {
	mu.Lock()
	defer mu.Unlock()
	for p, left := range cooldown {
		if left <= 1 {
			delete(cooldown, p)
		} else {
			cooldown[p] = left - 1
		}
	}
}

var (
	persistCh = make(chan struct{}, 1)
	persistMu sync.Mutex
)

func init() {
	go persistLoop()
}

func persistLoop() {
	for range persistCh {
		time.Sleep(200 * time.Millisecond) // debounce: tunggu 200ms sebelum nulis
		mu.RLock()
		var buf bytes.Buffer
		for _, p := range proxies {
			buf.WriteString(p + "\n")
		}
		f := proxyFile
		mu.RUnlock()
		_ = os.WriteFile(f, buf.Bytes(), 0644)
	}
}

func triggerPersist() {
	select {
	case persistCh <- struct{}{}:
	default: // already queued
	}
}

func DeleteProxy(proxyStr string) {
	mu.Lock()
	// Hapus pake map biar O(1)
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
	delete(failures, proxyStr)
	delete(winners, proxyStr)
	delete(usage, proxyStr)
	delete(cooldown, proxyStr)
	mu.Unlock()

	triggerPersist() // async, gak blocking
}

func GetStats() (int, int, int, int) {
	mu.RLock()
	defer mu.RUnlock()
	total := len(proxies)
	banned := 0
	cooling := 0
	active := 0
	for _, p := range proxies {
		if failures[p] >= 5 {
			banned++
		} else if cooldown[p] > 0 {
			cooling++
		} else {
			active++
		}
	}
	return total, active, cooling, banned
}

func PrintTopWinners(limit int) {
	mu.RLock()
	defer mu.RUnlock()

	type item struct {
		p string
		s int
	}
	var list []item
	for p, s := range winners {
		list = append(list, item{p, s})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].s > list[j].s
	})

	if len(list) < limit {
		limit = len(list)
	}

	fmt.Println("")
	for i := 0; i < limit; i++ {
		bar := logger.Cyan + strings.Repeat("█", int(math.Min(20, float64(list[i].s/5)))) + logger.Reset
		fmt.Printf("  %2d. %-21s %s %d\n", i+1, list[i].p, bar, list[i].s)
	}
	fmt.Println("")
}

func ResetStats() {
	mu.Lock()
	defer mu.Unlock()
	failures = make(map[string]int)
	winners = make(map[string]int)
	usage = make(map[string]int)
	cooldown = make(map[string]int)
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
