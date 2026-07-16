package scanner

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"time"

	"raced_proxy/internal/config"
	"raced_proxy/internal/logger"
	"raced_proxy/internal/proxy"
)

type CheckResult struct {
	Proxy string
	Ms    int
}

type SourceData struct {
	Name    string
	URL     string
	Proxies []string
	Fetched int
}

var realIP string

func RunScanner() {
	targetURL := "https://opencode.ai/zen/v1/chat/completions"
	timeoutMs := config.GetEnvInt("TIMEOUT", 1500)
	outputFile := config.GetEnv("OUTPUT", "proxy.txt")
	concurrencyLimit := config.GetEnvInt("CONCURRENCY", 1000)

	logger.Banner("PROXY SCANNER (Golang Edition)",
		fmt.Sprintf("Target:       %s", targetURL),
		fmt.Sprintf("Timeout:      %dms", timeoutMs),
		fmt.Sprintf("Concurrency:  %d", concurrencyLimit),
		fmt.Sprintf("Output:       %s", outputFile),
	)

	logger.Section("VPS IP DETECTION")
	logger.Info("Detecting real VPS IP by contacting ifconfig.me...")
	ip, err := proxy.GetRealIP()
	if err == nil && ip != "" {
		realIP = ip
		logger.Ok("VPS IP detected: %s", realIP)
	} else {
		logger.Info("Could not detect VPS IP — leak detection disabled.")
	}
	logger.Divider()

	logger.Section("FETCHING PROXY SOURCES")
	logger.Info("Reading proxy source URLs from url-list.txt...")
	allProxies, sources := fetchAllProxies()
	if len(allProxies) == 0 {
		logger.Fail("No proxies fetched from any source.")
		return
	}
	logger.Ok("Total unique proxies collected: %d", len(allProxies))

	var filtered []string
	for _, p := range allProxies {
		if proxy.IsRateLimitedToday(p) {
			continue
		}
		filtered = append(filtered, p)
	}
	skipped := len(allProxies) - len(filtered)
	if skipped > 0 {
		logger.Info("Skipped %d proxies rate-limited today", skipped)
	}
	allProxies = filtered

	logger.Info("Proxy sources: %d sources contributed to the pool", len(sources))
	logger.Divider()

	start := time.Now()

	logger.Section("STAGE 1: IP LEAK DETECTION")
	logger.Info("Checking %d proxies against ifconfig.me...", len(allProxies))
	stage1Start := time.Now()
	stage1Passed := runStage1(allProxies, concurrencyLimit, timeoutMs)
	stage1Elapsed := time.Since(stage1Start)
	stage1Fail := len(allProxies) - len(stage1Passed)
	logger.Divider()
	logger.Ok("Stage 1 Result — Passed: %d | Failed: %d | Time: %.1fs", len(stage1Passed), stage1Fail, stage1Elapsed.Seconds())
	if len(stage1Passed) == 0 {
		logger.Fail("All proxies failed IP leak detection.")
		_ = os.WriteFile(outputFile, []byte(""), 0644)
		return
	}
	fmt.Println()

	logger.Section("STAGE 2: TARGET ACCESSIBILITY")
	logger.Info("Testing %d proxies against opencode.ai...", len(stage1Passed))
	stage2Start := time.Now()
	working := runStage2(stage1Passed, concurrencyLimit, timeoutMs)
	stage2Elapsed := time.Since(stage2Start)
	stage2Fail := len(stage1Passed) - len(working)
	logger.Divider()
	logger.Ok("Stage 2 Result — Passed: %d | Failed: %d | Time: %.1fs", len(working), stage2Fail, stage2Elapsed.Seconds())
	if len(working) == 0 {
		logger.Fail("All proxies blocked or inaccessible.")
		_ = os.WriteFile(outputFile, []byte(""), 0644)
		return
	}
	fmt.Println()

	results := make([]CheckResult, 0, len(working))
	for _, p := range working {
		results = append(results, CheckResult{Proxy: p})
	}
	results = dedupByIP(results)

	sort.Slice(results, func(i, j int) bool {
		return results[i].Ms < results[j].Ms
	})

	elapsed := time.Since(start)

	var buf bytes.Buffer
	for _, w := range results {
		buf.WriteString(w.Proxy + "\n")
	}
	_ = os.WriteFile(outputFile, buf.Bytes(), 0644)

	logger.Section("SCAN COMPLETE")
	fmt.Println()
	if len(results) == 0 {
		logger.Fail("No working proxies found after scanning.")
	} else {
		sum := 0
		for _, w := range results {
			sum += w.Ms
		}
		avg := sum / len(results)
		logger.Ok("Working proxies: %d | Average latency: %dms | Total time: %.1fs", len(results), avg, elapsed.Seconds())
		logger.Ok("Results written to: %s", outputFile)

		fmt.Printf("\n%s  Source Success Rates%s\n\n", logger.Bold, logger.Reset)
		workingMap := make(map[string]bool)
		for _, w := range results {
			workingMap[w.Proxy] = true
		}
		for _, src := range sources {
			success := 0
			for _, p := range src.Proxies {
				if workingMap[p] {
					success++
				}
			}
			pct := 0.0
			if src.Fetched > 0 {
				pct = (float64(success) / float64(src.Fetched)) * 100
			}
			color := logger.Red
			if pct >= 5 {
				color = logger.Green
			} else if pct >= 1 {
				color = logger.Yellow
			}
			fmt.Printf("  %-30s %d/%d (%s%.1f%%%s)\n", src.Name, success, src.Fetched, color, pct, logger.Reset)
		}
	}

	fmt.Println()
	logger.Divider()
	logger.Info("Pipeline summary: %d fetched → %d stage1 → %d stage2 → %d final",
		len(allProxies), len(stage1Passed), len(results), len(results))
	logger.Divider()
}
