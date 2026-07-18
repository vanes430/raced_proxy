package scanner

import (
	"fmt"

	"raced_proxy/internal/logger"
)

// CheckResult holds a proxy address and its measured latency in milliseconds.
type CheckResult struct {
	Proxy string
	Ms    int
}

// SourceData holds metadata for a single proxy source URL and its fetched proxies.
type SourceData struct {
	Name    string
	URL     string
	Proxies []string
	Fetched int
}

// printSourceStats prints per-source success rates for proxies that survived all stages.
// sources: the list of source data entries from the fetch phase.
// workingMap: set of proxy addresses that passed all validation stages.
func printSourceStats(sources []SourceData, workingMap map[string]bool) {
	fmt.Printf("\n%s  Source Success Rates%s\n\n", logger.Bold, logger.Reset)
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
