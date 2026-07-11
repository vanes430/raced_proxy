package main

import (
	"fmt"
	"os"
	"strings"

	"proxy_check/internal/rotator"
	"proxy_check/internal/scanner"
)

func main() {
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
	fmt.Println("  ./proxy scan     - Run the proxy checker/scanner")
	fmt.Println("  ./proxy rotate   - Start the TCP proxy rotator server")
}
