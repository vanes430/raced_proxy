package scanner

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"raced_proxy/internal/config"
	"raced_proxy/internal/logger"
)

// generateDefaultURLList creates an empty url-list.txt file with a comment header
// when the source file is missing or empty.
// path: file path to write the default URL list.
func generateDefaultURLList(path string) {
	var buf strings.Builder
	buf.WriteString("# Proxy source URLs (one per line)\n")
	buf.WriteString("# Lines starting with # are ignored\n")
	_ = os.WriteFile(path, []byte(buf.String()), 0644)
	logger.Warn("Generated empty %s — add your proxy source URLs", path)
}

// fetchAllProxies reads source URLs from the URL list file, fetches proxy lists
// from each source in parallel, and deduplicates across all sources.
// Returns: a deduplicated list of all proxy addresses, and the per-source metadata.
func fetchAllProxies() ([]string, []SourceData) {
	urlListFile := config.GetEnv("SOURCE_FILE", "url-list.txt")

	if info, err := os.Stat(urlListFile); err != nil || info.Size() == 0 {
		logger.Warn("%s missing or empty, generating default source list", urlListFile)
		generateDefaultURLList(urlListFile)
	}

	file, err := os.Open(urlListFile)
	if err != nil {
		fmt.Printf("✗ Failed to open %s: %v\n", urlListFile, err)
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

	logger.Info("Found %d proxy source URLs in %s", len(sources), urlListFile)
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
