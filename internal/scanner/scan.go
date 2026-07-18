package scanner

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/vanes430/raced_proxy/internal/config"
	"github.com/vanes430/raced_proxy/internal/logger"
	"github.com/vanes430/raced_proxy/internal/proxy"
)

// realIP stores the host machine's public IP, used for IP-leak detection in test.go.
var realIP string

// RunScanner executes the full 2-stage proxy scanning pipeline:
// fetch sources, detect IP, filter rate-limited, stage1 (IP leak), stage2 (target), dedup, write.
func RunScanner() {
	targetURL := config.GetEnv("SCAN_TARGET", "https://opencode.ai/zen/v1/chat/completions")
	timeoutMs := config.GetEnvInt("REQUEST_TIMEOUT", 1500)
	outputFile := config.GetEnv("PROXY_FILE", "proxy.txt")
	concurrencyLimit := config.GetEnvInt("WORKER_COUNT", 1000)

	modelName := config.GetEnv("MODEL_NAME", "big-pickle")

	logger.Banner("PROXY SCANNER (Golang Edition)",
		fmt.Sprintf("Target:       %s", targetURL),
		fmt.Sprintf("Model:        %s", modelName),
		fmt.Sprintf("Timeout:      %dms", timeoutMs),
		fmt.Sprintf("Concurrency:  %d", concurrencyLimit),
		fmt.Sprintf("Output:       %s", outputFile),
		fmt.Sprintf("Source File: %s", config.GetEnv("SOURCE_FILE", "url-list.txt")),
	)

	logger.Section("HOST IP DETECTION")
	logger.Info("Detecting real host IP by contacting ifconfig.me...")
	ip, err := proxy.GetRealIP()
	if err == nil && ip != "" {
		realIP = ip
		logger.Ok("Host IP detected: %s", realIP)
	} else {
		logger.Info("Could not detect host IP — leak detection disabled.")
	}
	logger.Divider()

	logger.Section("FETCHING PROXY SOURCES")
	urlListFile := config.GetEnv("SOURCE_FILE", "url-list.txt")
	logger.Info("Reading proxy source URLs from %s...", urlListFile)
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
		_ = os.WriteFile(outputFile, []byte(""), 0644) //nolint:gosec // clearing output file
		return
	}
	fmt.Println()

	logger.Section("STAGE 2: TARGET ACCESSIBILITY")
	logger.Info("Testing %d proxies against %s...", len(stage1Passed), targetURL)
	stage2Start := time.Now()
	working := runStage2(stage1Passed, concurrencyLimit, timeoutMs)
	stage2Elapsed := time.Since(stage2Start)
	stage2Fail := len(stage1Passed) - len(working)
	logger.Divider()
	logger.Ok("Stage 2 Result — Passed: %d | Failed: %d | Time: %.1fs", len(working), stage2Fail, stage2Elapsed.Seconds())
	if len(working) == 0 {
		logger.Fail("All proxies blocked or inaccessible.")
		_ = os.WriteFile(outputFile, []byte(""), 0644) //nolint:gosec // clearing output file
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
	_ = os.WriteFile(outputFile, buf.Bytes(), 0644) //nolint:gosec // writing proxy output file

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

		workingMap := make(map[string]bool)
		for _, w := range results {
			workingMap[w.Proxy] = true
		}
		printSourceStats(sources, workingMap)
	}

	fmt.Println()
	logger.Divider()
	logger.Info("Pipeline summary: %d fetched → %d stage1 → %d stage2 → %d final",
		len(allProxies), len(stage1Passed), len(results), len(results))
	logger.Divider()
}
