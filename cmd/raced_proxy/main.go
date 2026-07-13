package main

import (
	"fmt"
	"os"
	"strings"

	"raced_proxy/internal/logger"
	"raced_proxy/internal/rotator"
	"raced_proxy/internal/scanner"
)

var Version = "dev"

func main() {
	ver := Version
	if ver == "dev" {
		if v := readVersionFromFile(); v != "" {
			ver = v
		}
	}
	logger.ShowBanner(ver)

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	mode := strings.ToLower(os.Args[1])
	switch mode {
	case "scan", "scanner":
		scanner.RunScanner()
	case "rotate", "rotator", "server":
		rotator.RunRotator()
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  ./raced_proxy scan     - Run the proxy checker/scanner")
	fmt.Println("  ./raced_proxy rotate   - Start the TCP proxy rotator server")
}

func readVersionFromFile() string {
	data, err := os.ReadFile("file.properties")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version=") {
			return strings.TrimPrefix(line, "version=")
		}
	}
	return ""
}
