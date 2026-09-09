# Contributing to simple-status

Thanks for your interest in contributing! This document covers the basics;
for questions open a [discussion](../../discussions) or an issue.

## Development setup

```sh
git clone <your-fork-url>
cd simple-status
go test ./...
```

Requirements:

- Go ≥ 1.25
- `golangci-lint` (optional, for `make lint`)

## Workflow

1. Fork, then create a feature branch from `main`.
2. Make your change. Keep it focused — one logical change per PR.
3. Add or update tests; every package has `_test.go` files to extend.
4. Make sure everything passes:

   ```sh
   make test   # includes the race detector
   make lint
   make fmt    # gofmt
   ```

5. Update `README.md` and `config.example.yml` when your change affects
   configuration or behaviour, and add a `CHANGELOG.md` entry under
   *[Unreleased]*.
6. Open a pull request describing **what** changed and **why**.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/):
`feat: add icmp probe`, `fix: clamp dns resolver timeout`,
`docs: expand config reference`.

## Code style

- Follow standard Go conventions; when in doubt, `gofmt` and
  `golangci-lint` decide.
- Keep functions small and documented: exported identifiers carry doc
  comments (this repo runs with `golint`-style expectations).
- Don't repeat yourself: probe-specific behaviour belongs in
  `internal/prober/<type>.go`, not in the monitor or API layers.
- New probe types integrate via `prober.Register` — see
  `internal/prober/tcp.go` for a minimal example.
- Tests must not depend on the network or on external services.

## Reporting bugs and security issues

Bugs go to the [issue tracker](../../issues). Security vulnerabilities are
handled privately — see [SECURITY.md](SECURITY.md).
