# 🔄 Raced Proxy (Scanner & Rotator)

Fast proxy checker + racing proxy server built with **Golang**. Zero runtime dependencies.

[![release](https://img.shields.io/github/v/release/vanes430/raced_proxy)](https://github.com/vanes430/raced_proxy/releases)

## Features

- **2-Stage Proxy Scanner** — 
  - *Stage 1*: CONNECT + TLS to `ifconfig.me` — detects transparent/IP-leaking proxies.
  - *Stage 2*: CONNECT + TLS to `opencode.ai` → POST chat completion — drops 403/429/error proxies.
- **Proxy Rotator** — Local TCP server that serves the fastest verified proxy per request.
- **Top Winners System** — Bootstraps 20 proxies at startup, tests via chat completion, keeps top 20 fastest in memory.
- **Pre-Flight Check** — Every connection tests the top winner via chat completion POST before bridging.
- **Auto Archive** — Rate-limited (429) proxies moved to `proxy_bekas/YYYY-MM-DD.txt`, auto-skipped by scanner.
- **Auto Refill** — When winners drop to ≤10, async refill picks+verifies 20 random proxies from pool.
- **Auth Support** — Optional Basic proxy authentication.
- **Hot Reload** — Auto-reloads `proxy.txt` when file changes (SHA-256 watched every 3s).
- **Fancy Logging** — ANSI colored terminal output with connection ID and timing.
- **Auto CI/CD** — Every push builds 4 targets (linux/windows × amd64/arm64) and creates a GitHub release.

## Download

Grab the latest pre-built binary from [releases](https://github.com/vanes430/raced_proxy/releases).

## Build from Source

```bash
go build -ldflags="-s -w -X main.Version=$(git describe --tags --abbrev=0 2>/dev/null || echo dev)" -o raced_proxy cmd/raced_proxy/main.go
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

| Variable     | Default   | Description |
|-------------|-----------|-------------|
| `PORT`      | `8090`    | Rotator listen port |
| `CONCURRENCY`| `1000`   | Scanner goroutine limit |
| `TIMEOUT`   | `1500`    | Connect/read timeout (ms) |
| `MAX_LATENCY`| `1500`   | Max accepted latency (ms) |
| `OUTPUT`    | `proxy.txt` | Output file |
| `RACE`      | `20`      | Proxies to race per request |
| `STAGGER`   | `20`      | Racing stagger delay (ms) |
| `PROXY_USER`| -         | Auth username |
| `PROXY_PASS`| -         | Auth password |

## How It Works

1. **Startup** — Rotator loads `proxy.txt`, bootstraps by testing 20 random proxies via chat completion POST. Keeps top 20 as "winners".
2. **Connection** — Pick top winner (fastest). Send chat completion POST through it. If 200 → bridge tunnel. If 429 → archive to `proxy_bekas/`, remove from winners, try next.
3. **Refill** — When winners ≤ 10, async pick 20 random from pool, test, add passing ones to winners until full.
4. **Scanner** — Fetches from `url-list.txt`, runs Stage 1 (IP leak) then Stage 2 (chat completion with model `big-pickle`). Skips proxies archived today.
