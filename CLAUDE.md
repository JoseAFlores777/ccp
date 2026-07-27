# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`ccp` (profiles for Claude Code) is a CLI (Go, v2.0) that routes Claude Code to a named **profile** per terminal and per directory — never global. A profile is one of: an **official** Anthropic account (its own `CLAUDE_CONFIG_DIR`), a **DeepSeek/compatible provider** (its own `ANTHROPIC_BASE_URL`/`ANTHROPIC_AUTH_TOKEN` + models), or the reserved **`default`** (the user's normal `~/.claude` login). So repo A → account *work*, repo B → account *personal*, repo C → *deepseek*. User-facing strings are bilingual (English default, Spanish via `ccp lang es` / `CCP_LANG=es`).

`ccp` was **rewritten from Bash to Go** in v2.0 (plan: `docs/superpowers/plans/migracion-go-v2.md`). The Bash implementation is **archived in `legacy/`** and is now the **contract oracle**: the Go binary's observable surface is held byte-identical to it by a golden-diff gate. The chained migrator still auto-upgrades old state: the first Go run migrates `~/.config/dsctl` → `ccp` (TSV) → `ccp.yaml`, backing up first (`~/.config/ccp/.backup-pre-go-*`). The old rc block is unchanged (it only calls `command ccp`), so no rc reinstall is needed.

## Commands

```bash
go build ./...                 # compile (binary: ./cmd/ccp)
go test ./...                  # unit tests (core/cli) + golden gates
gofmt -l internal cmd          # must print nothing (CI gate)
go vet ./...                   # CI gate
golangci-lint run              # CI gate — CI pins v2.12.2 (golangci-lint-action@v8)
# reproduce CI exactly without installing anything:
#   go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --timeout 5m
# `.golangci.yml` is in the v2 schema (`version: "2"`); a v1 binary cannot read it.

bash legacy/tests/run.sh       # bash ORACLE suite (still a CI gate)
bash testdata/golden/capture.sh --check   # oracle reproduces committed golden
bash testdata/golden/capture.sh           # regenerate golden from the oracle
```

The **parity gate** lives in `internal/golden/parity_test.go`: it builds the Go binary, runs it over the `testdata/golden/basic` fixture, and asserts its stdout + exit code match the committed `expected/` (which were captured from the bash oracle by `capture.sh`). Green ⇒ Go == bash on the frozen contract (`_env`, `_hook`, `resolve`, `path test`, `status --json`, `completion bash|zsh`, `completion-shellinit`). When you change any of those, update the bash oracle in `legacy/`, regenerate the golden with `capture.sh`, and keep the parity test green.

Always run the binary in tests with a temp `CCP_HOME` (an existing dir) so auto-migration does not fire against real `~/.config/dsctl`.

Auto-handoff surface (added on top of the frozen contract). `session` and `auto` are **not** shell-only: they reach the binary through the `*) command ccp "$@"` fallthrough, so no rc reinstall. `handoff prune`/`sessions` **do** need one — `handoff` is intercepted by the shell function, and the read-only passthrough list in `ShellInit` had to gain `prune|sessions` (an rc installed before that forwards them to `_handoff` as if they were a target profile; the binary detects it and says to run `ccp install`).

```bash
ccp session [-p|--headless] [--policy <n>] [--max-hops N] [--yolo] \
            [--session <uuid>] [--dry-run] [--no-return] [--claude-bin <path>] [-- <claude args>]
ccp auto init [--force] | install [<profile>…] | uninstall [<profile>…] | status [--json] | test [--profile <n>]
ccp handoff prune [--keep N] | sessions [--json]
ccp _statusline [-- <wrapped cmd>]   # internal sensor: persists rate_limits, always exits 0
ccp _limit-hook                      # internal StopFailure hook: writes a sentinel, always exits 0
```

