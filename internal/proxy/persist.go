package proxy

import (
	"os"
	"strings"
	"sync"
	"time"
)

var (
	persistCh = make(chan struct{}, 1)
	persistMu sync.Mutex
)

func init() {
	go persistLoop()
}

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

func triggerPersist() {
	select {
	case persistCh <- struct{}{}:
	default:
	}
}
