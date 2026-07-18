package rotator

import (
	"fmt"
	"net"
	"strings"
	"time"

	"raced_proxy/internal/logger"
)

// handleClient reads the initial request from a client connection,
// checks optional Basic proxy authentication, and dispatches to the
// appropriate protocol handler (CONNECT tunnel or plain HTTP).
// client: the accepted TCP connection. authEnabled: whether proxy auth is required.
// authHeaderVal: expected Base64-encoded "user:pass" value for auth.
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
