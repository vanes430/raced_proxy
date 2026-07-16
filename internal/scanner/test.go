package scanner

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"raced_proxy/internal/config"
)

func testIPLeak(proxyStr string, timeoutMs int) bool {
	dialer := &net.Dialer{
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		KeepAlive: 0,
	}

	conn, err := dialer.Dial("tcp", proxyStr)
	if err != nil {
		return false
	}
	defer conn.Close()

	req := "CONNECT ifconfig.me:443 HTTP/1.1\r\nHost: ifconfig.me:443\r\n\r\n"
	_, err = conn.Write([]byte(req))
	if err != nil {
		return false
	}

	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	n, err := conn.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "200") {
		return false
	}

	tlsConn := tls.Client(conn, config.GetTLSConfig("ifconfig.me"))
	defer tlsConn.Close()

	err = tlsConn.HandshakeContext(context.Background())
	if err != nil {
		return false
	}

	getReq := "GET /ip HTTP/1.1\r\nHost: ifconfig.me\r\nConnection: close\r\n\r\n"
	_, err = tlsConn.Write([]byte(getReq))
	if err != nil {
		return false
	}

	var respBuf bytes.Buffer
	_ = tlsConn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	_, _ = io.Copy(&respBuf, tlsConn)

	body := respBuf.String()
	ipRegex := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)
	match := ipRegex.FindString(body)

	return match != "" && (realIP == "" || match != realIP)
}

func testTarget(proxyStr string, timeoutMs int) bool {
	dialer := &net.Dialer{
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		KeepAlive: 0,
	}

	conn, err := dialer.Dial("tcp", proxyStr)
	if err != nil {
		return false
	}
	defer conn.Close()

	req := "CONNECT opencode.ai:443 HTTP/1.1\r\nHost: opencode.ai:443\r\n\r\n"
	_, err = conn.Write([]byte(req))
	if err != nil {
		return false
	}

	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	n, err := conn.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "200") {
		return false
	}

	tlsConn := tls.Client(conn, config.GetTLSConfig("opencode.ai"))
	defer tlsConn.Close()

	err = tlsConn.HandshakeContext(context.Background())
	if err != nil {
		return false
	}

	chatBody := `{"model":"big-pickle","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`
	postReq := fmt.Sprintf("POST /zen/v1/chat/completions HTTP/1.1\r\nHost: opencode.ai\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(chatBody), chatBody)
	_, err = tlsConn.Write([]byte(postReq))
	if err != nil {
		return false
	}

	var respBuf bytes.Buffer
	_ = tlsConn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	_, _ = io.Copy(&respBuf, tlsConn)

	resp := respBuf.String()
	m := regexp.MustCompile(`HTTP/\d\.\d\s+(\d+)`).FindStringSubmatch(resp)
	if len(m) < 2 {
		return false
	}
	status, _ := strconv.Atoi(m[1])

	return status == 200 &&
		!strings.Contains(resp, "Rate limit") &&
		!strings.Contains(resp, "FreeUsageLimitError")
}
