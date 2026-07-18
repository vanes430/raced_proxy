package proxy

import (
	"math/rand"
	"sync"
	"time"

	"github.com/vanes430/raced_proxy/internal/logger"
)

// Bootstrap fills the winner pool by testing random proxies from the pool
// in parallel batches of 20 until maxWinners is reached or the pool is exhausted.
// testFn: a function that tests a proxy and returns (ok, latencyMs).
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

// Refill picks n random proxies, tests them in parallel, and inserts passing
// ones into the winner pool until it reaches maxWinners.
// n: how many candidates to pick. testFn: proxy test function returning (ok, ms).
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

// pickRandom selects up to n random proxies from the pool that are not in the
// skip map, not already winners, and not in cooldown.
// n: maximum candidates to return. skip: proxies to exclude.
// Returns: a shuffled slice of candidate proxy addresses.
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

// inSlice checks whether a string exists in a slice.
// slice: the string slice to search. s: the string to find.
// Returns: true if s is found in slice.
func inSlice(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
