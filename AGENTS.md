# AGENTS.md

Golang proxy checker + rotator. Zero runtime dependencies — pre-compiled native binaries run directly.

## Commands

```bash
# 1. Compile
go build -o raced_proxy cmd/raced_proxy/main.go

# 2. Run the scanner to scan and filter proxies -> proxy.txt
./raced_proxy scan

# 3. Run the rotator server on :8090
./raced_proxy rotate
```

## Environment

```bash
cp .env.example .env
# edit .env as needed
```

## Architecture

```
cmd/raced_proxy/main.go   Entry point: CLI dispatcher (scan / rotate)
internal/config/          .env file parser
internal/logger/          ANSI colored terminal output
internal/proxy/           Pool management, selection, scoring, cooldowns, CLI console
internal/scanner/         Triple-stage proxy validation pipeline
internal/rotator/         TCP server: races proxies, bridges fastest to client
```

## Key Facts

* Single binary: `raced_proxy scan` or `raced_proxy rotate`.
* **Triple Stage Check in Scanner:**
  1. **Stage 1:** CONNECT SSL to `ifconfig.me` — eliminates transparent / leaking proxies.
  2. **Stage 2:** CONNECT SSL to `opencode.ai` — drops 403 / 429 blocked IPs.
  3. **Stage 3:** Stability re-check after 100ms — filters single-use dead proxies.
* **Rotator CLI Console** (type commands while running):
  * `del <ip:port>` — remove a bad proxy on the fly.
  * `status` — pool stats (active / cooling / banned).
  * `top [n]` — top N winning proxies by score.
  * `reload` — force reload `proxy.txt`.
  * `reset` — reset all stats.
  * `help` — show commands.
* Version is injected at build time via `-ldflags="-X main.Version=..."`.
