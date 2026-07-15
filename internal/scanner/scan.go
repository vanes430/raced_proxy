package scanner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	maxLatencyMs := config.GetEnvInt("MAX_LATENCY", 1500)
	outputFile := config.GetEnv("OUTPUT", "proxy.txt")
	concurrencyLimit := config.GetEnvInt("CONCURRENCY", 1000)

	logger.Banner("PROXY SCANNER (Golang Edition)",
		fmt.Sprintf("Target:       %s", targetURL),
		fmt.Sprintf("Timeout:      %dms", timeoutMs),
		fmt.Sprintf("Max Latency:  %dms", maxLatencyMs),
		fmt.Sprintf("Concurrency:  %d", concurrencyLimit),
		fmt.Sprintf("Output:       %s", outputFile),
	)

	// ---- VPS IP Detection ----
	logger.Section("VPS IP DETECTION")
	logger.Info("Detecting real VPS IP by contacting ifconfig.me...")
	ip, err := proxy.GetRealIP()
	if err == nil && ip != "" {
		realIP = ip
		logger.Ok("VPS IP detected: %s", realIP)
		logger.Info("Proxies that leak this IP will be filtered out in Stage 1")
	} else {
		logger.Info("Could not detect VPS IP — leak detection disabled.")
		logger.Info("Reason: %v", err)
	}
	logger.Divider()

	// ---- Fetch Proxies ----
	logger.Section("FETCHING PROXY SOURCES")
	logger.Info("Reading proxy source URLs from url-list.txt...")
	allProxies, sources := fetchAllProxies()
	if len(allProxies) == 0 {
		logger.Fail("No proxies fetched from any source.")
		logger.Info("Check url-list.txt and network connectivity.")
		return
	}
	logger.Ok("Total unique proxies collected: %d", len(allProxies))
	logger.Info("Proxy sources: %d sources contributed to the pool", len(sources))
	logger.Divider()

	start := time.Now()

	// ---- Stage 1: IP Leak Detection ----
	logger.Section("STAGE 1: IP LEAK DETECTION")
	logger.Info("Checking %d proxies against ifconfig.me via CONNECT + TLS...", len(allProxies))
	logger.Info("Goal: Eliminate transparent proxies that expose the VPS IP")
	stage1Start := time.Now()
	stage1Passed := runStage1(allProxies, concurrencyLimit, timeoutMs)
	stage1Elapsed := time.Since(stage1Start)
	stage1Fail := len(allProxies) - len(stage1Passed)
	logger.Divider()
	logger.Ok("Stage 1 Result — Passed: %d | Failed: %d | Time: %.1fs", len(stage1Passed), stage1Fail, stage1Elapsed.Seconds())
	if len(stage1Passed) == 0 {
		logger.Fail("All proxies failed IP leak detection. No usable proxies.")
		_ = os.WriteFile(outputFile, []byte(""), 0644)
		return
	}
	fmt.Println()

	// ---- Stage 2: Target Accessibility ----
	logger.Section("STAGE 2: TARGET ACCESSIBILITY")
	logger.Info("Testing %d proxies against opencode.ai...", len(stage1Passed))
	logger.Info("Goal: Drop proxies that return 403/429 or are blocked by the target")
	stage2Start := time.Now()
	stage2Passed := runStage2(stage1Passed, concurrencyLimit, timeoutMs)
	stage2Elapsed := time.Since(stage2Start)
	stage2Fail := len(stage1Passed) - len(stage2Passed)
	logger.Divider()
	logger.Ok("Stage 2 Result — Passed: %d | Failed: %d | Time: %.1fs", len(stage2Passed), stage2Fail, stage2Elapsed.Seconds())
	if len(stage2Passed) == 0 {
		logger.Fail("All proxies blocked or inaccessible to the target.")
		_ = os.WriteFile(outputFile, []byte(""), 0644)
		return
	}
	fmt.Println()

	// ---- Stage 3: Stability Check ----
	logger.Section("STAGE 3: STABILITY CHECK")
	logger.Info("Re-testing %d proxies after 100ms delay...", len(stage2Passed))
	logger.Info("Goal: Filter out single-use / unstable proxies")
	stage3Start := time.Now()
	working := runStage3(stage2Passed, concurrencyLimit, timeoutMs, maxLatencyMs)
	stage3Elapsed := time.Since(stage3Start)
	stage3Fail := len(stage2Passed) - len(working)
	logger.Divider()
	logger.Ok("Stage 3 Result — Passed: %d | Failed: %d | Time: %.1fs", len(working), stage3Fail, stage3Elapsed.Seconds())
	fmt.Println()

	// ---- Dedup ----
	working = dedupByIP(working)

	sort.Slice(working, func(i, j int) bool {
		return working[i].Ms < working[j].Ms
	})

	elapsed := time.Since(start)

	var buf bytes.Buffer
	for _, w := range working {
		buf.WriteString(w.Proxy + "\n")
	}
	_ = os.WriteFile(outputFile, buf.Bytes(), 0644)

	// ---- Final Summary ----
	logger.Section("SCAN COMPLETE")
	fmt.Println()
	if len(working) == 0 {
		logger.Fail("No working proxies found after all 3 stages.")
	} else {
		sum := 0
		for _, w := range working {
			sum += w.Ms
		}
		avg := sum / len(working)
		logger.Ok("Working proxies: %d | Average latency: %dms | Total time: %.1fs", len(working), avg, elapsed.Seconds())
		logger.Ok("Results written to: %s", outputFile)

		fmt.Printf("\n%s  Source Success Rates%s\n\n", logger.Bold, logger.Reset)
		workingMap := make(map[string]bool)
		for _, w := range working {
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

	// ---- Pipeline Summary ----
	fmt.Println()
	logger.Divider()
	logger.Info("Pipeline summary: %d fetched → %d stage1 → %d stage2 → %d stage3 → %d final",
		len(allProxies), len(stage1Passed), len(stage2Passed), len(working), len(working))
	logger.Divider()
}

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

func runStage3(proxiesList []string, concurrencyLimit int, timeoutMs int, maxLatencyMs int) []CheckResult {
	var passed []CheckResult
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

			time.Sleep(100 * time.Millisecond)

			start := time.Now()
			ok := testTarget(p, timeoutMs)
			if ok {
				ms := int(time.Since(start).Milliseconds())
				if ms <= maxLatencyMs {
					muLock.Lock()
					passed = append(passed, CheckResult{Proxy: p, Ms: ms})
					muLock.Unlock()
				}
			}

			curr := atomic.AddInt64(&completed, 1)
			if curr%10 == 0 || curr == int64(len(proxiesList)) {
				muLock.Lock()
				pct := float64(curr) / float64(len(proxiesList)) * 100
				logger.Info("Stage 3 Progress: [%d/%d] %.0f%% — Passed: %d | Failed: %d",
					curr, len(proxiesList), pct, len(passed), curr-int64(len(passed)))
				muLock.Unlock()
			}
		}(proxyVal)
	}

	wg.Wait()
	return passed
}

