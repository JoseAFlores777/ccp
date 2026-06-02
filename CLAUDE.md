# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`ccp` is a CLI (Go, v2.0) that routes Claude Code to a named **profile** per terminal and per directory — never global. A profile is one of: an **official** Anthropic account (its own `CLAUDE_CONFIG_DIR`), a **DeepSeek/compatible provider** (its own `ANTHROPIC_BASE_URL`/`ANTHROPIC_AUTH_TOKEN` + models), or the reserved **`default`** (the user's normal `~/.claude` login). So repo A → account *work*, repo B → account *personal*, repo C → *deepseek*. User-facing strings are in Spanish.

`ccp` was **rewritten from Bash to Go** in v2.0 (plan: `docs/superpowers/plans/migracion-go-v2.md`). The Bash implementation is **archived in `legacy/`** and is now the **contract oracle**: the Go binary's observable surface is held byte-identical to it by a golden-diff gate. The chained migrator still auto-upgrades old state: the first Go run migrates `~/.config/dsctl` → `ccp` (TSV) → `ccp.yaml`, backing up first (`~/.config/ccp/.backup-pre-go-*`). The old rc block is unchanged (it only calls `command ccp`), so no rc reinstall is needed.

## Commands

```bash
go build ./...                 # compile (binary: ./cmd/ccp)
go test ./...                  # unit tests (core/cli) + golden gates
gofmt -l internal cmd          # must print nothing (CI gate)
go vet ./...                   # CI gate
golangci-lint run              # CI gate (v1.62.2 in CI)

bash legacy/tests/run.sh       # bash ORACLE suite (still a CI gate)
bash testdata/golden/capture.sh --check   # oracle reproduces committed golden
bash testdata/golden/capture.sh           # regenerate golden from the oracle
```

The **parity gate** lives in `internal/golden/parity_test.go`: it builds the Go binary, runs it over the `testdata/golden/basic` fixture, and asserts its stdout + exit code match the committed `expected/` (which were captured from the bash oracle by `capture.sh`). Green ⇒ Go == bash on the frozen contract (`_env`, `_hook`, `resolve`, `path test`, `status --json`, `completion bash|zsh`, `completion-shellinit`). When you change any of those, update the bash oracle in `legacy/`, regenerate the golden with `capture.sh`, and keep the parity test green.

Always run the binary in tests with a temp `CCP_HOME` (an existing dir) so auto-migration does not fire against real `~/.config/dsctl`.

## Architecture

The binary/shell-function split is the central design constraint and is **unchanged** by the rewrite — the Go binary just emits the same shell tail the bash did:

- **`cmd/ccp` + `internal/cli`** — the CLI binary. Dispatch is by hand (no cobra) so the bash/zsh completion is emitted **verbatim**. Runs in a child process, so it **cannot** mutate the parent shell's environment.
- **Shell function `ccp`** — emitted by `core.WriteShellInit` (the bash heredoc ported to a Go string constant, byte-identical) and appended to the rc by `ccp install`. Because it runs *in* the shell, only it can `export`/`unset`. It intercepts `use <perfil>` / `default` / `off` / `on` / `run`, applying the env delta via `eval "$(command ccp _env <perfil>)"`; everything else falls through to `command ccp "$@"`.
- **`_ccp_autocheck` hook** — runs on every prompt (zsh `precmd_functions` / bash `PROMPT_COMMAND`), caches by `$PWD`, applies the resolved profile via `eval "$(command ccp _hook "$PWD")"`. This is why `cd` alone flips the profile.

So: env mutation lives in the rc-installed function; all logic lives in the binary; the binary EMITS env (`ccp _env`/`_hook`); the shell EVALs it — the only place env changes.

### `internal/core` (the engine — no presentation I/O)

Returns data/strings; the front-ends format. The exceptions are `env.go` and `shellinit.go`, which produce exact strings because they ARE the contract.

