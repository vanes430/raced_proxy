package rotator

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
)

const checkTimeout = 5 * time.Second

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

func targetCheck(proxyStr, host string, port int) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", proxyStr)
	if err != nil {
		return 0, ""
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(checkTimeout))

	req := fmt.Sprintf("CONNECT %s:%d HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port, host, port)
	_, err = conn.Write([]byte(req))
	if err != nil {
		return 0, ""
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "200") {
		return 0, ""
	}

	tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
	defer tlsConn.Close()

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return 0, ""
	}

	chatBody := `{"model":"deepseek-v4-flash-free","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`
	postReq := fmt.Sprintf("POST /zen/v1/chat/completions HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", host, len(chatBody), chatBody)
	_, err = tlsConn.Write([]byte(postReq))
	if err != nil {
		return 0, ""
	}

	var respBuf bytes.Buffer
	tlsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _ = io.Copy(&respBuf, tlsConn)
	raw := respBuf.String()

	m := regexp.MustCompile(`HTTP/\d\.\d\s+(\d+)`).FindStringSubmatch(raw)
	if len(m) < 2 {
		return 0, ""
	}
	status, _ := strconv.Atoi(m[1])

	id := extractChatID(raw)
	return status, id
}

var chatIDRe = regexp.MustCompile(`"id"\s*:\s*"([^"]+)"`)

func extractChatID(raw string) string {
	m := chatIDRe.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
