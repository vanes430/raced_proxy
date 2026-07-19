package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func resetArchiveState(t *testing.T) {
	t.Helper()
	mu.Lock()
	proxies = nil
	topWinners = nil
	winnerMs = make(map[string]int)
	mu.Unlock()

	origArchiveDir := archiveDir
	t.Cleanup(func() {
		archiveDir = origArchiveDir
	})
}

func TestIsRateLimitedToday_NoArchive(t *testing.T) {
	resetArchiveState(t)
	archiveDir = t.TempDir()

	if IsRateLimitedToday("1.2.3.4:8080") {
		t.Error("expected false when no archive file exists")
	}
}

func TestIsRateLimitedToday_TrueAfterArchive(t *testing.T) {
	resetArchiveState(t)
	archiveDir = t.TempDir()

	// Create today's archive file with a proxy entry
	today := "2026-07-19.txt"
	path := filepath.Join(archiveDir, today)
	if err := os.WriteFile(path, []byte("10.0.0.1:3128\n"), 0644); err != nil { //nolint:gosec // test file
		t.Fatalf("setup: %v", err)
	}

	if !IsRateLimitedToday("10.0.0.1:3128") {
		t.Error("expected true for archived proxy")
	}
}

func TestIsRateLimitedToday_FalseForDifferentProxy(t *testing.T) {
	resetArchiveState(t)
	archiveDir = t.TempDir()

	today := "2026-07-19.txt"
	path := filepath.Join(archiveDir, today)
	if err := os.WriteFile(path, []byte("10.0.0.1:3128\n"), 0644); err != nil { //nolint:gosec // test file
		t.Fatalf("setup: %v", err)
	}

	if IsRateLimitedToday("10.0.0.2:80") {
		t.Error("expected false for a proxy not in the archive")
	}
}

func TestArchiveRateLimited_CreatesFileAndAppends(t *testing.T) {
	resetArchiveState(t)
	archiveDir = t.TempDir()
	// Set proxyFile to a temp file so triggerPersist doesn't panic
	proxyFile = filepath.Join(t.TempDir(), "proxy.txt")
	_ = os.WriteFile(proxyFile, []byte(""), 0644) //nolint:gosec // test file

	ArchiveRateLimited("192.168.1.1:8080")

	// Check the archive file was created and contains the proxy
	today := "2026-07-19.txt"
	path := filepath.Join(archiveDir, today)
	data, err := os.ReadFile(path) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("archive file not created: %v", err)
	}
	if got := string(data); got != "192.168.1.1:8080\n" {
		t.Errorf("archive content = %q, want %q", got, "192.168.1.1:8080\n")
	}

	// Also verify proxy was removed from the pool
	mu.RLock()
	for _, p := range proxies {
		if p == "192.168.1.1:8080" {
			t.Error("proxy should have been removed from proxies list")
		}
	}
	mu.RUnlock()
}

func TestDeleteProxy_RemovesFromProxiesAndWinners(t *testing.T) {
	resetArchiveState(t)
	archiveDir = t.TempDir()
	proxyFile = filepath.Join(t.TempDir(), "proxy.txt")
	_ = os.WriteFile(proxyFile, []byte(""), 0644) //nolint:gosec // test file

	// Populate state
	mu.Lock()
	proxies = []string{"10.0.0.1:80", "10.0.0.2:443", "10.0.0.3:3128"}
	topWinners = []string{"10.0.0.1:80", "10.0.0.2:443"}
	winnerMs["10.0.0.1:80"] = 100
	winnerMs["10.0.0.2:443"] = 200
	mu.Unlock()

	DeleteProxy("10.0.0.1:80")

	mu.RLock()
	defer mu.RUnlock()

	// Check proxies list
	for _, p := range proxies {
		if p == "10.0.0.1:80" {
			t.Error("proxy should be removed from proxies list")
		}
	}
	if len(proxies) != 2 {
		t.Errorf("proxies len = %d, want 2", len(proxies))
	}

	// Check topWinners list
	for _, p := range topWinners {
		if p == "10.0.0.1:80" {
			t.Error("proxy should be removed from topWinners list")
		}
	}
	if len(topWinners) != 1 {
		t.Errorf("topWinners len = %d, want 1", len(topWinners))
	}

	// Check winnerMs
	if _, ok := winnerMs["10.0.0.1:80"]; ok {
		t.Error("proxy should be removed from winnerMs map")
	}
}
