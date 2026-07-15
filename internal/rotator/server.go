package rotator

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"raced_proxy/internal/config"
	"raced_proxy/internal/logger"
	"raced_proxy/internal/proxy"
)

type RaceResult struct {
	Conn     net.Conn
	Rest     []byte
	Proxy    string
	Attempts int
}

func RunRotator() {
	port := config.GetEnvInt("PORT", 8090)
	raceCount := config.GetEnvInt("RACE", 20)
	staggerMs := config.GetEnvInt("STAGGER", 20)
	winnerTTL := config.GetEnvInt("WINNER_TTL", 10)
	winnerCooldown := config.GetEnvInt("WINNER_COOLDOWN", 20)
	maxLatencyMs := config.GetEnvInt("MAX_LATENCY", 1500)
	outputFile := config.GetEnv("OUTPUT", "proxy.txt")
	proxyUser := config.GetEnv("PROXY_USER", "")
	proxyPass := config.GetEnv("PROXY_PASS", "")

	logger.Info("Initializing rotator with config...")
	logger.Info("Port: %d | Race: %d | Stagger: %dms | MaxLatency: %dms", port, raceCount, staggerMs, maxLatencyMs)
	logger.Info("Winner TTL: %d | Winner Cooldown: %d | Proxy file: %s", winnerTTL, winnerCooldown, outputFile)

	var vpsIP string
	ip, err := proxy.GetRealIP()
	if err == nil {
		vpsIP = ip
		logger.Info("VPS IP detected: %s", vpsIP)
	} else {
		logger.Warn("VPS IP detection failed: %v", err)
	}

	logger.Info("Loading proxy pool from %s...", outputFile)
	proxy.InitPool(outputFile, vpsIP)
	if proxy.GetProxiesCount() == 0 {
		logger.Fail("No proxies found in %s. Run checker first.", outputFile)
		logger.Info("Run: ./raced_proxy scan")
		os.Exit(1)
	}
	logger.Ok("Proxy pool loaded: %d proxies", proxy.GetProxiesCount())

	go proxy.WatchProxyFile()
	go proxy.StartCLI()
	logger.Info("File watcher and CLI console started")

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		logger.Fail("Failed to bind port %d: %v", port, err)
		os.Exit(1)
	}
	defer listener.Close()

	authEnabled := proxyUser != "" && proxyPass != ""
	authHeaderVal := ""
	if authEnabled {
		authHeaderVal = "Basic " + base64.StdEncoding.EncodeToString([]byte(proxyUser+":"+proxyPass))
	}

	authStatusStr := "\x1b[31mnone\x1b[0m"
	if authEnabled {
		authStatusStr = fmt.Sprintf("\x1b[32menabled\x1b[0m (%s)", proxyUser)
	}

	logger.Banner("PROXY ROTATOR",
		fmt.Sprintf("Port:      %d", port),
		fmt.Sprintf("VPS IP:    %s", vpsIP),
		fmt.Sprintf("Proxies:   %d loaded", proxy.GetProxiesCount()),
		fmt.Sprintf("Race:      %d per request", raceCount),
		fmt.Sprintf("Auth:      %s", authStatusStr),
		fmt.Sprintf("Command:   curl -x http://127.0.0.1:%d https://ifconfig.me/ip", port),
		"CLI:       Type 'help' for runtime commands",
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var activeConns sync.WaitGroup
	go func() {
		<-sigCh
		logger.Info("Shutting down rotator...")
		listener.Close()
	}()

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			break
		}
		activeConns.Add(1)
		go func() {
			defer activeConns.Done()
			handleClient(clientConn, authEnabled, authHeaderVal, raceCount, staggerMs, maxLatencyMs, winnerTTL, winnerCooldown)
		}()
	}

	activeConns.Wait()
	logger.Info("Rotator stopped.")
}

