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
	logger.ShowBanner(Version)

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
