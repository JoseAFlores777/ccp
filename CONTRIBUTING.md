# Contributing to ccp (profiles for Claude Code)

Thanks for your interest in improving `ccp`. This guide covers how to build,
test, and submit changes.

## Prerequisites

- **Go 1.24+** (the binary is `./cmd/ccp`).
- **bash** and **zsh** (the shell-function/completion contract is exercised in both).
- Optional: `golangci-lint` (v1.62.2 in CI), `shellcheck`.

## Build & run

```bash
go build ./...        # compile (binary: ./cmd/ccp)
go run ./cmd/ccp      # launch the TUI (no args + TTY) or the CLI
```

## The gates (all must be green before a PR)

CI enforces these; run them locally first:

```bash
gofmt -l internal cmd          # must print nothing
go vet ./...
golangci-lint run              # if installed
go test ./...                  # unit tests + golden/parity gates

bash legacy/tests/run.sh                   # bash ORACLE suite
bash testdata/golden/capture.sh --check    # oracle reproduces committed golden
```

### The contract oracle

The Bash implementation in `legacy/` is the **contract oracle**: the Go
binary's observable surface (`_env`, `_hook`, `resolve`, `path test`,
`status --json`, `completion bash|zsh`, `completion-shellinit`) is held
byte-identical to it by `internal/golden/parity_test.go`. If you change any of
those surfaces, you must:

1. update the bash oracle in `legacy/`,
2. regenerate the golden with `bash testdata/golden/capture.sh`,
3. keep `go test ./internal/golden/` green.

The parity gate runs with `CCP_LANG=es` so the (Spanish) frozen prose matches.

### Tests must not touch real config

Always run the binary in tests with a temp `CCP_HOME` so auto-migration does
not fire against your real `~/.config/ccp`.

## Internationalization

User-facing prose lives in the `internal/core/i18n` catalog (English default,
Spanish via `ccp lang es` / `CCP_LANG=es`). When you add a user-facing string:

- add a namespaced key (`cli.*`, `tui.*`, `core.*`) with **both** `En` and `Es`
  entries — `TestCatalogComplete` fails on any orphan,
- resolve the language at the call-site (`i18n.Resolve(cfg.Lang)` or the cli
  `currentLang()` helper) and format with `i18n.T(lang, key, args...)`,
- never hard-code prose in `internal/core` except the shell-init contract
  surfaces (`env.go`, `shellinit.go`).

## Commit & PR conventions

- **Conventional Commits** for subjects (`feat:`, `fix:`, `chore:`, `docs:`,
  `refactor:`, `test:`), under 72 chars; body explains the *why* when not obvious.
- Keep commits focused; prefer a green build at each commit.
- Open a PR against `main`. Make sure CI is green before requesting review.
- **Do not** add tool-attribution trailers (e.g. `Co-Authored-By` bots) to commits.

## Releases

Releases are cut by pushing a `vX.Y.Z` tag; `.github/workflows/release.yml`
builds the 4-platform matrix + `checksums.txt`. Bump the README version badge
in the same change. `install.sh` downloads the prebuilt binary and verifies its
sha256 against `checksums.txt`.

## License

By contributing, you agree that your contributions are licensed under the
project's [MIT License](LICENSE).
