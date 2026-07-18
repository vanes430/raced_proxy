package rotator

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"raced_proxy/internal/config"
	"raced_proxy/internal/logger"
	"raced_proxy/internal/proxy"
)

var (
	raceCount   int
	staggerMs   int
	maxLatency  int
	scanHost    string
)

func RunRotator() {
	port := config.GetEnvInt("LISTEN_PORT", 8090)
	outputFile := config.GetEnv("PROXY_FILE", "proxy.txt")
	proxyUser := config.GetEnv("AUTH_USER", "")
	proxyPass := config.GetEnv("AUTH_PASS", "")
	raceCount = config.GetEnvInt("RACE", 15)
	staggerMs = config.GetEnvInt("STAGGER", 0)
	maxLatency = config.GetEnvInt("MAX_LATENCY", 0)
	scanHost = config.GetEnv("SCAN_TARGET", "opencode.ai")

	logger.Info("Initializing rotator...")
	logger.Info("Port: %d | Proxy file: %s", port, outputFile)

	hostIP := ""
	ip, err := proxy.GetRealIP()
	if err == nil {
		hostIP = ip
		logger.Info("Host IP: %s", hostIP)
	} else {
		logger.Warn("Host IP detection failed: %v", err)
	}

		proxy.InitPool(outputFile, hostIP)
	if proxy.GetProxiesCount() == 0 {
		logger.Fail("No proxies found. Run scanner first.")
		os.Exit(1)
	}
	logger.Ok("Proxy pool: %d loaded", proxy.GetProxiesCount())

	proxy.InitWinnerConfig()
	logger.Info("Bootstrapping top winners...")
	proxy.Bootstrap(func(p string) (bool, int) {
		t0 := time.Now()
		status, _ := targetCheck(p, scanHost, 443)
		ms := int(time.Since(t0).Milliseconds())
		if status == 200 {
			logger.Ok("bootstrap OK %s (%dms)", p, ms)
			return true, ms
		}
		if status == 429 || status == 403 {
			logger.Warn("bootstrap BLOCKED %d %s (%dms)", status, p, ms)
			proxy.ArchiveRateLimited(p)
		} else {
			logger.Fail("bootstrap FAIL %s (http:%d %dms)", p, status, ms)
			proxy.DeleteProxy(p)
		}
		return false, 0
	})

	go proxy.WatchProxyFile()
	go proxy.StartCLI()
	logger.Info("File watcher and CLI started")

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

	authStr := "\x1b[31mnone\x1b[0m"
	if authEnabled {
		authStr = fmt.Sprintf("\x1b[32menabled\x1b[0m (%s)", proxyUser)
	}

	modelName := config.GetEnv("MODEL_NAME", "mimo-v2.5-free")
	winners := proxy.GetTopWinners()
	logger.Banner("PROXY ROTATOR",
		fmt.Sprintf("Port:      %d", port),
		fmt.Sprintf("Model:     %s", modelName),
		fmt.Sprintf("Host IP:   %s", hostIP),
		fmt.Sprintf("Proxies:   %d loaded", proxy.GetProxiesCount()),
		fmt.Sprintf("Winners:   %d", len(winners)),
		fmt.Sprintf("Auth:      %s", authStr),
		fmt.Sprintf("Command:   curl -x http://127.0.0.1:%d https://ifconfig.me/ip", port),
		"CLI:       Type 'help' for runtime commands",
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var activeConns sync.WaitGroup
	go func() {
		<-sigCh
		logger.Info("Shutting down...")
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
			handleClient(clientConn, authEnabled, authHeaderVal)
		}()
	}

	activeConns.Wait()
	logger.Info("Rotator stopped.")
}

func handleClient(client net.Conn, authEnabled bool, authHeaderVal string) {
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
			logger.Warn("← %s %s %s — AUTH FAIL", from, method, target)
			_, _ = client.Write([]byte("HTTP/1.1 407\r\nProxy-Authenticate: Basic realm=\"proxy\"\r\n\r\n"))
			return
		}
	}

	logger.Section(fmt.Sprintf("← %s %s %s", from, method, target))

	if method == "CONNECT" {
		onCONNECT(client, target, from)
	} else {
		onHTTP(client, buf[:n], from)
	}
}

func onCONNECT(client net.Conn, target, from string) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		host = target
		portStr = "443"
	}
	port, _ := strconv.Atoi(portStr)

	// Route opencode.ai through proxy pool; other targets connect direct.
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

func isRoutedHost(host string) bool {
	return host == scanHost || strings.HasSuffix(host, "."+scanHost)
}

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
