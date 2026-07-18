# RACED PROXY

**Generated:** 2026-07-16T00:00:00Z
**Commit:** bb3567b
**Branch:** master

## OVERVIEW

Go-based proxy checker + rotator. Zero runtime dependencies — single static binary. 2-stage scanner validates proxies, rotator keeps top 20 fastest in memory via chat completion pre-flight checks, archives rate-limited proxies.

## STRUCTURE

```
raced_proxy/
├── cmd/raced_proxy/main.go    # Entry: CLI dispatcher (scan / rotate)
├── internal/
│   ├── config/                # .env parser
│   ├── logger/                # ANSI-colored terminal output
│   ├── proxy/                 # Pool state, winners, archive, file persist
│   ├── rotator/               # TCP proxy server, pre-flight checks, bridge
│   └── scanner/               # 2-stage proxy validation pipeline
├── .github/workflows/release.yml  # CI: 4-target build + GitHub release
├── file.properties            # Version file (bumped by CI)
└── url-list.txt               # Proxy source URLs for scanner
```

## WHERE TO LOOK

| Task | File(s) | Notes |
|------|---------|-------|
| Entry point | `cmd/raced_proxy/main.go` | Switch on `scan` / `rotate` |
| Scanner pipeline | `internal/scanner/scan.go` | 2 stages: ifconfig.me → chat completion |
| Stage runners | `internal/scanner/stages.go` | runStage1, runStage2 |
| Per-proxy tests | `internal/scanner/test.go` | testIPLeak, testTarget (model big-pickle) |
| Proxy source fetch | `internal/scanner/fetch.go` | Parallel HTTP fetch from url-list.txt |
| TCP server | `internal/rotator/server.go` | RunRotator, onCONNECT, onHTTP |
| Pre-flight check | `internal/rotator/check.go` | targetCheck (model deepseek-v4-flash-free) |
| Bridge + refill | `internal/rotator/bridge.go` | tunnelAndBridge, triggerRefill |
| Winners system | `internal/proxy/winners.go` | Bootstrap, Refill, PickTopWinner, RemoveWinner |
| Archive | `internal/proxy/archive.go` | ArchiveRateLimited, IsRateLimitedToday, DeleteProxy |
| File persist | `internal/proxy/persist.go` | Async proxy.txt write via channel |
| Pool stats | `internal/proxy/pool.go` | InitPool, LoadProxies, GetStats |
| CLI commands | `internal/proxy/cli.go` | del, status, top, reload, reset |
| ENV config | `internal/config/config.go` | GetEnv reads .env or OS env |
| Colored logging | `internal/logger/color.go` | ANSI escape wrappers |
| CI/CD | `.github/workflows/release.yml` | Auto-bump + build 4 targets + release |

## CONVENTIONS

- **Package structure**: Standard Go `cmd/` + `internal/` layout. No external deps.
- **Config**: `config.GetEnv(key, fallback)` — checks OS env first, falls back to `.env` file.
- **Logging**: `logger.Info/Ok/Warn/Fail` everywhere. No raw `fmt.Print` outside logger/CLI.
- **Concurrency**: Goroutines + `sync.WaitGroup` for fan-out, `sync.Mutex`/`sync.RWMutex` for state.
- **Error handling**: Errors logged, rarely returned up stack. Top-level functions handle own errors.
- **CLI commands**: `./raced_proxy scan` / `./raced_proxy rotate`.
- **Version**: Injected via `-ldflags="-X main.Version=X.Y.Z"` at build.

## ANTI-PATTERNS (THIS PROJECT)

- **No tests** — 16 source files, zero `_test.go` files.
- **Regexp.MustCompile in hot path** (`pool.go:60`) — compiled on every `LoadProxies()`. Should be package-level `var`.
- **Inconsistent error returns** — many errors swallowed with `_ =` or bare calls.
- **`InsecureSkipVerify: true`** (`config.go:43`) — TLS skips cert verification.
- **Hardcoded target URLs** — `opencode.ai`, `ifconfig.me` baked in. Changing target requires rebuild.
- **No Docker / no linter / no pre-commit hooks** — zero containerization or quality gates.
- **Module path `raced_proxy` not VCS URL** — breaks `go install remote`.

## UNIQUE STYLES

- **Single-proxy flow**: No racing. Pick top winner → pre-flight chat completion → bridge.
- **Bootstrap + Refill**: Startup tests 20 random in parallel, fills top 20 winners. Auto-refill when ≤10 left.
- **Rate-limit archive**: Proxies with 429 go to `proxy_bekas/YYYY-MM-DD.txt`. Scanner skips today's archived ones.
- **Hot reload**: SHA-256 file hash watched every 3s — `proxy.txt` changes auto-reload.
- **ASCII banner**: Blue pixel-art "RACED PROXY" on startup.
- **Dedup by IP**: Prefers common proxy ports (80, 443, 8080, 8443, 3128, 1080...).

## COMMANDS

```bash
go build -ldflags="-s -w -X main.Version=$(git rev-parse --short HEAD)" -o raced_proxy cmd/raced_proxy/main.go
./raced_proxy scan              # Fetch + validate proxies → proxy.txt
./raced_proxy rotate            # Start TCP proxy server on :8090
curl -x http://127.0.0.1:8090 https://ifconfig.me/ip
```

## NOTES

- `.env` is optional — all vars have sensible defaults.
- `file.properties` stores version string — CI bumps on every push.
- `url-list.txt` contains remote proxy source URLs.
- `proxy_bekas/` directory auto-created for rate-limited proxy archives.
- Scanner uses model `big-pickle`, rotator uses model `deepseek-v4-flash-free`.
