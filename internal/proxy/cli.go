package proxy

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func StartCLI() {
	inputScan := bufio.NewScanner(os.Stdin)
	for inputScan.Scan() {
		line := strings.TrimSpace(inputScan.Text())
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		cmd := strings.ToLower(parts[0])
		switch cmd {
		case "del", "delete":
			if len(parts) < 2 || !strings.Contains(parts[1], ":") {
				fmt.Println("\x1b[33mUsage: del <ip:port>\x1b[0m")
				continue
			}
			DeleteProxy(parts[1])
			fmt.Printf("\x1b[37mDeleted proxy %s manually.\x1b[0m\n", parts[1])

		case "status", "stat":
			total, active, cooling, banned := GetStats()
			fmt.Printf("Total: %d | Active: \x1b[32m%d\x1b[0m | Cooling: \x1b[33m%d\x1b[0m | Banned: \x1b[31m%d\x1b[0m\n", total, active, cooling, banned)

		case "top":
			limit := 10
			if len(parts) > 1 {
				if val, err := strconv.Atoi(parts[1]); err == nil {
					limit = val
				}
			}
			PrintTopWinners(limit)

		case "reload":
			LoadProxies()
			fmt.Printf("Reloaded proxy file manually. Total: %d proxies\n", GetProxiesCount())

		case "reset":
			ResetStats()
			fmt.Println("All proxy scores and stats reset successfully.")

		case "help":
			fmt.Println(`
  del <ip:port>   Remove a proxy
  status          Pool stats (active/cooling/banned)
  top [n]         Top N winning proxies (default: 10)
  reload          Force reload proxy.txt
  reset           Reset all stats and cooldowns
  help            Show this help`)

		default:
			fmt.Println("\x1b[33mUnknown command. Type 'help' for commands.\x1b[0m")
		}
	}
}
