package proxy

import (
	"math/rand"
	"sort"
	"sync"

	"raced_proxy/internal/logger"
)

func pickRandom(n int, skip map[string]bool) []string {
	var cands []string
	for _, p := range proxies {
		if skip[p] {
			continue
		}
		if inSlice(topWinners, p) {
			continue
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

func insertWinner(p string, ms int) {
	winnerMs[p] = ms
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

	for {
		mu.RLock()
		full := len(topWinners) >= maxWinners
		mu.RUnlock()
		if full {
			logger.Ok("Bootstrap complete: %d winners", maxWinners)
			return
		}

		skip := make(map[string]bool)
		cands := pickRandom(20, skip)
		if len(cands) == 0 {
			break
		}

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
