package rotator

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/vanes430/raced_proxy/internal/logger"
	"github.com/vanes430/raced_proxy/internal/proxy"
)

// bridgeTimeout is the maximum idle duration before a bridged tunnel is closed.
const bridgeTimeout = 60 * time.Second

// tunnelAndBridge copies data bidirectionally between two connections until
// one side closes or the bridge timeout fires. c1: client connection.
// c2: upstream/proxy connection. proxyStr: the winning proxy address (empty
// for direct). host: target hostname for logging. port: target port.
func tunnelAndBridge(c1, c2 net.Conn, proxyStr, host string, port int) {
	_ = c1.SetDeadline(time.Now().Add(bridgeTimeout))
	_ = c2.SetDeadline(time.Now().Add(bridgeTimeout))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		_, _ = io.Copy(c1, c2)
		_ = c1.Close()
		_ = c2.Close()
		wg.Done()
	}()
	go func() {
		_, _ = io.Copy(c2, c1)
		_ = c2.Close()
		_ = c1.Close()
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
			status, _ := targetCheck(p, scanHost, 443)
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

// triggerRefill starts an async refill of 20 proxies when the winner pool is low.
func triggerRefill() {
	if proxy.NeedRefill() {
		go proxy.Refill(20, func(p string) (bool, int) {
			t0 := time.Now()
			status, _ := targetCheck(p, scanHost, 443)
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
