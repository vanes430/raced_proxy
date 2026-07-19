package proxy

import (
	"testing"
	"time"
)

// resetWinners zeroes out all winner-related globals for test isolation.
func resetWinners() {
	mu.Lock()
	defer mu.Unlock()
	topWinners = nil
	winnerMs = make(map[string]int)
	winnerBorn = make(map[string]time.Time)
	winnerCooldownUntil = make(map[string]time.Time)
	maxWinners = 20
	winnerTTL = 0
	winnerCooldown = 0
	maxLatencyMs = 0
}

// --- insertWinner ------------------------------------------------------

func TestInsertWinner_SortsByLatency(t *testing.T) {
	resetWinners()

	mu.Lock()
	insertWinner("slow:1", 200)
	insertWinner("fast:2", 50)
	insertWinner("mid:3", 100)
	mu.Unlock()

	winners := GetTopWinners()
	if len(winners) != 3 {
		t.Fatalf("len = %d, want 3", len(winners))
	}
	want := []string{"fast:2", "mid:3", "slow:1"}
	for i, w := range want {
		if winners[i] != w {
			t.Errorf("winners[%d] = %q, want %q", i, winners[i], w)
		}
	}
}

func TestInsertWinner_RespectsMaxWinners(t *testing.T) {
	resetWinners()
	maxWinners = 20

	mu.Lock()
	for i := 0; i < 25; i++ {
		insertWinner("p:"+string(rune('A'+i%26))+":1", (25-i)*10)
	}
	mu.Unlock()

	winners := GetTopWinners()
	if len(winners) > maxWinners {
		t.Errorf("len = %d, want <= %d", len(winners), maxWinners)
	}
	if len(winners) != maxWinners {
		t.Errorf("len = %d, want exactly %d", len(winners), maxWinners)
	}
	// Verify sorted by latency ascending
	for i := 1; i < len(winners); i++ {
		if winnerMs[winners[i]] < winnerMs[winners[i-1]] {
			t.Errorf("not sorted: %dms at [%d] < %dms at [%d]",
				winnerMs[winners[i]], i, winnerMs[winners[i-1]], i-1)
		}
	}
}

func TestInsertWinner_MaxLatencyRejects(t *testing.T) {
	resetWinners()
	maxLatencyMs = 100

	mu.Lock()
	insertWinner("fast:1", 50)
	insertWinner("slow:2", 200)
	mu.Unlock()

	winners := GetTopWinners()
	if len(winners) != 1 {
		t.Fatalf("len = %d, want 1 (slow rejected)", len(winners))
	}
	if winners[0] != "fast:1" {
		t.Errorf("winner = %q, want fast:1", winners[0])
	}
}

func TestInsertWinner_MaxLatencyZeroDisables(t *testing.T) {
	resetWinners()
	maxLatencyMs = 0 // disabled

	mu.Lock()
	insertWinner("fast:1", 50)
	insertWinner("slow:2", 99999)
	mu.Unlock()

	if got := len(GetTopWinners()); got != 2 {
		t.Errorf("len = %d, want 2 (maxLatencyMs=0 should not reject)", got)
	}
}

// --- PickTopWinner -----------------------------------------------------

func TestPickTopWinner_Fastest(t *testing.T) {
	resetWinners()

	mu.Lock()
	insertWinner("slow:1", 300)
	insertWinner("fast:2", 100)
	mu.Unlock()

	if got := PickTopWinner(); got != "fast:2" {
		t.Errorf("PickTopWinner() = %q, want fast:2", got)
	}
}

func TestPickTopWinner_Empty(t *testing.T) {
	resetWinners()

	if got := PickTopWinner(); got != "" {
		t.Errorf("PickTopWinner() = %q, want empty string", got)
	}
}

// --- NeedRefill --------------------------------------------------------

func TestNeedRefill_Empty(t *testing.T) {
	resetWinners()

	if !NeedRefill() {
		t.Error("NeedRefill() = false, want true when empty")
	}
}

func TestNeedRefill_AtHalf(t *testing.T) {
	resetWinners()
	maxWinners = 20

	// Insert exactly 10 (= maxWinners/2) — should need refill
	mu.Lock()
	for i := 0; i < 10; i++ {
		insertWinner("h:"+string(rune('a'+i))+":1", i*10)
	}
	mu.Unlock()

	if !NeedRefill() {
		t.Error("NeedRefill() = false, want true when len == maxWinners/2")
	}
}

