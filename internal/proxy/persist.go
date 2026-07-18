package proxy

import (
	"os"
	"strings"
	"sync"
	"time"
)

// persistCh is a buffered channel used to coalesce proxy.txt write requests.
var persistCh = make(chan struct{}, 1)

// persistMu guards the persist loop (currently unused but reserved for future coordination).
var persistMu sync.Mutex

// init starts the background persist loop goroutine at package load time.
func init() {
	go persistLoop()
}

// persistLoop listens on persistCh, waits 200ms to coalesce rapid changes,
// then writes the current proxy list to proxyFile.
func persistLoop() {
	for range persistCh {
		time.Sleep(200 * time.Millisecond)
		mu.RLock()
		var buf strings.Builder
		for _, p := range proxies {
			buf.WriteString(p + "\n")
		}
		f := proxyFile
		mu.RUnlock()
		_ = os.WriteFile(f, []byte(buf.String()), 0644)
	}
}

// triggerPersist sends a non-blocking signal to the persist loop to write
// the current proxy list to disk.
func triggerPersist() {
	select {
	case persistCh <- struct{}{}:
	default:
	}
}