`ccp session` exit codes: `0` ok · `1` usage/config · `2` handoff I/O (`core.ErrHandoffIO`) · `75` all profiles exhausted (`supervisor.ParkedExitCode`, EX_TEMPFAIL) · otherwise the child's own code.

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
- **`cfg.go`/`cfg_cmd.go`** — profile-config overlay: `cc-home/CLAUDE.md` = `@import`s of global + overlay; `cc-home/settings.json` = a **pure-Go** deep-merge of global ⊕ overlay ⊕ **auto layer** when `AutoHooksEnabled` (no `jq`). `secrets.go`, `backup.go`, `instruct.go`, `doctor.go`, `status.go`, `shellinit.go` round it out.
- **`envpairs.go`** — `EnvPairs`/`EnvForChild`: the same delta `env.go` emits, as data instead of shell text. `EnvForChild(os.Environ(), …)` is how the supervisor gives a child process a profile's environment without a shell to `eval` in.

Auto-handoff engine (all of it additive; `ccp.yaml` stays at schema `version: 2` and `handoffs.yaml` at `2`, so an older binary preserves the new keys instead of refusing the file):

- **`auto.go`** — the `auto_handoff` block: `AutoPolicy.Effective` (defaults + validation, duration strings parsed once) and `ResolveAutoChain(home, cfg, policy, cwd)` → `{Policy, Primary, Fallback, Denied}`. `Primary` is `Resolve(cwd, cfg.Rules)` and is dropped from `Fallback` silently (it's implicit). `AutoInit` seeds the block. The `allow_from` gate has three states and the third is the point: **map absent/empty ⇒ no gate; declared with an entry for the primary ⇒ only that entry passes; declared without an entry ⇒ total deny.**
- **`ratelimit.go`** — pure parsers, no I/O: `ClassifyLimitText` (window from CC's prose), `ParseTranscriptLine`, `ParseStreamJSONLine`, `ParseStatusLineInput`, `ReadCachedUsage(ccHome)` (`.claude.json` → `cachedUsageUtilization`), and `RateLimits.Exhausted(threshold)`. Everything normalizes to a `LimitEvent{Window, ResetsAt, Source, Detail}`.
- **`autostate.go`** — sensor state under `<home>/state/auto/`: `rate-limits/<profile>.json` (last statusLine sample) and `sentinels/` (what the StopFailure hook leaves). Profile/session names are sanitized to `[a-zA-Z0-9._-]` before they become filenames — the values come from a hook payload, i.e. from outside.
- **`autohooks.go`** — the managed settings.json layer: `AutoHooksFragment` (`hooks.StopFailure` → `ccp _limit-hook`, `statusLine` → `ccp _statusline -- <the user's own>`), `ExtractStatusLineCommand` (returns `""` for our own command, which is what stops the wrapper from wrapping itself every regeneration), and `keepForeignStopFailure` (needed because `MergeJSON` *replaces* arrays). The binary path is package state (`SetAutoHooksBin`, injected once from `internal/cli` via `os.Executable`) so `auto install` and `profile sync` can't write two different values.
- **`handoff_chain.go`** — `HandoffChain` lifts the no-chain invariant *for the supervisor only* by **mutating the live marker in place** (`To = to`, `Hops += to`; `From`/`Since` untouched) instead of stacking a level, so `handoff end` still comes home in one step. Fan-out stays forbidden. `HandoffEndSession` is `HandoffEnd` minus the shell emit (returns the closed marker and the new uuid); `HandoffPrune(home, keep)` trims `archived`.

### `internal/supervisor` (the `ccp session` loop)

Runs `claude` as a child, watches for a usage limit, hands the session off and relaunches. Split so the dangerous part — rotating profiles at 3am with nobody watching — is auditable in one file:

- **`policy.go`** — `Chain`, the rotation state machine. **Pure**: no clock, no disk, every instant arrives as a parameter, so a 5-hour cooldown is testable in microseconds. `Next()` encodes the pendulum: *the primary if its cooldown expired* (`ReturnDue`) even when fresh loans remain, then the max-hops budget (backstop), then the first available fallback. `max_hops` counts **loans**: `Advance` does not charge a trip home and `Next`/`ReturnDue` do not let an exhausted budget block one — stranding the conversation in someone else's account with the primary free is worse than one extra relaunch, and the backstop still holds because every return must be preceded by a loan that did pay (≤ `2·max_hops + 1` launches). `ReturnDue` is also what the `return_check` timer asks. `MarkExhausted` treats a stale or zero `resets_at` as absent (a `resets_at` in the past would make a profile available the instant it was marked exhausted → ping-pong).
- **`detect.go`** — the sensors, all behind `Detector{Events() <-chan core.LimitEvent; Close() error}`: `NewStreamDetector` (parses `claude -p`'s stream-json *while teeing it verbatim* to the user), `NewTranscriptWatcher` (tails the `.jsonl`; never consumes a half-written line), `NewSentinelWatcher` (the StopFailure hook's sentinels, filtered by session), `NewUsageWatcher` (the only **proactive** one: statusLine sample, `.claude.json` as fallback, fires once per crossed window), and `MergeDetectors`. Contract: the goroutine is the only one that closes `events`; `Close` only closes `quit` and never blocks.
- **`launcher.go`** — process, signals and tty. The child shares the supervisor's **process group** (no `Setpgid`, no `TIOCSPGRP`): it is already the tty's foreground process, so `Ctrl-C` reaches it, and the supervisor just ignores `SIGINT` so it survives to report the result. Saves/restores `termios` (killing claude mid-TUI would otherwise leave the shell in raw mode). `BuildArgs` is pure and defers to the user: it never duplicates a `--resume`/`--session-id`/`-p`/`--output-format` the user already passed.
- **`supervisor.go`** — the loop, and the **only** place that mutates disk state. Invariant: never two children alive at once. Pre-assigns the session uuid (`--session-id`) so it knows which transcript to watch. On a limit: honour `min_dwell` → `Terminate(10s)` → `MarkExhausted` → `Next` → `HandoffEndSession` if the target is home, `HandoffChain` otherwise, else park (exit 75). Deduplicates events by content across the whole run, because `HandoffChain` copies the `.jsonl` to the destination and the next launch would re-read the very line that caused the previous hop.
  - **The `return_check` timer** is the second reason to kill the child, and the only one nobody reports: sensors say "*this* is exhausted", never "*that other* account freed up". Its only action when it fires is `Terminate(10s)` on a **healthy** `claude` plus a relaunch in the primary — so the arming condition is a guard, not bookkeeping. `armReturnTicker(onLoan, noReturn, check)` holds it: on loan **and** the live marker's `From` is the primary (`onLoan`), `--no-return` off, `return_check > 0`. That `onLoan` guard is load-bearing: with a **degraded** loan (rotation with no transcript ⇒ no marker) there is no loan to close, and arming there would move the conversation for a reason nobody asked for. It is a separate function because arming-too-much has **no observable effect** in most states (`ReturnDue` is false at home, so an end-to-end test can't tell it apart) — the condition has to be asserted by name (`TestArmReturnTickerSoloConPrestamoVivo`). It ticks at `o.Poll` and gates on `o.Now()` so the period is the policy's (10m) while tests drive an injected clock. Firing sets `childOutcome.returnHome`, which the loop handles **before** the rotation branch and **without** `MarkExhausted` on the current profile: we left it voluntarily, it keeps its credit. The trip home itself is the same `HandoffEndSession` + new uuid + resume as the limit-driven one.
  - **`decideReturn`** is that timer's whole decision, extracted from the `select` so each rule is testable without launching anything — because this is the only move that kills a *healthy* child. In order: a already-dequeued limit yields to the pending rotation; `Chain.ReturnDue`; `DwellSatisfied`; **`sessionIdle`** (the transcript's mtime must be `return_idle` old — 90s by default; a missing transcript is **not** idle, and `return_idle: 0s` is the explicit opt-out); and finally a **non-blocking drain** of the event channel, because a `LimitEvent` the `select` had not dequeued yet would be lost when the child dies (Go picks randomly between ready channels) and the profile we leave would never reach `MarkExhausted` — it would look fresh to the next `Next()` and burn a hop on an account still in 429.
  - **Going home never creates an inverted marker.** `backHome` (`target == Primary()`) is separate from `home` (`backHome` *with a live marker*). Without that split, returning from a degraded loan fell through to `HandoffChain(loan → primary)`, which opens an ACTIVE `{From: loan, To: primary}` marker that can never be closed while at home — and the next clean exit would back-sync the conversation *towards* the loan profile, leaving the transcript where the user is not and a trace that lies about where to `--resume`. `runner.adoptHome` handles it instead: no transcript ⇒ fresh uuid in the primary (`resume=false`); transcript ⇒ `core.HandoffAdoptHome` (RewriteSession under a new uuid, **no** marker, no `archived` entry). The `returnHome` branch degrades to the same helper if it ever runs without a marker, rather than calling `HandoffEndSession` and dying with an empty `%s`.
