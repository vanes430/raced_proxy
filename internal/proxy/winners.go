package proxy

import (
	"math/rand"
	"sort"
	"sync"
	"time"

	"raced_proxy/internal/config"
	"raced_proxy/internal/logger"
)

var (
	winnerBorn       = make(map[string]time.Time)
	winnerCooldownUntil = make(map[string]time.Time)
	winnerTTL        time.Duration
	winnerCooldown   time.Duration
	maxLatencyMs     int
)

func pickRandom(n int, skip map[string]bool) []string {
	now := time.Now()
	var cands []string
	for _, p := range proxies {
		if skip[p] {
			continue
		}
		if inSlice(topWinners, p) {
			continue
		}
		if until, ok := winnerCooldownUntil[p]; ok {
			if now.Before(until) {
				continue
			}
			delete(winnerCooldownUntil, p)
		}
		cands = append(cands, p)
	}
	rand.Shuffle(len(cands), func(i, j int) {
		cands[i], cands[j] = cands[j], cands[i]
	})
	if len(cands) > n {
		cands = cands[:n]
	}
	return cands
}

func inSlice(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func InitWinnerConfig() {
	ttlMin := config.GetEnvInt("WINNER_TTL", 0)
	if ttlMin > 0 {
		winnerTTL = time.Duration(ttlMin) * time.Minute
	}
	cooldownSec := config.GetEnvInt("WINNER_COOLDOWN", 0)
	if cooldownSec > 0 {
		winnerCooldown = time.Duration(cooldownSec) * time.Second
	}
	maxLatencyMs = config.GetEnvInt("MAX_LATENCY", 0)
	if winnerTTL > 0 || winnerCooldown > 0 {
		go startExpiryLoop()
	}
}

func startExpiryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		expireWinners()
	}
}

func expireWinners() {
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()
	var keep []string
	for _, p := range topWinners {
		if winnerTTL > 0 {
			if born, ok := winnerBorn[p]; ok && now.Sub(born) > winnerTTL {
				logger.Info("TTL expired: %s (lived %s)", p, now.Sub(born).Round(time.Second))
				delete(winnerMs, p)
				delete(winnerBorn, p)
				continue
			}
		}
		keep = append(keep, p)
	}
	if len(keep) != len(topWinners) {
		topWinners = keep
	}
}

func insertWinner(p string, ms int) {
	if maxLatencyMs > 0 && ms > maxLatencyMs {
		logger.Warn("Rejected %s (%dms > %dms max latency)", p, ms, maxLatencyMs)
		return
	}
	winnerMs[p] = ms
	winnerBorn[p] = time.Now()
	topWinners = append(topWinners, p)
	sort.Slice(topWinners, func(i, j int) bool {
		return winnerMs[topWinners[i]] < winnerMs[topWinners[j]]
	})
	if len(topWinners) > maxWinners {
		delete(winnerMs, topWinners[len(topWinners)-1])
		topWinners = topWinners[:maxWinners]
	}
}

func Bootstrap(testFn func(proxy string) (ok bool, ms int)) {
	mu.RLock()
	total := len(proxies)
	mu.RUnlock()

	skip := make(map[string]bool)

	for {
		mu.RLock()
		full := len(topWinners) >= maxWinners
		mu.RUnlock()
		if full {
			logger.Ok("Bootstrap complete: %d winners", maxWinners)
			return
		}

		cands := pickRandom(20, skip)
		if len(cands) == 0 {
			break
		}

		var wg sync.WaitGroup
		for _, p := range cands {
			skip[p] = true
			wg.Add(1)
			go func(proxy string) {
				defer wg.Done()
				ok, ms := testFn(proxy)
				if ok {
					mu.Lock()
					insertWinner(proxy, ms)
					mu.Unlock()
				}
			}(p)
		}
		wg.Wait()
		mu.RLock()
		logger.Info("Bootstrap batch done: %d/%d winners", len(topWinners), maxWinners)
		mu.RUnlock()
	}

	mu.RLock()
	count := len(topWinners)
	mu.RUnlock()
	if count == 0 {
		logger.Warn("Bootstrap: no winners found from %d proxies", total)
	} else {
		logger.Ok("Bootstrap complete: %d winners (pool exhausted)", count)
	}
}

func PickTopWinner() string {
	mu.RLock()
	defer mu.RUnlock()
	if len(topWinners) == 0 {
		return ""
	}
	return topWinners[0]
}

func RemoveWinner(p string) {
	mu.Lock()
	defer mu.Unlock()
	idx := -1
	for i, v := range topWinners {
		if v == p {
			idx = i
			break
		}
	}
	if idx >= 0 {
		topWinners = append(topWinners[:idx], topWinners[idx+1:]...)
	}
	delete(winnerMs, p)
	delete(winnerBorn, p)
	if winnerCooldown > 0 {
		winnerCooldownUntil[p] = time.Now().Add(winnerCooldown)
	}
}

func NeedRefill() bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(topWinners) <= maxWinners/2 && len(topWinners) < maxWinners
}

func Refill(n int, testFn func(proxy string) (ok bool, ms int)) {
	mu.RLock()
	remain := maxWinners - len(topWinners)
	mu.RUnlock()
	if remain <= 0 {
		return
	}

	skip := make(map[string]bool)
	cands := pickRandom(n, skip)
	var wg sync.WaitGroup
	for _, p := range cands {
		wg.Add(1)
		go func(proxy string) {
			defer wg.Done()
			ok, ms := testFn(proxy)
			if ok {
				mu.Lock()
				insertWinner(proxy, ms)
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()

	mu.RLock()
	logger.Info("Refill done: %d/%d winners", len(topWinners), maxWinners)
	mu.RUnlock()
}
