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

	"github.com/vanes430/raced_proxy/internal/config"
)

// checkTimeout is the deadline for dial, TLS handshake, and chat completion.
const checkTimeout = 5 * time.Second

// peekStatus reads from a connection within a timeout and extracts the HTTP
// status code from the response. conn: the connection to read from.
// timeoutMs: read deadline in milliseconds.
// Returns: the HTTP status code and the raw response bytes.
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

// targetCheck performs a full pre-flight check through a proxy: dials the
// proxy, sends a CONNECT tunnel, upgrades to TLS, and POSTs a chat
// completion request to verify the proxy can reach the target.
// proxyStr: proxy address (ip:port). host: target hostname.
// port: target port.
// Returns: HTTP status code and the chat completion response ID.
func targetCheck(proxyStr, host string, port int) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	dialer := &net.Dialer{Timeout: checkTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", proxyStr)
	if err != nil {
		return 0, ""
	}
	defer func() { _ = conn.Close() }()

	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

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

	tlsConn := tls.Client(conn, config.GetTLSConfig(host))
	defer func() { _ = tlsConn.Close() }()

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return 0, ""
	}

	model := config.GetEnv("MODEL_NAME", "mimo-v2.5-free")
	chatBody := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`, model)
	postReq := fmt.Sprintf("POST /zen/v1/chat/completions HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", host, len(chatBody), chatBody)
	_, err = tlsConn.Write([]byte(postReq))
	if err != nil {
		return 0, ""
	}

	var respBuf bytes.Buffer
	rem := time.Until(ctxDeadline(ctx))
	_ = tlsConn.SetReadDeadline(time.Now().Add(rem))
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

// ctxDeadline extracts the deadline from a context, falling back to now+2s.
func ctxDeadline(ctx context.Context) time.Time {
	dl, ok := ctx.Deadline()
	if !ok {
		return time.Now().Add(2 * time.Second)
	}
	return dl
}

// chatIDRe matches the "id" field in a JSON chat completion response.
var chatIDRe = regexp.MustCompile(`"id"\s*:\s*"([^"]+)"`)

// extractChatID extracts the chat completion ID from a raw HTTP response body.
// raw: the full response string. Returns: the ID value, or empty string if not found.
func extractChatID(raw string) string {
	m := chatIDRe.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