- **`trace.go`** — presentation. The hop trace goes to `Out` (it *is* the command's output, and in headless `Out` also carries the child's stream-json); operational chatter goes to `Err`. The package does not speak i18n on purpose — `internal/cli` owns the catalog. Every movement of the conversation goes through **one** formatter, `traceMove`, whichever of the two things caused it: the budget is named the same way always (`préstamo N/M` outbound, `vuelta a casa, no gasta préstamo: siguen N/M` home), the trip home is never called a *préstamo*, and because it doesn't charge budget the repeated number is explained in the line instead of left dangling. Sharing the formatter is the point — two entry points printing the same event two ways is exactly the bug `trace_test.go` now pins.

### `internal/cli` & `internal/tui`

`internal/cli` is dispatch + text/JSON formatting only. `internal/tui` is a bubbletea+huh app (3 panels: Profiles | Rules | Status) launched when `ccp` runs with **no args and a TTY**; with no TTY it falls to the CLI (never blocks scripting). The TUI only calls `internal/core` — every action has a CLI equivalent.

### Config & state locations (`~/.config/ccp`)

- **`ccp.yaml`** — the single source of truth (replaces `profiles.tsv` + `rules.tsv` + `config` + per-profile `meta` + global/profile `authored.tsv`). Schema `version: 2`. `default` is implicit (never serialized). No inheritance: deepseek profiles store their 4 fields explicitly; `defaults` only seeds new ones.
- **`auto_handoff:` (in `ccp.yaml`)** — the auto-rotation policy. `enabled` (master switch for `ccp session`), `policies.<name>` (`fallback` = the loans *in order*, primary implicit; `threshold` %, `min_dwell`, `max_hops`, `return_check`, `return_idle`, `cooldown.{strategy,fallback}`), `allow_from` (the compliance gate, see `auto.go` above) and `hooks` (the list of profiles whose `settings.json` carries the sensor layer — the source of truth for that, **not** the generated file). Additive: `version` stays at `2`, so an older binary round-trips the block through `Config.Extra`.
- `handoffs.yaml` markers gained `auto: true` (created by the supervisor) and `hops: [...]` (the trail of destination profiles). Both `omitempty`, `HandoffsVersion` still `2`.
- `state/auto/rate-limits/<profile>.json` — the last statusLine sample (`used_percentage` + `resets_at` per window, plus when it was taken). `state/auto/sentinels/*.json` — what `ccp _limit-hook` leaves for the supervisor to pick up. Both are caches: deleting them costs detection quality, never correctness.
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
- Machine-readable surface (keep stable): `ccp resolve [path]` and `ccp path test [path]` print the profile name and set exit codes (0=non-default, 1=default); `ccp status --json` emits `active`/`profile`/`profile_type`/`cwd`/`repo`; `ccp auto status --json` emits `enabled`/`cwd`/`policy`/`primary`/`fallback`/`denied`/`sensors` (+ `error` when the policy doesn't resolve — the JSON stays valid and the exit code is 1), with `fallback`/`denied`/`sensors` always arrays, never `null`.
- The two internal sensors (`ccp _statusline`, `ccp _limit-hook`) must **always exit 0** and never fail loudly: a statusLine that breaks leaves Claude Code without a status bar, and a hook that fails nags the user on every turn. `_statusline` also deliberately skips `ensureMigrated` — it runs several times a minute from inside CC, and migrating the user's config from a sensor would be mutating state nobody asked to mutate.
- The auto layer writes the **absolute** path of the running binary (`os.Executable`) into each profile's `settings.json`. Moving the binary without re-running `ccp upgrade` / `ccp auto install` / `ccp profile sync` leaves the sensors pointing at the old path.
- **Never commit without explicit user authorization in the current turn**, and never add `Co-Authored-By` trailers (see the user's global instructions).