- **`rules.go`** — pure resolver. `Resolve(query, rules)` returns the **profile name**: among rules whose path is P or an ancestor, the **deepest** wins; no match → `default`. `rules_cmd.go` does the CRUD (`RuleSet/RuleDel/RulesClear/RulesList`) over `ccp.yaml`. `NormalizePath` resolves `.`/`..`/`~` textually (not `realpath`).
- **`profile.go`** — profile CRUD on an explicit `home` arg (`ProfileAddOfficial/AddDeepseek/Rm/List/Show/SetKey`). Seeds each non-`default` profile's `cc-home` (symlinks `plugins/ commands/ agents/ skills/` from `~/.claude`).
- **`env.go`** — `EnvDelta(home, profile, cfg)` emits the eval-able delta: always `unset` all managed vars first, then `export` the target's. Every value is quoted by `shellQuote`, a hand-rolled replica of bash `printf %q` (NOT `strconv.Quote`) — this is contract risk #1; it has a dedicated test plus an eval-effect test in zsh+bash.
- **`store.go`** — reads/writes the canonical `ccp.yaml` (atomic tmp+rename under a `flock`; preserves comments + unknown keys; aborts if the file's schema version is newer than this binary knows).
- **`migrate.go`** — the universal chained migrator dsctl→ccp(TSV)→`ccp.yaml`, idempotent, backs up before touching anything.
- **`cfg.go`/`cfg_cmd.go`** — profile-config overlay: `cc-home/CLAUDE.md` = `@import`s of global + overlay; `cc-home/settings.json` = a **pure-Go** deep-merge of global ⊕ overlay (no `jq`). `secrets.go`, `backup.go`, `instruct.go`, `doctor.go`, `status.go`, `shellinit.go` round it out.

### `internal/cli` & `internal/tui`

`internal/cli` is dispatch + text/JSON formatting only. `internal/tui` is a bubbletea+huh app (3 panels: Profiles | Rules | Status) launched when `ccp` runs with **no args and a TTY**; with no TTY it falls to the CLI (never blocks scripting). The TUI only calls `internal/core` — every action has a CLI equivalent.

### Config & state locations (`~/.config/ccp`)

- **`ccp.yaml`** — the single source of truth (replaces `profiles.tsv` + `rules.tsv` + `config` + per-profile `meta` + global/profile `authored.tsv`). Schema `version: 2`. `default` is implicit (never serialized). No inheritance: deepseek profiles store their 4 fields explicitly; `defaults` only seeds new ones.
- `profiles/<name>/api_key` — provider key, `chmod 600`, **never** in `ccp.yaml`, rc, or git.
- `profiles/<name>/cc-home/` — every non-`default` profile's `CLAUDE_CONFIG_DIR`. `CLAUDE.md`/`settings.json` are generated (not symlinked). `ccp profile login <name>` once per official profile.
- `profiles/<name>/overlay/` — the profile's own config (`CLAUDE.md` + `settings.overlay.json`), a lowest-precedence baseline; a repo's `.claude/settings.json` wins.
- `install-source` — repo path for `ccp upgrade`. `.claude/ccp-authored.tsv` (project-scope authored) stays versioned per-repo (ccp never rewrites the user's repo files).

### Distribution

`install.sh` is **Go-aware**: detects OS/arch → downloads the prebuilt release binary and verifies its `sha256` against `checksums.txt`, falling back to `go build` if there's no release but a Go toolchain. It removes the old bash libs (`~/.local/lib/ccp`), re-points `install-source`, and copies `commands/ccp/*.md`. `.github/workflows/release.yml` builds the 4-platform matrix (darwin/linux × amd64/arm64) + checksums on each `v*` tag. `ccp upgrade` re-runs `install.sh` then `profile sync` with the new binary. **Go is not installed on the user's machine → the prebuilt-release path is the effective one.**

## Conventions

- Dispatch is by hand in `internal/cli`. Internal/scripting commands: `_resolve`/`resolve`, `_env`, `_hook`, `completion-shellinit`. Migration fires lazily (`ensureMigrated`) at the top of config-touching commands.
- Output helpers respect `NO_COLOR` and non-TTY (`internal/cli/present.go`). Keep user-facing text Spanish.
- The shell function / hook / completion text lives as **byte-identical string constants** in `core/shellinit.go`. `ccp uninstall` strips the block by the `# >>> ccp shell init >>>` / `# <<<` markers; don't change those markers casually. If you change the shell tail, update the bash oracle in `legacy/` and regenerate the golden too.
- **macOS portability**: Go is portable; `install.sh` stays POSIX-ish bash with no GNU-only coreutils (checksum via `sha256sum` *or* `shasum -a 256`).
- Machine-readable surface (keep stable): `ccp resolve [path]` and `ccp path test [path]` print the profile name and set exit codes (0=non-default, 1=default); `ccp status --json` emits `active`/`profile`/`profile_type`/`cwd`/`repo`.
- **Never commit without explicit user authorization in the current turn**, and never add `Co-Authored-By` trailers (see the user's global instructions).