func handleClient(client net.Conn, authEnabled bool, authHeaderVal string, raceCount int, staggerMs int, maxLatencyMs int, winnerTTL int, winnerCooldown int) {
	defer client.Close()

	buf := make([]byte, 4096)
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := client.Read(buf)
	if err != nil || n == 0 {
		return
	}
	_ = client.SetReadDeadline(time.Time{})

	head := string(buf[:n])
	lines := strings.Split(head, "\r\n")
	if len(lines) == 0 {
		return
	}

	parts := strings.Split(lines[0], " ")
	if len(parts) < 2 {
		return
	}
	method := parts[0]
	target := parts[1]

	from := client.RemoteAddr().String()
	logger.Section("CONNECTION ← " + from)
	logger.Info("Method: %s | Target: %s", method, target)
	logger.Info("Auth: %v", authEnabled)

	if authEnabled {
		authorized := false
		for _, line := range lines {
			if strings.HasPrefix(strings.ToLower(line), "proxy-authorization:") {
				val := strings.TrimSpace(line[strings.Index(line, ":")+1:])
				if val == authHeaderVal {
					authorized = true
					break
				}
			}
		}
		if !authorized {
			logger.Warn("AUTH FAIL ← %s", from)
			logger.Warn("Authentication required but no valid Proxy-Authorization header found")
			logger.Info("Sending 407 Proxy Authentication Required response")
			_, _ = client.Write([]byte("HTTP/1.1 407\r\nProxy-Authenticate: Basic realm=\"proxy\"\r\n\r\n"))
			return
		} else {
			logger.Ok("AUTH OK ← %s", from)
		}
	}

	if method == "CONNECT" {
		onCONNECT(client, target, from, raceCount, staggerMs, maxLatencyMs, winnerTTL, winnerCooldown)
	} else {
		onHTTP(client, buf[:n], from, raceCount, staggerMs, maxLatencyMs, winnerTTL, winnerCooldown)
	}
}

func onCONNECT(client net.Conn, target string, from string, raceCount int, staggerMs int, maxLatencyMs int, winnerTTL int, winnerCooldown int) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		host = target
		portStr = "443"
	}
	port, _ := strconv.Atoi(portStr)

	logger.Info("CONNECT target: %s:%d", host, port)
	logger.Info("Race config: %d proxies | %dms stagger | max %d attempts", raceCount, staggerMs, 15)

	tried := make(map[string]bool)
	attempt := 0
	var winner *RaceResult

	for attempt < 15 {
		cands := proxy.PickProxies(raceCount, tried)
		if len(cands) == 0 {
			logger.Warn("CONNECT %s:%d — no more candidates (attempt %d)", host, port, attempt+1)
			break
		}
		for _, p := range cands {
			tried[p] = true
		}
		attempt++

		logger.Info("CONNECT attempt %d/15 — racing %d proxies...", attempt, len(cands))

		t0 := time.Now()
		res := raceCONNECT(host, port, cands, staggerMs)
		raceMs := time.Since(t0).Milliseconds()

		if res == nil {
			logger.Warn("CONNECT %s:%d — attempt %d: all %d proxies failed (%.0fms)", host, port, attempt, len(cands), float64(raceMs))
			continue
		}

		logger.Ok("CONNECT %s:%d — proxy %s won the race (attempt %d/%d, %.0fms)", host, port, res.Proxy, res.Attempts, len(cands), float64(raceMs))

		t1 := time.Now()
		ok := fastCheck(res.Proxy)
		checkMs := time.Since(t1).Milliseconds()

		if !ok {
			logger.Warn("CONNECT %s:%d via %s — fast check FAILED (%dms)", host, port, res.Proxy, checkMs)
			logger.Warn("Fast check failure: proxy may be leaking IP or timed out")
			proxy.RecordFail(res.Proxy)
			res.Conn.Close()
			continue
		}
		logger.Ok("Fast check PASSED for %s (%dms)", res.Proxy, checkMs)

		// Pre-flight: test proxy against actual target via HTTP
		t2 := time.Now()
		status := targetCheck(res.Proxy, host, port)
		targetMs := time.Since(t2).Milliseconds()
		if status == 429 || status == 403 {
			logger.Warn("CONNECT %s:%d via %s — target returned %d, deleting proxy...", host, port, res.Proxy, status)
			logger.Warn("Target blocked status %d — proxy removed from pool", status)
			proxy.DeleteProxy(res.Proxy)
			res.Conn.Close()
			continue
		}
		logger.Ok("Target check PASSED for %s (%dms, status %d)", res.Proxy, targetMs, status)

		proxy.RemoveSlowProxies(cands, res.Attempts, int(raceMs), maxLatencyMs)
		winner = res

		logger.Ok("CONNECT %s:%d → winner %s (race: %dms | fast: %dms | target: %dms | pos: %d/%d)", host, port, res.Proxy, raceMs, checkMs, targetMs, res.Attempts, len(cands))
		break
	}

	if winner == nil {
		logger.Fail("CONNECT %s:%d — ALL FAILED after %d attempts (%d proxies tried)", host, port, attempt, len(tried))
		logger.Fail("Returning 502 Bad Gateway to %s", from)
		_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer winner.Conn.Close()

	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if len(winner.Rest) > 0 {
		_, _ = client.Write(winner.Rest)
	}

	proxy.RecordWin(winner.Proxy, winner.Attempts, winnerTTL, winnerCooldown)
	proxy.TickCooldowns()

	logger.Info("CONNECT tunnel established → bridging %s ↔ %s:%d", from, host, port)
	bridge(client, winner.Conn)
	logger.Info("CONNECT tunnel closed: %s -> %s:%d", from, host, port)
}