func TestNeedRefill_AboveHalf(t *testing.T) {
	resetWinners()
	maxWinners = 20

	// Insert 11 (> maxWinners/2) — should NOT need refill
	mu.Lock()
	for i := 0; i < 11; i++ {
		insertWinner("f:"+string(rune('a'+i))+":1", i*10)
	}
	mu.Unlock()

	if NeedRefill() {
		t.Error("NeedRefill() = true, want false when len > maxWinners/2")
	}
}

func TestNeedRefill_Full(t *testing.T) {
	resetWinners()
	maxWinners = 20

	mu.Lock()
	for i := 0; i < 20; i++ {
		insertWinner("f:"+string(rune('a'+i%26))+":1", i*10)
	}
	mu.Unlock()

	if NeedRefill() {
		t.Error("NeedRefill() = true, want false when full")
	}
}

// --- RemoveWinner ------------------------------------------------------

func TestRemoveWinner_RemovesAndClearsMs(t *testing.T) {
	resetWinners()

	mu.Lock()
	insertWinner("a:1", 100)
	insertWinner("b:2", 200)
	mu.Unlock()

	RemoveWinner("a:1")

	winners := GetTopWinners()
	if len(winners) != 1 {
		t.Fatalf("len = %d, want 1", len(winners))
	}
	if winners[0] != "b:2" {
		t.Errorf("remaining = %q, want b:2", winners[0])
	}

	// Verify winnerMs is also cleared
	mu.RLock()
	_, ok := winnerMs["a:1"]
	mu.RUnlock()
	if ok {
		t.Error("winnerMs[a:1] still present after RemoveWinner")
	}
}

func TestRemoveWinner_Nonexistent(t *testing.T) {
	resetWinners()

	mu.Lock()
	insertWinner("a:1", 100)
	mu.Unlock()

	// Should not panic
	RemoveWinner("nonexistent:999")

	if got := len(GetTopWinners()); got != 1 {
		t.Errorf("len = %d, want 1 (remove of nonexistent should be no-op)", got)
	}
}

func TestRemoveWinner_SetsCooldown(t *testing.T) {
	resetWinners()
	winnerCooldown = 60 * time.Second

	mu.Lock()
	insertWinner("a:1", 100)
	mu.Unlock()

	RemoveWinner("a:1")

	mu.RLock()
	until, ok := winnerCooldownUntil["a:1"]
	mu.RUnlock()

	if !ok {
		t.Fatal("winnerCooldownUntil not set after RemoveWinner")
	}
	if time.Until(until) < 59*time.Second {
		t.Error("cooldown too short")
	}
}

// --- expireWinners -----------------------------------------------------

func TestExpireWinners_RemovesExpired(t *testing.T) {
	resetWinners()
	winnerTTL = 1 * time.Second

	mu.Lock()
	insertWinner("alive:1", 100)
	insertWinner("dead:2", 200)
	winnerBorn["dead:2"] = time.Now().Add(-2 * time.Second)
	mu.Unlock()

	expireWinners()

	winners := GetTopWinners()
	if len(winners) != 1 {
		t.Fatalf("len = %d, want 1 after expiry", len(winners))
	}
	if winners[0] != "alive:1" {
		t.Errorf("survivor = %q, want alive:1", winners[0])
	}

	mu.RLock()
	_, msOk := winnerMs["dead:2"]
	_, bornOk := winnerBorn["dead:2"]
	mu.RUnlock()
	if msOk || bornOk {
		t.Error("dead:2 data not cleaned from winnerMs/winnerBorn")
	}
}

func TestExpireWinners_NoTTL(t *testing.T) {
	resetWinners()
	winnerTTL = 0 // disabled

	mu.Lock()
	insertWinner("ancient:1", 100)
	winnerBorn["ancient:1"] = time.Now().Add(-999 * time.Hour)
	mu.Unlock()

	expireWinners()

	if got := len(GetTopWinners()); got != 1 {
		t.Errorf("len = %d, want 1 (TTL disabled, nothing should expire)", got)
	}
}