func dedupByIP(results []CheckResult) []CheckResult {
	type entry struct {
		proxy      CheckResult
		ip         string
		port       int
		commonPort bool
	}

	var entries []entry
	for _, r := range results {
		parts := strings.Split(r.Proxy, ":")
		if len(parts) != 2 {
			continue
		}
		port, _ := strconv.Atoi(parts[1])
		entries = append(entries, entry{
			proxy:      r,
			ip:         parts[0],
			port:       port,
			commonPort: isCommonPort(port),
		})
	}

	ipGroups := make(map[string][]entry)
	for _, e := range entries {
		ipGroups[e.ip] = append(ipGroups[e.ip], e)
	}

	var deduped []CheckResult
	dupCount := 0
	for _, group := range ipGroups {
		if len(group) == 1 {
			deduped = append(deduped, group[0].proxy)
			continue
		}

		var common []entry
		for _, e := range group {
			if e.commonPort {
				common = append(common, e)
			}
		}

		if len(common) > 0 {
			best := common[0]
			for _, e := range common[1:] {
				if e.proxy.Ms < best.proxy.Ms {
					best = e
				}
			}
			deduped = append(deduped, best.proxy)
		} else {
			best := group[0]
			for _, e := range group[1:] {
				if e.proxy.Ms < best.proxy.Ms {
					best = e
				}
			}
			deduped = append(deduped, best.proxy)
		}
		dupCount += len(group) - 1
	}

	logger.Ok("Dedup by IP: %d → %d (removed %d same-IP duplicates)", len(results), len(deduped), dupCount)
	return deduped
}

func isCommonPort(port int) bool {
	switch port {
	case 80, 443, 8080, 8443, 3128, 1080, 9050, 10808, 10809, 20170, 20171, 20172, 20173:
		return true
	}
	return false
}

func testIPLeak(proxyStr string, timeoutMs int) bool {
	dialer := &net.Dialer{
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		KeepAlive: 0,
	}

	conn, err := dialer.Dial("tcp", proxyStr)
	if err != nil {
		return false
	}
	defer conn.Close()

	req := "CONNECT ifconfig.me:443 HTTP/1.1\r\nHost: ifconfig.me:443\r\n\r\n"
	_, err = conn.Write([]byte(req))
	if err != nil {
		return false
	}

	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	n, err := conn.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "200") {
		return false
	}

	tlsConn := tls.Client(conn, config.GetTLSConfig("ifconfig.me"))
	defer tlsConn.Close()

	err = tlsConn.HandshakeContext(context.Background())
	if err != nil {
		return false
	}

	getReq := "GET /ip HTTP/1.1\r\nHost: ifconfig.me\r\nConnection: close\r\n\r\n"
	_, err = tlsConn.Write([]byte(getReq))
	if err != nil {
		return false
	}

	var respBuf bytes.Buffer
	_ = tlsConn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	_, _ = io.Copy(&respBuf, tlsConn)

	body := respBuf.String()
	ipRegex := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)
	match := ipRegex.FindString(body)

	return match != "" && (realIP == "" || match != realIP)
}

