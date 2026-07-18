package proxy

import (
	"sort"
	"time"

	"raced_proxy/internal/config"
	"raced_proxy/internal/logger"
)

// winnerBorn records the insertion timestamp of each winner for TTL expiry.
var winnerBorn = make(map[string]time.Time)

// winnerCooldownUntil tracks when a removed winner may be retried.
var winnerCooldownUntil = make(map[string]time.Time)

// winnerTTL is the time-to-live duration for winners (0 = disabled).
var winnerTTL time.Duration

// winnerCooldown is the cooldown before retrying a failed winner (0 = disabled).
var winnerCooldown time.Duration

// maxLatencyMs is the maximum allowed latency in ms; 0 = no limit.
var maxLatencyMs int

// InitWinnerConfig reads WINNER_TTL, WINNER_COOLDOWN, and MAX_LATENCY
// from environment and starts the expiry loop if TTL or cooldown is enabled.
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

// startExpiryLoop ticks every 30 seconds and removes expired winners.
func startExpiryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		expireWinners()
	}
}

// expireWinners removes winners whose TTL has elapsed. Caller need not hold mu.
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

// insertWinner adds a proxy to the winners list sorted by latency.
// p: proxy address. ms: latency in milliseconds.
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

// RemoveWinner removes a proxy from winners, clears its latency data,
// and sets a cooldown before it can be retried (if cooldown > 0).
// p: proxy address to remove.
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

// PickTopWinner returns the fastest available winner proxy address.
// Returns: proxy address string, or empty string if no winners exist.
func PickTopWinner() string {
	mu.RLock()
	defer mu.RUnlock()
	if len(topWinners) == 0 {
		return ""
	}
	return topWinners[0]
}

// NeedRefill reports whether the winner pool needs more proxies.
// Returns: true when winners are at or below half capacity.
func NeedRefill() bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(topWinners) <= maxWinners/2 && len(topWinners) < maxWinners
}
