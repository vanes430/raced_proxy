package rotator

import (
	"io"
	"net"
	"sync"
	"time"

	"raced_proxy/internal/logger"
	"raced_proxy/internal/proxy"
)

const bridgeTimeout = 60 * time.Second

func tunnelAndBridge(c1, c2 net.Conn, proxyStr, host string, port int) {
	c1.SetDeadline(time.Now().Add(bridgeTimeout))
	c2.SetDeadline(time.Now().Add(bridgeTimeout))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		_, _ = io.Copy(c1, c2)
		c1.Close()
		c2.Close()
		wg.Done()
	}()
	go func() {
		_, _ = io.Copy(c2, c1)
		c2.Close()
		c1.Close()
		wg.Done()
	}()
	wg.Wait()

	if host != "" {
		logger.Info("tunnel closed: -> %s:%d", host, port)
	} else {
		logger.Info("tunnel closed")
	}

	if proxy.NeedRefill() {
		go proxy.Refill(20, func(p string) (bool, int) {
			t0 := time.Now()
			status, _ := targetCheck(p, "opencode.ai", 443)
			ms := int(time.Since(t0).Milliseconds())
			if status == 200 {
				return true, ms
			}
			if status == 429 || status == 403 {
				proxy.ArchiveRateLimited(p)
			}
			return false, 0
		})
	}
}
