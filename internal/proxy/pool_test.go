package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

// resetPool zeroes out all pool-level globals for test isolation.
func resetPool() {
	mu.Lock()
	defer mu.Unlock()
	proxies = nil
	topWinners = nil
	winnerMs = make(map[string]int)
	hostIP = ""
	proxyFile = ""
}

// writeProxyFile creates a temp file with the given content and returns its path.
func writeProxyFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil { //nolint:gosec // test file
		t.Fatalf("writeProxyFile: %v", err)
	}
	return path
}

// --- LoadProxies -------------------------------------------------------

func TestLoadProxies_ValidAndInvalid(t *testing.T) {
	resetPool()
	path := writeProxyFile(t, "1.2.3.4:80\nnot_valid\n5.6.7.8:443\n# comment\n9.10.11.12:8080\n")

	InitPool(path, "1.2.3.4")

	if got := GetProxiesCount(); got != 3 {
		t.Errorf("GetProxiesCount() = %d, want 3", got)
	}
}

func TestLoadProxies_EmptyFile(t *testing.T) {
	resetPool()
	path := writeProxyFile(t, "")

	InitPool(path, "1.2.3.4")

	if got := GetProxiesCount(); got != 0 {
		t.Errorf("GetProxiesCount() = %d, want 0", got)
	}
}

func TestLoadProxies_NonexistentFile(t *testing.T) {
	resetPool()

	// Set proxyFile to a path that does not exist and call LoadProxies directly.
	mu.Lock()
	proxyFile = "/no/such/file/proxy.txt"
	mu.Unlock()

	LoadProxies()

	if got := GetProxiesCount(); got != 0 {
		t.Errorf("GetProxiesCount() = %d, want 0 for nonexistent file", got)
	}
}

func TestLoadProxies_SetsHostIP(t *testing.T) {
	resetPool()
	path := writeProxyFile(t, "10.0.0.1:8080\n")

	InitPool(path, "10.0.0.1")

	if got := GetHostIP(); got != "10.0.0.1" {
		t.Errorf("GetHostIP() = %q, want %q", got, "10.0.0.1")
	}
}

// --- GetStats ----------------------------------------------------------

func TestGetStats(t *testing.T) {
	resetPool()
	path := writeProxyFile(t, "1.1.1.1:80\n2.2.2.2:443\n3.3.3.3:8080\n")

	InitPool(path, "9.9.9.9")

	total, winners := GetStats()
	if total != 3 {
		t.Errorf("total proxies = %d, want 3", total)
	}
	if winners != 0 {
		t.Errorf("winners = %d, want 0 (no winners set)", winners)
	}
}

func TestGetStats_WithWinners(t *testing.T) {
	resetPool()

	mu.Lock()
	proxies = []string{"a:1", "b:2"}
	topWinners = []string{"x:1"}
	winnerMs["x:1"] = 50
	mu.Unlock()

	total, winners := GetStats()
	if total != 2 || winners != 1 {
		t.Errorf("GetStats() = (%d, %d), want (2, 1)", total, winners)
	}
}

// --- GetProxiesCount ---------------------------------------------------

func TestGetProxiesCount(t *testing.T) {
	resetPool()
	path := writeProxyFile(t, "1.1.1.1:80\n2.2.2.2:443\n")

	InitPool(path, "9.9.9.9")

	if got := GetProxiesCount(); got != 2 {
		t.Errorf("GetProxiesCount() = %d, want 2", got)
	}
}

// --- GetProxies snapshot -----------------------------------------------

func TestGetProxies_ReturnsCurrentData(t *testing.T) {
	resetPool()
	path := writeProxyFile(t, "1.1.1.1:80\n2.2.2.2:443\n")

	InitPool(path, "9.9.9.9")

	snap := GetProxies()
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(snap))
	}
	if snap[0] != "1.1.1.1:80" || snap[1] != "2.2.2.2:443" {
		t.Errorf("snapshot = %v, want [1.1.1.1:80, 2.2.2.2:443]", snap)
	}

	snap2 := GetProxies()
	if len(snap2) != 2 {
		t.Fatalf("second call len = %d, want 2", len(snap2))
	}
}
