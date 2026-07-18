package config

import (
	"bufio"
	"crypto/tls"
	"os"
	"strconv"
	"strings"
)

// GetEnv reads the value of an environment variable.
// It checks OS env first, then falls back to the .env file.
// key: environment variable name. fallback: value if not found.
// Returns: the resolved value.
func GetEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if file, err := os.Open(".env"); err == nil {
		defer func() { _ = file.Close() }()
		scan := bufio.NewScanner(file)
		for scan.Scan() {
			l := strings.TrimSpace(scan.Text())
			if strings.HasPrefix(l, key+"=") {
				return strings.Trim(strings.SplitN(l, "=", 2)[1], "\"' ")
			}
		}
	}
	return fallback
}

// GetEnvInt reads an integer environment variable.
// Uses GetEnv to resolve the string, then converts to int.
// key: environment variable name. fallback: value if not found or unparseable.
// Returns: the parsed integer.
func GetEnvInt(key string, fallback int) int {
	str := GetEnv(key, "")
	if str == "" {
		return fallback
	}
	val, err := strconv.Atoi(str)
	if err != nil {
		return fallback
	}
	return val
}

// GetTLSConfig returns a TLS configuration for the given server name.
// serverName: the hostname for SNI and certificate verification.
// Returns: a *tls.Config with InsecureSkipVerify set to false.
func GetTLSConfig(serverName string) *tls.Config {
	return &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: false,
	}
}