func testTarget(proxyStr string, timeoutMs int) bool {
	dialer := &net.Dialer{
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		KeepAlive: 0,
	}

	conn, err := dialer.Dial("tcp", proxyStr)
	if err != nil {
		return false
	}
	defer conn.Close()

	req := "CONNECT opencode.ai:443 HTTP/1.1\r\nHost: opencode.ai:443\r\n\r\n"
	_, err = conn.Write([]byte(req))
	if err != nil {
		return false
	}

	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	n, err := conn.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "200") {
		return false
	}

	tlsConn := tls.Client(conn, config.GetTLSConfig("opencode.ai"))
	defer tlsConn.Close()

	err = tlsConn.HandshakeContext(context.Background())
	if err != nil {
		return false
	}

	getReq := "GET /zen/v1/chat/completions HTTP/1.1\r\nHost: opencode.ai\r\nConnection: close\r\n\r\n"
	_, err = tlsConn.Write([]byte(getReq))
	if err != nil {
		return false
	}

	var respBuf bytes.Buffer
	_ = tlsConn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	_, _ = io.Copy(&respBuf, tlsConn)

	body := respBuf.String()
	m := regexp.MustCompile(`HTTP/\d\.\d\s+(\d+)`).FindStringSubmatch(body)
	if len(m) < 2 {
		return false
	}
	status, _ := strconv.Atoi(m[1])

	return status > 0 && status != 403 && status != 429 &&
		!strings.Contains(body, "Rate limit") &&
		!strings.Contains(body, "FreeUsageLimitError")
}

func fetchAllProxies() ([]string, []SourceData) {
	file, err := os.Open("url-list.txt")
	if err != nil {
		fmt.Printf("✗ Failed to open url-list.txt: %v\n", err)
		return nil, nil
	}
	defer file.Close()

	var sources []string
	scannerObj := bufio.NewScanner(file)
	for scannerObj.Scan() {
		l := strings.TrimSpace(scannerObj.Text())
		if l != "" && !strings.HasPrefix(l, "#") {
			sources = append(sources, l)
		}
	}

	logger.Info("Found %d proxy source URLs in url-list.txt", len(sources))
	fmt.Println()

	var results []SourceData
	allSet := make(map[string]bool)

	client := &http.Client{Timeout: 15 * time.Second}
	var muLock sync.Mutex
	var wg sync.WaitGroup

	for _, url := range sources {
		wg.Add(1)
		go func(targetURL string) {
			defer wg.Done()
			name := targetURL
			name = strings.ReplaceAll(name, "https://", "")
			name = strings.ReplaceAll(name, "http://", "")
			name = strings.ReplaceAll(name, "raw.githubusercontent.com/", "gh:")
			name = strings.ReplaceAll(name, "github.com/", "gh:")
			name = strings.ReplaceAll(name, "/raw/refs/heads/main/", "/")
			name = strings.ReplaceAll(name, "/raw/refs/heads/master/", "/")
			name = strings.ReplaceAll(name, "/raw/", "/")
			if len(name) > 40 {
				name = name[:40]
			}

			fetchStart := time.Now()
			resp, err := client.Get(targetURL)
			fetchElapsed := time.Since(fetchStart)
			if err != nil {
				logger.Warn("Fetch failed: %s — %v (%.1fs)", name, err, fetchElapsed.Seconds())
				return
			}
			defer resp.Body.Close()

			var proxiesList []string
			scan := bufio.NewScanner(resp.Body)
			ipPortRegex := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+:\d+`)

			for scan.Scan() {
				line := strings.TrimSpace(scan.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				match := ipPortRegex.FindString(line)
				if match != "" {
					proxiesList = append(proxiesList, match)
				}
			}

			muLock.Lock()
			results = append(results, SourceData{
				Name:    name,
				URL:     targetURL,
				Proxies: proxiesList,
				Fetched: len(proxiesList),
			})
			for _, p := range proxiesList {
				allSet[p] = true
			}
			fmt.Printf("  %s✓%s %-40s %d proxies (%.1fs)%s\n",
				logger.Green, logger.Reset, name, len(proxiesList), fetchElapsed.Seconds(), logger.Reset)
			muLock.Unlock()
		}(url)
	}

	wg.Wait()
	fmt.Println()

	var allList []string
	for p := range allSet {
		allList = append(allList, p)
	}

	return allList, results
}
