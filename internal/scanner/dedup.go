package scanner

import (
	"strconv"
	"strings"

	"github.com/vanes430/raced_proxy/internal/logger"
)

// dedupByIP removes duplicate proxies sharing the same IP, keeping the one with
// the lowest latency (preferring common ports like 80, 443, 8080, etc.).
// results: slice of CheckResult with proxy addresses and latencies.
// Returns: deduplicated slice with one entry per unique IP.
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

		var best entry
		if len(common) > 0 {
			best = common[0]
			for _, e := range common[1:] {
				if e.proxy.Ms < best.proxy.Ms {
					best = e
				}
			}
		} else {
			best = group[0]
			for _, e := range group[1:] {
				if e.proxy.Ms < best.proxy.Ms {
					best = e
				}
			}
		}
		deduped = append(deduped, best.proxy)
		dupCount += len(group) - 1
	}

	logger.Ok("Dedup by IP: %d → %d (removed %d same-IP duplicates)", len(results), len(deduped), dupCount)
	return deduped
}

// isCommonPort checks whether a port number is a commonly used proxy port.
// port: TCP port number to check.
// Returns: true if the port is in the well-known proxy port set.
func isCommonPort(port int) bool {
	switch port {
	case 80, 443, 8080, 8443, 3128, 1080, 9050, 10808, 10809, 20170, 20171, 20172, 20173:
		return true
	}
	return false
}
