package scanner

import (
	"sync"
	"sync/atomic"

	"raced_proxy/internal/logger"
)

func runStage1(proxiesList []string, concurrencyLimit int, timeoutMs int) []string {
	var passed []string
	var muLock sync.Mutex
	var completed int64

	sem := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup

	for _, proxyVal := range proxiesList {
		sem <- struct{}{}
		wg.Add(1)

		go func(p string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			ok := testIPLeak(p, timeoutMs)
			if ok {
				muLock.Lock()
				passed = append(passed, p)
				muLock.Unlock()
			}

			curr := atomic.AddInt64(&completed, 1)
			if curr%100 == 0 || curr == int64(len(proxiesList)) {
				muLock.Lock()
				pct := float64(curr) / float64(len(proxiesList)) * 100
				logger.Info("Stage 1 Progress: [%d/%d] %.0f%% — Passed: %d | Failed: %d",
					curr, len(proxiesList), pct, len(passed), curr-int64(len(passed)))
				muLock.Unlock()
			}
		}(proxyVal)
	}

	wg.Wait()
	return passed
}

func runStage2(proxiesList []string, concurrencyLimit int, timeoutMs int) []string {
	var passed []string
	var muLock sync.Mutex
	var completed int64

	sem := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup

	for _, proxyVal := range proxiesList {
		sem <- struct{}{}
		wg.Add(1)

		go func(p string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			ok := testTarget(p, timeoutMs)
			if ok {
				muLock.Lock()
				passed = append(passed, p)
				muLock.Unlock()
			}

			curr := atomic.AddInt64(&completed, 1)
			if curr%10 == 0 || curr == int64(len(proxiesList)) {
				muLock.Lock()
				pct := float64(curr) / float64(len(proxiesList)) * 100
				logger.Info("Stage 2 Progress: [%d/%d] %.0f%% — Passed: %d | Failed: %d",
					curr, len(proxiesList), pct, len(passed), curr-int64(len(passed)))
				muLock.Unlock()
			}
		}(proxyVal)
	}

	wg.Wait()
	return passed
}
