# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Proxy Rotator — a Bun-based TypeScript proxy checker + rotating proxy server. Zero runtime dependencies; uses Bun's native `fetch` with `proxy` option and raw `node:net` TCP sockets.

## Commands

```bash
bun install                          # install deps
bun run index.ts                     # scan & save working proxies to proxy.txt
bun run rotator.ts                   # start rotator server on :8090
curl -x http://127.0.0.1:8090 https://ifconfig.me/ip   # test the rotator
bunx @biomejs/biome check           # lint + format check
bunx @biomejs/biome check --write   # auto-fix lint/format issues
```

No build step — `tsconfig.json` has `noEmit: true`, Bun runs `.ts` files directly. No tests exist.

## Architecture

Two entry points, three shared modules:

```
index.ts (scanner)  → src/races.ts, src/logger.ts, src/proxyPool.ts
rotator.ts (server) → src/races.ts, src/logger.ts, src/proxyPool.ts
```

- **`index.ts`** — Proxy scanner. Fetches free proxies from 12 sources (`url-list.txt`), tests each against a target, checks for IP leaks, filters by latency, resolves country, writes survivors to `proxy.txt`.
- **`rotator.ts`** — TCP server (port 8090). Per request: picks N proxies, races them staggered, fast-checks the winner, pipes connection to client. Hot-reloads `proxy.txt` every 3s. Optional Basic auth.
- **`src/proxyPool.ts`** — Pool management. Smart selection (top 20% pinned), win/loss scoring with 50% decay, cooldown after N wins, slow-proxy elimination (race > 3s), auto-blacklist (5+ failures).
- **`src/races.ts`** — `raceCONNECT()` and `raceHTTP()`. Fires proxies one-by-one with 1ms stagger. First success wins, all pending destroyed. 6s timeout per proxy.
- **`src/logger.ts`** — ANSI-colored console output (ok/fail/warn/info, latency coloring, banner).

## Key Design Decisions

- **Staggered racing**: proxies fire with 1ms delay (not parallel) — first responder wins, rest killed. This avoids connection storms and naturally selects the fastest proxy.
- **Score system**: winners gain score, losers decay by 50%. Top 20% always pinned. After `WINNER_TTL` wins, champion cools down for `WINNER_COOLDOWN` runs.
- **Hot reload**: `rotator.ts` watches `proxy.txt` for changes every 3 seconds — scanner and rotator can run independently.

## Environment Variables

Defaults in `.env.example`. Key rotator vars: `PORT` (8090), `RACE` (20 proxies per request), `WINNER_TTL` (10), `WINNER_COOLDOWN` (20). Key scanner vars: `TIMEOUT` (6000ms), `MAX_LATENCY` (3000ms), `TARGET` (ifconfig.me/ip).

## Code Style

Biome for linting and formatting — tabs, double quotes, recommended ruleset. Pre-commit hook runs `bunx @biomejs/biome check`. All TypeScript, strict mode.
