# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-09-09

### Added

- Automated service checks on per-service intervals with per-service timeouts.
- Probe types: `http` (status codes, body pattern, custom headers,
  `insecure_tls`), `tcp`, `dns` (A/AAAA/CNAME/MX/NS/TXT, custom resolver).
- Read-only status page: light/dark theme, heartbeat bars, uptime windows
  (24 h / 7 d / 30 d), outage durations, grouped services, auto-refresh.
- JSON API: `/api/v1/summary`, `/api/v1/services/{id}`, `/healthz`.
- Single YAML configuration with CLI flag overrides (`--config`,
  `--listen`, `--interval`, `--timeout`).
- `--validate` and `--once` modes (exit code reflects service health).
- Flap protection via configurable consecutive `retries`.
- In-memory bounded history and incident log per service.
- Docker image (distroless), docker-compose example, Makefile,
  GitHub Actions CI, goreleaser configuration.

[Unreleased]: https://github.com/simple-status/simple-status/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/simple-status/simple-status/releases/tag/v0.1.0