func onHTTP(client net.Conn, firstChunk []byte, from string, raceCount int, staggerMs int, maxLatencyMs int, winnerTTL int, winnerCooldown int) {
	tried := make(map[string]bool)
	attempt := 0
	var winner *RaceResult
	var buffered []byte

	logger.Info("HTTP proxy request from %s", from)
	logger.Info("Race config: %d proxies | %dms stagger | max %d attempts", raceCount, staggerMs, 15)

	for attempt < 15 {
		cands := proxy.PickProxies(raceCount, tried)
		if len(cands) == 0 {
			logger.Warn("HTTP — no more candidates (attempt %d)", attempt+1)
			break
		}
		for _, p := range cands {
			tried[p] = true
		}
		attempt++

		logger.Info("HTTP attempt %d/15 — racing %d proxies...", attempt, len(cands))

		t0 := time.Now()
		res := raceHTTP(firstChunk, cands, staggerMs)
		raceMs := time.Since(t0).Milliseconds()

		if res == nil {
			logger.Warn("HTTP attempt %d: all %d proxies failed (%.0fms)", attempt, len(cands), float64(raceMs))
			continue
		}

		logger.Ok("HTTP — proxy %s won the race (attempt %d/%d, %.0fms)", res.Proxy, res.Attempts, len(cands), float64(raceMs))

		proxy.RemoveSlowProxies(cands, res.Attempts, int(raceMs), maxLatencyMs)

		status, bufPeek := peekStatus(res.Conn, 3000)
		if status == 429 {
			logger.Warn("HTTP via %s returned 429 (rate limited) — deleting", res.Proxy)
			res.Conn.Close()
			proxy.DeleteProxy(res.Proxy)
			continue
		}

		winner = res
		buffered = bufPeek
		logger.Ok("HTTP → winner %s (race: %dms | pos: %d/%d)", res.Proxy, raceMs, res.Attempts, len(cands))
		break
	}

	if winner == nil {
		logger.Fail("HTTP — ALL FAILED after %d attempts (%d proxies tried)", attempt, len(tried))
		logger.Fail("Returning 502 Bad Gateway to %s", from)
		_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer winner.Conn.Close()

	if len(buffered) > 0 {
		_, _ = client.Write(buffered)
	}

	proxy.RecordWin(winner.Proxy, winner.Attempts, winnerTTL, winnerCooldown)
	proxy.TickCooldowns()

	logger.Info("HTTP tunnel established → bridging %s ↔ target", from)
	bridge(client, winner.Conn)
	logger.Info("HTTP tunnel closed: %s", from)
}

func bridge(c1, c2 net.Conn) {
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
}

func raceCONNECT(host string, port int, candidates []string, staggerMs int) *RaceResult {
	var muLock sync.Mutex
	var winner *RaceResult
	var wg sync.WaitGroup
	done := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for idx, p := range candidates {
		if idx > 0 {
			select {
			case <-done:
				break
			case <-time.After(time.Duration(staggerMs) * time.Millisecond):
			}
		}

		muLock.Lock()
		if winner != nil {
			muLock.Unlock()
			break
		}
		muLock.Unlock()

		wg.Add(1)
		go func(proxyStr string, attemptNum int) {
			defer wg.Done()

			dialer := &net.Dialer{Timeout: 6 * time.Second}
			conn, err := dialer.DialContext(ctx, "tcp", proxyStr)
			if err != nil {
				return
			}

			req := fmt.Sprintf("CONNECT %s:%d HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port, host, port)
			_, err = conn.Write([]byte(req))
			if err != nil {
				conn.Close()
				return
			}

			buf := make([]byte, 2048)
			_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				conn.Close()
				return
			}
			_ = conn.SetReadDeadline(time.Time{})

			resp := string(buf[:n])
			if !strings.Contains(resp, " 200") || strings.Contains(resp, " 429 ") || strings.Contains(resp, "429 Too Many") || strings.Contains(resp, "Rate limit") {
				conn.Close()
				return
			}

			muLock.Lock()
			if winner == nil {
				winner = &RaceResult{
					Conn:     conn,
					Proxy:    proxyStr,
					Attempts: attemptNum,
					Rest:     buf[strings.Index(resp, "\r\n\r\n")+4 : n],
				}
				close(done)
				cancel()
			} else {
				conn.Close()
			}
			muLock.Unlock()
		}(p, idx+1)
	}

	wg.Wait()
	return winner
}

func raceHTTP(firstChunk []byte, candidates []string, staggerMs int) *RaceResult {
	var muLock sync.Mutex
	var winner *RaceResult
	var wg sync.WaitGroup
	done := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for idx, p := range candidates {
		if idx > 0 {
			select {
			case <-done:
				break
			case <-time.After(time.Duration(staggerMs) * time.Millisecond):
			}
		}

		muLock.Lock()
		if winner != nil {
			muLock.Unlock()
			break
		}
		muLock.Unlock()

		wg.Add(1)
		go func(proxyStr string, attemptNum int) {
			defer wg.Done()

			dialer := &net.Dialer{Timeout: 6 * time.Second}
			conn, err := dialer.DialContext(ctx, "tcp", proxyStr)
			if err != nil {
				return
			}

			_, err = conn.Write(firstChunk)
			if err != nil {
				conn.Close()
				return
			}

			muLock.Lock()
			if winner == nil {
				winner = &RaceResult{
					Conn:     conn,
					Proxy:    proxyStr,
					Attempts: attemptNum,
				}
				close(done)
				cancel()
			} else {
				conn.Close()
			}
			muLock.Unlock()
		}(p, idx+1)
	}

	wg.Wait()
	return winner
}

func peekStatus(conn net.Conn, timeoutMs int) (int, []byte) {
	buf := make([]byte, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	n, err := conn.Read(buf)
	_ = conn.SetReadDeadline(time.Time{})

	if err != nil || n == 0 {
		return 0, nil
	}

	text := string(buf[:n])
	m := regexp.MustCompile(`HTTP/\d\.\d\s+(\d+)`).FindStringSubmatch(text)
	status := 0
	if len(m) > 1 {
		status, _ = strconv.Atoi(m[1])
	}

	if n < 1000 && (strings.Contains(text, "Rate limit") || strings.Contains(text, "FreeUsageLimitError")) {
		status = 429
	}

	return status, buf[:n]
}

func fastCheck(proxyStr string) bool {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.Dial("tcp", proxyStr)
	if err != nil {
		return false
	}
	defer conn.Close()

	_, err = conn.Write([]byte("CONNECT ifconfig.me:443 HTTP/1.1\r\nHost: ifconfig.me:443\r\n\r\n"))
	if err != nil {
		return false
	}

	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "200") {
		return false
	}

	tlsConn := tls.Client(conn, config.GetTLSConfig("ifconfig.me"))
	defer tlsConn.Close()

	_ = tlsConn.SetDeadline(time.Now().Add(5 * time.Second))
	err = tlsConn.Handshake()
	if err != nil {
		return false
	}

	_, err = tlsConn.Write([]byte("GET /ip HTTP/1.1\r\nHost: ifconfig.me\r\nConnection: close\r\n\r\n"))
	if err != nil {
		return false
	}

	var respBuf bytes.Buffer
	_, _ = io.Copy(&respBuf, tlsConn)
	body := respBuf.String()

	match := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`).FindString(body)
	return match != "" && (proxy.GetVPSIP() == "" || match != proxy.GetVPSIP())
}

// targetCheck does a quick HTTP GET through proxy to target to detect 429/403.
func targetCheck(proxyStr, host string, port int) int {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.Dial("tcp", proxyStr)
	if err != nil {
		return 0
	}
	defer conn.Close()

	req := fmt.Sprintf("CONNECT %s:%d HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port, host, port)
	_, err = conn.Write([]byte(req))
	if err != nil {
		return 0
	}
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	conn.SetReadDeadline(time.Time{})
	if err != nil || !strings.Contains(string(buf[:n]), "200") {
		return 0
	}

	tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
	defer tlsConn.Close()
	tlsConn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := tlsConn.Handshake(); err != nil {
		return 0
	}
	tlsConn.SetDeadline(time.Time{})

	// Send GET request and check status
	getReq := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host)
	_, err = tlsConn.Write([]byte(getReq))
	if err != nil {
		return 0
	}

	var respBuf bytes.Buffer
	tlsConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.Copy(&respBuf, tlsConn)
	body := respBuf.String()

	m := regexp.MustCompile(`HTTP/\d\.\d\s+(\d+)`).FindStringSubmatch(body)
	if len(m) < 2 {
		return 0
	}
	status, _ := strconv.Atoi(m[1])
	return status
}
