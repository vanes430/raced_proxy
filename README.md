# 🔄 Raced Proxy (Scanner & Rotator)

Fast proxy checker + racing proxy server built with **Golang**. Zero runtime dependencies.

[![release](https://img.shields.io/github/v/release/vanes430/raced_proxy)](https://github.com/vanes430/raced_proxy/releases)
[![golangci-lint](https://golangci.com/badges/github.com/vanes430/raced_proxy.svg)](https://golangci.com/report/github.com/vanes430/raced_proxy)

## Features

- **2-Stage Proxy Scanner** — 
  - *Stage 1*: CONNECT + TLS to `ifconfig.me` — detects transparent/IP-leaking proxies.
  - *Stage 2*: CONNECT + TLS to `opencode.ai` → POST chat completion — drops 403/429/error proxies.
- **Proxy Rotator** — Local TCP server that serves the fastest verified proxy per request.
- **Top Winners System** — Bootstraps 20 proxies at startup, tests via chat completion, keeps top 20 fastest in memory.
- **Pre-Flight Check** — Every connection tests the top winner via chat completion POST before bridging.
- **Auto Archive** — Rate-limited (429) proxies moved to `ARCHIVE_DIR/YYYY-MM-DD.txt`, auto-skipped by scanner.
- **Auto Refill** — When winners drop to ≤10, async refill picks+verifies 20 random proxies from pool.
- **Auth Support** — Optional Basic proxy authentication.
- **Hot Reload** — Auto-reloads `proxy.txt` when file changes (SHA-256 watched every 3s).
- **Fancy Logging** — ANSI colored terminal output with connection ID and timing.
- **Auto CI/CD** — Every push builds 4 targets (linux/windows × amd64/arm64) and creates a GitHub release.

## Download

Grab the latest pre-built binary from [releases](https://github.com/vanes430/raced_proxy/releases).

## Build from Source

```bash
go build -ldflags="-s -w -X main.Version=$(git rev-parse --short HEAD)" -o raced_proxy cmd/raced_proxy/main.go
```

## Usage

### Scanner (`scan`)
Fetches free proxies from multiple sources, runs 2-stage validation, saves working ones to `proxy.txt`.

```bash
./raced_proxy scan
```

### Rotator (`rotate`)
Runs a local TCP proxy server. Bootstraps by testing 20 random proxies, keeps top 20 winners. Each connection uses the fastest verified proxy.

```bash
./raced_proxy rotate

curl -x http://127.0.0.1:8090 https://ifconfig.me/ip
```

## Architecture

```
cmd/raced_proxy/main.go → CLI dispatcher (scan / rotate)
internal/config/        → .env config parser
internal/logger/        → ANSI colored logging
internal/proxy/         → Pool state, top winners, archive, file persistence
  ├ pool.go             — state, load, stats
  ├ winners.go          — bootstrap, refill, top winner selection
  ├ archive.go          — rate-limit archive (proxy_bekas/)
  └ persist.go          — async proxy.txt write
internal/scanner/       → 2-stage proxy validation
  ├ scan.go             — pipeline orchestration
  ├ stages.go           — stage 1 + stage 2 runners
  ├ test.go             — IP leak + chat completion checks
  ├ dedup.go            — IP-based deduplication
  └ fetch.go            — remote proxy list fetcher
internal/rotator/       → TCP proxy server
  ├ server.go           — listener, CONNECT/HTTP handlers
  ├ check.go            — pre-flight chat completion check
  └ bridge.go           — tunnel bridging + refill trigger
```

## CLI Console (runtime)

Type these while rotator is running:

```
del <ip:port>   Remove a proxy
status          Pool stats (total / winners)
top [n]         Top N fastest winners
reload          Force reload proxy.txt
reset           Reset winners
help            Show help
```

## Environment Variables (.env)

### Rotator

| Variable | Default | Description |
|---|---|---|
| `LISTEN_PORT` | `8090` | TCP listen port |
| `RACE` | `15` | Max proxy attempts per request |
| `STAGGER` | `0` | Delay between race attempts (ms) |
| `AUTH_USER` | - | Basic auth username |
| `AUTH_PASS` | - | Basic auth password |
| `WINNER_TTL` | `0` | Auto-expire winners after N minutes (0 = disabled) |
| `WINNER_COOLDOWN` | `0` | Cooldown before retrying failed winner in seconds (0 = disabled) |
| `MAX_LATENCY` | `0` | Drop winners exceeding N ms latency (0 = disabled) |

### Scanner

| Variable | Default | Description |
|---|---|---|
| `SCAN_TARGET` | `opencode.ai` | Target host for Stage 2 validation |
| `SOURCE_FILE` | `url-list.txt` | Proxy source URLs file (auto-generated if missing) |
| `REQUEST_TIMEOUT` | `1500` | Connect/read timeout (ms) |
| `PROXY_FILE` | `proxy.txt` | Output file for working proxies |
| `WORKER_COUNT` | `1000` | Scanner goroutine limit |
| `ARCHIVE_DIR` | `proxy_bekas` | Directory for rate-limited proxy archives |

### Model

| Variable | Default | Description |
|---|---|---|
| `MODEL_NAME` | `big-pickle` / `mimo-v2.5-free` | LLM model for validation (scanner/rotator) |

## Connecting to OpenCode

Use the rotator as a proxy for OpenCode:

```bash
# Terminal 1: start rotator
./raced_proxy rotate

# Terminal 2: set proxy and run opencode
export HTTPS_PROXY=http://127.0.0.1:8090
export NO_PROXY=localhost,127.0.0.1
opencode
```

> `NO_PROXY=localhost,127.0.0.1` is **required** — OpenCode's TUI communicates with a local HTTP server. Without it, traffic loops back through the proxy.

For full proxy configuration (authentication, custom certificates, enterprise networks), see the [OpenCode Network docs](https://opencode.ai/docs/network/).

## Using with 9Router

Run Raced Proxy alongside [9Router](https://github.com/decolua/9router) for free AI routing with proxy-rotated connections:

```bash
# Terminal 1: start rotator
./raced_proxy rotate

# Terminal 2: start 9router with proxy
HTTPS_PROXY=http://127.0.0.1:8090 NO_PROXY=localhost,127.0.0.1 npx 9router
```

Or set `HTTPS_PROXY` in your `.env` / shell profile so all tools (Claude Code, Cursor, Cline, OpenCode...) route through the rotator automatically. See [9Router environment variables](https://github.com/decolua/9router#environment-variables) for more options.

## How It Works

1. **Startup** — Rotator loads `proxy.txt`, bootstraps by testing 20 random proxies via chat completion POST. Keeps top 20 as "winners".
2. **Connection** — Pick top winner (fastest). Send chat completion POST through it to `SCAN_TARGET` (default `opencode.ai`). If 200 → bridge tunnel. If 429 → archive to `ARCHIVE_DIR`, remove from winners, try next.
3. **Refill** — When winners ≤ 10, async pick 20 random from pool, test, add passing ones to winners until full.
4. **Scanner** — Fetches from `SOURCE_FILE` (default `url-list.txt`), runs Stage 1 (IP leak) then Stage 2 (chat completion via `SCAN_TARGET`). Skips proxies archived today.
