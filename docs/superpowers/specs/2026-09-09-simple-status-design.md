# simple-status — Design

Date: 2026-09-09
Status: Accepted (autonomous run — defaults documented below in lieu of an interactive Q&A)

## Purpose

A lightweight, self-hosted status page with automated service checks, in the spirit of
Uptime Kuma but radically simpler: there is **no web management interface**. Everything is
controlled by a single YAML config file plus CLI flags. The web UI is read-only.

## Requirements (from the goal)

1. Automated pinging of services on a schedule.
2. No web management UI — CLI flags / single config file only.
3. Multiple service types: `http`, `tcp`, `dns` (extensible via a probe registry).
4. A beautiful, read-only status page.
5. Repository follows open-source standards; clean, DRY, documented code.

## Key decisions (defaults chosen autonomously)

| Decision | Choice | Rationale |
|---|---|---|
| Language | Go | Single static binary, first-class concurrency for parallel probes, `embed` for a dependency-free UI, trivial Docker images. Toolchain is installed via Homebrew. |
| Config format | YAML (`gopkg.in/yaml.v3`), durations as Go duration strings (`30s`, `1m`) | Human-friendly single file; one well-scoped dependency. |
| Storage | In-memory ring buffer + incident log | Keeps the tool simple; a restart starts a fresh history. Documented as a non-goal to persist. |
| UI tech | Server-rendered shell + vanilla JS/CSS embedded via `embed.FS`, no framework, no CDN assets | Works offline/air-gapped, zero build step, small attack surface. |
| Failure confirmation | `retries` (consecutive failures before "down", default 1) | Prevents flapping without adding a queue/DB. |
| Versioning | SemVer, starts at v0.1.0 | Standard for Go OSS. |
| License | MIT | Permissive, standard. |

## Architecture

```
cmd/simple-status/      main: flags, config load, wiring, graceful shutdown
internal/config/        schema, defaults, validation, YAML+flag layering
internal/prober/        Probe interface, Result type, registry; http/tcp/dns implementations
internal/monitor/       one goroutine per service: tick -> probe -> record
internal/store/         thread-safe history (ring buffer), uptime stats, incident log
internal/api/           JSON API (v1) + static UI handler
web/                    index.html, app.js, style.css (embedded at build time)
```

Data flow: `monitor` goroutines write `prober.Result` → `store` → `api` reads snapshots →
UI polls `/api/v1/summary` every 10 s.

### Probes

- **http** — GET (configurable method) a URL; success = expected status class (default
  `2xx`); optional regex body check; records latency. Honours `timeout`.
- **tcp** — `net.Dial` with timeout to `host:port`.
- **dns** — resolve a hostname (optional custom resolver and record type), optionally
  verify the answer is non-empty.
- New types plug into the registry (`prober.Register`), keeping the monitor generic (OCP).

### Config schema (summary)

```yaml
title: Example Status
listen: ":8080"
interval: 30s          # service default
timeout: 10s           # service default
retries: 1             # consecutive failures before "down"
history: 720           # results kept per service
services:
  - id: api            # unique, URL-safe
    name: Public API
    group: Production  # optional, groups cards in UI
    type: http
    target: https://api.example.com/health
    expected_statuses: [200, 204]   # optional, default 2xx class
    body_pattern: '"ok"'            # optional regex
  - id: db
    type: tcp
    target: db.internal:5432
  - id: dns
    type: dns
    target: example.com
```

CLI flags: `--config`, `--listen`, `--interval`, `--timeout`, `--version`,
`--validate` (check config and exit 0/1), `--once` (probe all services once and exit).
Flags override config values; config overrides built-in defaults.

### Status model

`up | down | pending` (pending until first result). Status flips to `down` after `retries`
consecutive failures; any success flips back to `up`. Transitions are appended to a bounded
incident log (last 100 per service) and shown in the UI.

### API

- `GET /api/v1/summary` — overall status + per-service snapshot (name, status, latency,
  uptime 24h/7d/30d, last error)
- `GET /api/v1/services/{id}` — detail incl. recent history and incidents
- `GET /healthz` — liveness of simple-status itself
- `GET /` — status page

### UI

- Overall banner ("All systems operational" / "2 of 7 services down").
- Cards grouped by `group`, each: status dot, name, type badge, current latency,
  uptime percentages, heartbeat bar of recent checks, "down since …" when applicable.
- Auto light/dark via `prefers-color-scheme`, responsive layout, system font stack.
- Auto-refresh (fetch polling); no websockets needed.

## Testing

- `internal/config`: parse + validate (durations, unique IDs, unknown types, defaults).
- `internal/prober`: http via `httptest`, tcp/dns via local listeners; timeout paths.
- `internal/store`: uptime math, ring eviction, incident log, `-race` concurrency test.
- `internal/monitor`: fake probe asserting retries/down/up transitions.
- `internal/api`: handler tests against a populated store.

## Repository standards

README (quickstart, full config reference), LICENSE (MIT), CONTRIBUTING.md,
CODE_OF_CONDUCT.md, SECURITY.md, CHANGELOG.md (Keep a Changelog), .gitignore, Makefile,
.golangci.yml, Dockerfile (multi-stage → distroless), docker-compose.yml example,
config.example.yml, GitHub Actions CI (test matrix, vet, golangci-lint, build), goreleaser
config for releases.

## Non-goals

- Web-based editing of the config (explicitly out of scope).
- Auth on the status page (put it behind a reverse proxy if needed).
- Persistent history across restarts.
- Notification channels (email/webhook/etc.) — future work.
- Multi-instance setups / HA.
