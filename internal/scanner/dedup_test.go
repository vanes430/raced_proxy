package scanner

import (
	"sort"
	"strconv"
	"testing"
)

// --- isCommonPort tests ---

func TestIsCommonPort(t *testing.T) {
	tests := []struct {
		port int
		want bool
	}{
		{80, true},
		{443, true},
		{8080, true},
		{3128, true},
		{8443, true},
		{1080, true},
		{9050, true},
		{10808, true},
		{12345, false},
		{65535, false},
		{0, false},
		{1, false},
		{9999, false},
	}
	for _, tt := range tests {
		t.Run("_"+strconv.Itoa(tt.port), func(t *testing.T) {
			got := isCommonPort(tt.port)
			if got != tt.want {
				t.Errorf("isCommonPort(%d) = %v, want %v", tt.port, got, tt.want)
			}
		})
	}
}

// --- dedupByIP tests ---

func TestDedupByIP_EmptyInput(t *testing.T) {
	got := dedupByIP(nil)
	if len(got) != 0 {
		t.Errorf("dedupByIP(nil) returned %d results, want 0", len(got))
	}

	got2 := dedupByIP([]CheckResult{})
	if len(got2) != 0 {
		t.Errorf("dedupByIP([]) returned %d results, want 0", len(got2))
	}
}

func TestDedupByIP_UniqueIPs(t *testing.T) {
	input := []CheckResult{
		{Proxy: "1.1.1.1:80", Ms: 100},
		{Proxy: "2.2.2.2:443", Ms: 200},
		{Proxy: "3.3.3.3:8080", Ms: 300},
	}
	got := dedupByIP(input)
	if len(got) != 3 {
		t.Errorf("dedupByIP returned %d results, want 3", len(got))
	}
}

func TestDedupByIP_SameIPDifferentPorts_KeepsLowestLatencyCommonPort(t *testing.T) {
	input := []CheckResult{
		{Proxy: "10.0.0.1:80", Ms: 150},   // common port, higher latency
		{Proxy: "10.0.0.1:443", Ms: 50},   // common port, lowest latency → should win
		{Proxy: "10.0.0.1:9999", Ms: 10},  // uncommon port, lowest latency overall
	}
	got := dedupByIP(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	// Among common ports (80, 443), 443 has lowest latency (50ms)
	if got[0].Proxy != "10.0.0.1:443" {
		t.Errorf("kept %s, want 10.0.0.1:443 (common port, lowest latency)", got[0].Proxy)
	}
}

func TestDedupByIP_SameIPSamePort_KeepsOne(t *testing.T) {
	input := []CheckResult{
		{Proxy: "10.0.0.1:8080", Ms: 200},
		{Proxy: "10.0.0.1:8080", Ms: 50},
		{Proxy: "10.0.0.1:8080", Ms: 100},
	}
	got := dedupByIP(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	// Lowest latency among same port
	if got[0].Ms != 50 {
		t.Errorf("kept latency %d ms, want 50 ms", got[0].Ms)
	}
}

func TestDedupByIP_MixedIPs(t *testing.T) {
	input := []CheckResult{
		{Proxy: "1.1.1.1:80", Ms: 100},
		{Proxy: "1.1.1.1:8080", Ms: 50},
		{Proxy: "2.2.2.2:443", Ms: 200},
	}
	got := dedupByIP(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}

	// Sort by proxy for deterministic check
	sort.Slice(got, func(i, j int) bool {
		return got[i].Proxy < got[j].Proxy
	})

	if got[0].Proxy != "1.1.1.1:8080" {
		t.Errorf("IP 1.1.1.1: got %s, want 1.1.1.1:8080 (common port, lowest latency)", got[0].Proxy)
	}
	if got[1].Proxy != "2.2.2.2:443" {
		t.Errorf("IP 2.2.2.2: got %s, want 2.2.2.2:443", got[1].Proxy)
	}
}

func TestDedupByIP_OnlyUncommonPorts(t *testing.T) {
	input := []CheckResult{
		{Proxy: "10.0.0.1:9999", Ms: 100},
		{Proxy: "10.0.0.1:12345", Ms: 50},
		{Proxy: "10.0.0.1:7777", Ms: 200},
	}
	got := dedupByIP(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	// No common ports → picks lowest latency overall
	if got[0].Ms != 50 {
		t.Errorf("kept latency %d ms, want 50 ms", got[0].Ms)
	}
}

func TestDedupByIP_InvalidFormat_Skipped(t *testing.T) {
	input := []CheckResult{
		{Proxy: "invalid-proxy", Ms: 100},
		{Proxy: "1.2.3.4:80", Ms: 50},
	}
	got := dedupByIP(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Proxy != "1.2.3.4:80" {
		t.Errorf("got %s, want 1.2.3.4:80", got[0].Proxy)
	}
}
