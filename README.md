# 🔄 Raced Proxy (Scanner & Rotator)

Fast proxy checker + racing proxy server built with **Golang**. Runs high-concurrency checks using native goroutines, performs triple-stage verification, and races multiple proxies simultaneously to return the fastest response.

[![release](https://img.shields.io/github/v/release/vanes430/raced_proxy)](https://github.com/vanes430/raced_proxy/releases)

## Features

- **High-Performance Concurrency** — Built with Golang's native goroutines for massive connection throughput without event-loop lag.
- **Triple-Stage Proxy Checker** — 
  - *Stage 1*: CONNECT SSL to `ifconfig.me` (eliminates transparent / leaking proxies).
  - *Stage 2*: CONNECT SSL HTTP target test to `opencode.ai` (drops 403 / 429 blocked IPs).
  - *Stage 3*: Stability check (secondary connection after 100ms to filter out single-use dead proxies).
- **Proxy Rotator** — Local TCP server that races proxies per request and returns the fastest.
- **Smart Selection** — Remembers winning proxies, prioritizes them in future races.
- **Winner Cooldown** — After N wins, champion cools down so other proxies get a turn.
- **Score Decay** — Losers lose rank over time, new winners can overtake.
- **Staggered Racing** — Fires proxies one-by-one with staggered delay, kills all pending on first success.
- **Fast Check** — Verifies proxy against `ifconfig.me` before forwarding the actual request.
- **Slow Proxy Elimination** — Slow losers are automatically penalized or removed from `proxy.txt`.
- **Auth Support** — Optional Basic proxy authentication.
- **Hot Reload** — Auto-reloads `proxy.txt` when file changes.
- **Fancy Logging** — ANSI colored terminal output with unicode icons.
- **Auto CI/CD** — Every push builds 4 targets (linux/windows × amd64/arm64) and creates a GitHub release.

## Download

Grab the latest pre-built binary from [releases](https://github.com/vanes430/raced_proxy/releases).  
Each zip includes the binary, `.env.example`, and `url-list.txt`.

## Build from Source

```bash
go build -ldflags="-s -w" -o raced_proxy cmd/raced_proxy/main.go
```

## Usage

### Proxy Checker (`scan` mode)
Fetches free proxies from multiple sources, runs them through the 3-stage validation, and saves working ones to `proxy.txt`.

```bash
./raced_proxy scan
```

### Proxy Rotator (`rotate` mode)
Runs a local proxy server that races proxies per request and returns the fastest.

```bash
./raced_proxy rotate

# Test it with curl:
curl -x http://127.0.0.1:8090 https://ifconfig.me/ip
```

## Environment Variables (.env)

Customize parameters inside the `.env` file:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8090` | Rotator listen port |
| `CONCURRENCY` | `1000` | Scanner concurrent goroutine limit |
| `TIMEOUT` | `1500` | Proxy connect / read timeout (ms) |
| `MAX_LATENCY` | `1500` | Max accepted latency (ms) |
| `OUTPUT` | `proxy.txt` | Output file |
| `RACE` | `20` | Proxies to race per request |
| `STAGGER` | `20` | Racing stagger delay (ms) |
| `PROXY_USER` | - | Auth username (empty = no auth) |
| `PROXY_PASS` | - | Auth password (empty = no auth) |
| `WINNER_TTL` | `10` | Max wins before champion goes on cooldown |
| `WINNER_COOLDOWN` | `20` | Cooldown runs for a champion |

## Architecture

```
cmd/raced_proxy/main.go → CLI dispatcher (scan / rotate commands)
internal/config/        → Configuration parser (.env)
internal/logger/        → Colorful console logging system
internal/proxy/         → Pool state tracker, CLI engine, and stats management
internal/scanner/       → Triple stage checker routines
internal/rotator/       → Multi-threaded TCP bridge server and racing engines
```
