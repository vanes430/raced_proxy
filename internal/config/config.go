package config

import (
	"bufio"
	"crypto/tls"
	"os"
	"strconv"
	"strings"
)

func GetEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if file, err := os.Open(".env"); err == nil {
		defer file.Close()
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

func GetTLSConfig(serverName string) *tls.Config {
	return &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: false,
	}
}
