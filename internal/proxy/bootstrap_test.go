package proxy

import (
	"sync"
	"testing"
	"time"
)

func resetBootstrapState() {
	mu.Lock()
	defer mu.Unlock()
	proxies = []string{"1.1.1.1:80", "2.2.2.2:80", "3.3.3.3:80", "4.4.4.4:80", "5.5.5.5:80"}
	topWinners = nil
	winnerMs = make(map[string]int)
	winnerBorn = make(map[string]time.Time)
	winnerCooldownUntil = make(map[string]time.Time)
	maxWinners = 20
	winnerTTL = 0
	winnerCooldown = 0
}

// --- inSlice tests ---

func TestInSlice_Found(t *testing.T) {
	s := []string{"a", "b", "c"}
	if !inSlice(s, "b") {
		t.Fatal("expected true for existing element")
	}
}

func TestInSlice_NotFound(t *testing.T) {
	s := []string{"a", "b", "c"}
	if inSlice(s, "d") {
		t.Fatal("expected false for missing element")
	}
}

func TestInSlice_Empty(t *testing.T) {
	if inSlice(nil, "x") {
		t.Fatal("expected false for nil slice")
	}
}

// --- pickRandom tests ---

func TestPickRandom_EnoughProxies(t *testing.T) {
	resetBootstrapState()
	cands := pickRandom(3, nil)
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(cands))
	}
	// All returned candidates should be from the pool
	m := make(map[string]bool)
	for _, p := range proxies {
		m[p] = true
	}
	for _, c := range cands {
		if !m[c] {
			t.Fatalf("candidate %q not in pool", c)
		}
	}
}

func TestPickRandom_SkipMap(t *testing.T) {
	resetBootstrapState()
	skip := map[string]bool{"1.1.1.1:80": true, "2.2.2.2:80": true}
	cands := pickRandom(10, skip)
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates after skipping 2, got %d", len(cands))
	}
	for _, c := range cands {
		if skip[c] {
			t.Fatalf("skipped candidate %q was returned", c)
		}
	}
}

func TestPickRandom_AlreadyWinners(t *testing.T) {
	resetBootstrapState()
	mu.Lock()
	topWinners = []string{"1.1.1.1:80", "2.2.2.2:80"}
	mu.Unlock()
	cands := pickRandom(10, nil)
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(cands))
	}
	for _, c := range cands {
		if c == "1.1.1.1:80" || c == "2.2.2.2:80" {
			t.Fatalf("winner %q should not be in candidates", c)
		}
	}
}

func TestPickRandom_NExceedsPool(t *testing.T) {
	resetBootstrapState()
	skip := map[string]bool{
		"1.1.1.1:80": true,
		"2.2.2.2:80": true,
		"3.3.3.3:80": true,
	}
	cands := pickRandom(100, skip)
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates (pool - skip), got %d", len(cands))
	}
}

func TestPickRandom_Cooldown(t *testing.T) {
	resetBootstrapState()
	mu.Lock()
	winnerCooldownUntil["1.1.1.1:80"] = time.Now().Add(1 * time.Hour)
	mu.Unlock()
	cands := pickRandom(10, nil)
	for _, c := range cands {
		if c == "1.1.1.1:80" {
			t.Fatal("cooled-down proxy should be excluded")
		}
	}
	if len(cands) != 4 {
		t.Fatalf("expected 4 candidates, got %d", len(cands))
	}
}

// --- Bootstrap tests ---

func TestBootstrap_AllPass(t *testing.T) {
	resetBootstrapState()
	mu.Lock()
	maxWinners = 5
	mu.Unlock()

	passFn := func(p string) (bool, int) { return true, 10 }
	Bootstrap(passFn)

	mu.RLock()
	n := len(topWinners)
	mw := maxWinners
	mu.RUnlock()
	if n != mw {
		t.Fatalf("expected %d winners, got %d", mw, n)
	}
}

func TestBootstrap_AllFail(t *testing.T) {
	resetBootstrapState()
	failFn := func(p string) (bool, int) { return false, 0 }

	Bootstrap(failFn)

	mu.RLock()
	n := len(topWinners)
	mu.RUnlock()
	if n != 0 {
		t.Fatalf("expected 0 winners, got %d", n)
	}
}

// --- Refill tests ---

func TestRefill_AlreadyFull(t *testing.T) {
	resetBootstrapState()
	mu.Lock()
	for i, p := range proxies {
		winnerMs[p] = (i + 1) * 10
		winnerBorn[p] = time.Now()
	}
	topWinners = append([]string{}, proxies...)
	mu.Unlock()

	// Pick a different set so refilling would normally add new ones
	passFn := func(p string) (bool, int) { return true, 5 }
	Refill(5, passFn)

	mu.RLock()
	n := len(topWinners)
	mu.RUnlock()
	if n != 5 {
		t.Fatalf("expected 5 winners unchanged, got %d", n)
	}
}

func TestRefill_WinnersLow(t *testing.T) {
	resetBootstrapState()
	mu.Lock()
	topWinners = []string{"1.1.1.1:80"}
	winnerMs["1.1.1.1:80"] = 10
	winnerBorn["1.1.1.1:80"] = time.Now()
	mu.Unlock()

	passFn := func(p string) (bool, int) { return true, 20 }
	Refill(10, passFn)

	mu.RLock()
	n := len(topWinners)
	mu.RUnlock()
	if n <= 1 {
		t.Fatal("expected more winners after refill")
	}
	if n > maxWinners {
		t.Fatalf("winners %d exceeds maxWinners %d", n, maxWinners)
	}
}

// --- Concurrency sanity ---

func TestRefill_Concurrent(t *testing.T) {
	resetBootstrapState()
	passFn := func(p string) (bool, int) { return true, 10 }
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Refill(20, passFn)
		}()
	}
	wg.Wait()

	mu.RLock()
	n := len(topWinners)
	mu.RUnlock()
	if n > maxWinners {
		t.Fatalf("winners %d exceeds maxWinners %d", n, maxWinners)
	}
}
