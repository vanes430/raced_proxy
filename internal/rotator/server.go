package rotator

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/vanes430/raced_proxy/internal/config"
	"github.com/vanes430/raced_proxy/internal/logger"
	"github.com/vanes430/raced_proxy/internal/proxy"
)

// raceCount is the maximum number of proxy attempts per request.
var raceCount int

// staggerMs is the delay between race attempts in milliseconds.
var staggerMs int

// maxLatency is the maximum allowed latency in ms before a winner is dropped.
var maxLatency int

// scanHost is the target host for pre-flight chat completion checks.
var scanHost string

// RunRotator starts the TCP proxy listener, bootstraps the winner pool,
// and accepts client connections in a loop until SIGINT/SIGTERM.
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

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		logger.Fail("Failed to bind port %d: %v", port, err)
		os.Exit(1)
	}
	defer func() { _ = listener.Close() }()

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
		_ = listener.Close()
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
