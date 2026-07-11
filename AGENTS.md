# AGENTS.md

Golang proxy checker + rotator. No runtime dependencies needed — pre-compiled native binaries run directly.

## Commands

```bash
# 1. Run the scanner to scan and filter proxies -> proxy.txt
CONCURRENCY=1000 TIMEOUT=1500 ./scanner

# 2. Run the rotator server on :8090
./rotator
```

## Compilation

If you make modifications to the `.go` source files, re-compile them using:

```bash
go build -o scanner scanner.go
go build -o rotator rotator.go
```

## Architecture

```
scanner.go        Scanner: fetches sources -> Triple Stage Check (IP Leak, Access, Stability) -> proxy.txt
rotator.go        TCP Server: races proxies -> fast check -> pipes to client
scanner           Pre-compiled scanner executable
rotator           Pre-compiled rotator server executable
url-list.txt      Proxy source URLs (one per line)
proxy.txt         Working proxies (auto-generated, gitignored)
.env              System configurations
```

## Key Facts

* `rotator` listens on `0.0.0.0` (public by default).
* **Triple Stage Check in Scanner:**
  1. **Stage 1:** CONNECT SSL to `ifconfig.me` (eliminates transparent / leaking proxies).
  2. **Stage 2:** CONNECT SSL HTTP target test to `opencode.ai` (drops 403 / 429 blocked IPs).
  3. **Stage 3:** Stability check (makes a secondary connection after 100ms to filter out single-use dead proxies).
* **Rotator CLI Console:** You can type commands directly in the `rotator` running console:
  * `del <ip:port>` to remove a bad proxy on the fly.
  * `status` to print current active pool stats.
  * `top` to show top winning proxies based on latency score.
  * `reload` to reload `proxy.txt`.
  * `reset` to reset statistics.
