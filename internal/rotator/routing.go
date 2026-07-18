package rotator

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"raced_proxy/internal/logger"
	"raced_proxy/internal/proxy"
)

// onCONNECT handles CONNECT tunnel requests. Routes directly for non-routed hosts
// or races through the proxy pool for the scan target.
func onCONNECT(client net.Conn, target, from string) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		host = target
		portStr = "443"
	}
	port, _ := strconv.Atoi(portStr)

	if !isRoutedHost(host) {
		logger.Info("direct → %s:%d", host, port)
		direct, err := net.DialTimeout("tcp", target, 5*time.Second)
		if err != nil {
			logger.Fail("direct dial FAIL %s:%d (%v)", host, port, err)
			_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		logger.Info("tunnel open → bridging %s ↔ %s:%d (direct)", from, host, port)
		tunnelAndBridge(client, direct, "", host, port)
		return
	}
	for attempt := 0; attempt < raceCount; attempt++ {
		if attempt > 0 && staggerMs > 0 {
			time.Sleep(time.Duration(staggerMs) * time.Millisecond)
		}
		p := proxy.PickTopWinner()
		if p == "" {
			logger.Warn("No top winner available")
			break
		}
		t0 := time.Now()
		status, respID := targetCheck(p, host, port)
		ms := int(time.Since(t0).Milliseconds())
		if maxLatency > 0 && ms > maxLatency {
			logger.Warn("SLOW %s (%dms > %dms max)", p, ms, maxLatency)
			proxy.RemoveWinner(p)
			triggerRefill()
			continue
		}
		if status != 200 {
			if status == 429 || status == 403 {
				logger.Warn("BLOCKED %d %s (%dms)", status, p, ms)
				proxy.ArchiveRateLimited(p)
			} else {
				logger.Warn("FAILED %s (%dms)", p, ms)
				proxy.RemoveWinner(p)
			}
			triggerRefill()
			continue
		}
		conn, err := net.DialTimeout("tcp", p, 5*time.Second)
		if err != nil {
			logger.Warn("dial FAIL %s", p)
			proxy.RemoveWinner(p)
			continue
		}
		req := fmt.Sprintf("CONNECT %s:%d HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port, host, port)
		_, err = conn.Write([]byte(req))
		if err != nil {
			conn.Close()
			proxy.RemoveWinner(p)
			continue
		}
		resp := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(resp)
		conn.SetReadDeadline(time.Time{})
		if err != nil || !strings.Contains(string(resp[:n]), "200") {
			conn.Close()
			proxy.RemoveWinner(p)
			continue
		}
		logger.Ok("proxy %s (ms:%d id:%s)", p, ms, respID)
		_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		logger.Info("tunnel open → bridging %s ↔ %s:%d", from, host, port)
		tunnelAndBridge(client, conn, p, host, port)
		return
	}
	_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
	logger.Fail("ALL FAILED")
}

// isRoutedHost returns true if host matches scanHost exactly or as a subdomain.
func isRoutedHost(host string) bool {
	return host == scanHost || strings.HasSuffix(host, "."+scanHost)
}

// onHTTP forwards plain HTTP requests through a winning proxy and bridges the response.
func onHTTP(client net.Conn, firstChunk []byte, from string) {
	for attempt := 0; attempt < raceCount; attempt++ {
		if attempt > 0 && staggerMs > 0 {
			time.Sleep(time.Duration(staggerMs) * time.Millisecond)
		}
		p := proxy.PickTopWinner()
		if p == "" {
			break
		}
		conn, err := net.DialTimeout("tcp", p, 5*time.Second)
		if err != nil {
			proxy.RemoveWinner(p)
			continue
		}
		_, err = conn.Write(firstChunk)
		if err != nil {
			conn.Close()
			proxy.RemoveWinner(p)
			continue
		}
		status, bufPeek := peekStatus(conn, 3000)
		if status == 429 {
			logger.Warn("BLOCKED 429 %s", p)
			conn.Close()
			proxy.ArchiveRateLimited(p)
			continue
		}
		logger.Ok("proxy %s", p)
		if len(bufPeek) > 0 {
			_, _ = client.Write(bufPeek)
		}
		logger.Info("tunnel open → bridging %s", from)
		tunnelAndBridge(client, conn, p, "", 0)
		return
	}
	_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
	logger.Fail("ALL FAILED")
}
