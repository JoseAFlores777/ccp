# ccp — Per-path Claude Code profile & account router

> **Spec** · v0.1 — 2026-06-01 · Status: **gate passed, ready to plan** · Scope: rebrand `dsctl`→`ccp`, generalize binary toggle into a named-profile engine that routes each directory to an official Anthropic account or a DeepSeek/provider profile.

---

## 1. Overview

`dsctl` today switches Claude Code between the user's official Anthropic login and DeepSeek, scoped **per terminal** and **per directory** via a precmd hook. The model is **binary**: a path resolves to `deepseek` or `official`.

This spec generalizes that into **named profiles**. A directory resolves to a *profile name*, where a profile is one of:

- an **official** Anthropic account (its own `CLAUDE_CONFIG_DIR`), or
- a **DeepSeek / Anthropic-compatible provider** (its own base URL, key, models), or
- the reserved **`default`** profile (the user's normal `~/.claude` login).

So: repo A → official account *work*, repo B → official account *personal*, repo C → *deepseek*. All configurable from the CLI, with a help command.

The tool is **rebranded** from `dsctl`/`ds` to **`ccp`** (one name for both the binary and the shell function).

## 2. Goals / Non-goals

**Goals**

- Route each path to an arbitrary named profile (≥2 official accounts + ≥1 provider).
- Keep per-terminal, per-directory scoping and the auto-switch-on-`cd` hook.
- Single source of truth for env: the binary emits the env delta; the shell applies it.
- Auto-migrate existing `~/.config/dsctl` state; legacy `on`/`off` verbs keep working.
- Preserve the machine-readable surface (`resolve`, `status --json`) with profile awareness.

**Non-goals**

- No `ANTHROPIC_API_KEY` (Console) profile type in v1 (OAuth + provider only).
- No automation of the OAuth login itself — `ccp` cannot log you in; it routes env and launches `claude` so the user runs `/login` once per official profile.
- No layered/inheriting rule trees (single most-specific-wins rule per path).

## 3. Concepts

### 3.1 Profile

A profile = `name` + `type` + type-specific data.

| Type | Data | Activation (env delta) |
|------|------|------------------------|
| `official` | own `cc-home/` dir | `export CLAUDE_CONFIG_DIR=<cc-home>` + unset all `ANTHROPIC_*` managed vars |
| `deepseek` (provider) | `base_url`, `api_key` (600), `model_pro`, `model_flash`, `effort` | `export ANTHROPIC_BASE_URL/AUTH_TOKEN/MODEL/...` + unset `CLAUDE_CONFIG_DIR` |
| `default` (reserved) | none | unset everything managed (use `~/.claude`) |

`default` always exists, is the fallback when no rule matches, and is assignable to a path to carve an official hole inside a provider subtree.

### 3.2 Resolution

For a path P: among all rules whose path is P or an ancestor of P, the **most specific** (deepest path) wins. Paths are unique, so depth ties are impossible. No matching rule → `default`. (The old "exclude wins exact-depth tie" rule is gone — `exclude` no longer exists.)

## 4. Path rule engine

`rules.tsv` lines become `path<TAB>profile`. The pure-function resolver (today `lib/paths.sh`) keeps most-specific-wins but returns a **profile name** instead of `deepseek|official`. `ds_resolve` → `ccp_resolve` returns the name; unmatched → `default`.

CLI:

```
ccp path set <ruta> <perfil>     # asigna ruta → perfil (reemplaza include/exclude)
ccp path rm  <ruta>              # quita la regla
ccp path list                    # muestra reglas + regla efectiva del cwd
ccp path test <ruta>             # imprime el perfil resuelto (scriptable)
ccp path clear | edit
```

## 5. Env application

The binary runs in a child process and **cannot** mutate the parent shell. So:

- New internal command **`ccp _env <perfil>`** prints an eval-able **full delta**: `unset` every managed var, then `export` the target profile's vars. Stateless — applying any profile's delta from any prior state is correct.
- The `ccp` shell function and the precmd hook `eval "$(ccp _env <perfil>)"`.
- Optimization: a combined **`ccp _hook "$PWD"`** resolves *and* prints the delta in one fork, preserving today's "fork only on `$PWD` change" caching.

Managed vars: `CLAUDE_CONFIG_DIR`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_MODEL`, `ANTHROPIC_DEFAULT_OPUS_MODEL`, `ANTHROPIC_DEFAULT_SONNET_MODEL`, `ANTHROPIC_DEFAULT_HAIKU_MODEL`, `CLAUDE_CODE_SUBAGENT_MODEL`, `CLAUDE_CODE_EFFORT_LEVEL`, plus state marker `CCP_PROFILE`.

> **Security note:** the hook `eval`s the binary's output. The output is generated from the user's own config and is strongly quoted; the binary is trusted. No external input reaches `_env`.

## 6. Official profile config dir & inheritance

`CLAUDE_CONFIG_DIR` relocates **all** of `~/.claude` (plugins, settings, MCP, skills, history). A fresh official profile would start bare. So on `ccp profile add <name> --official`, dsctl seeds `cc-home/` with **selective symlinks** from `~/.claude`:

| Shared (symlink → ~/.claude) | Separate (per profile) |
|------------------------------|------------------------|
| `plugins/`, `commands/`, `agents/`, skills, `CLAUDE.md` | `.credentials`/keychain, `history.jsonl`, `projects/`, `.claude.json` |

**`settings.json` is the delicate one** — Claude Code writes to it (approvals, "use custom API key"). Default: **copy** (not symlink) so per-account writes don't bleed across accounts. (Open item 12.1 — confirm.)

## 7. Storage layout

```
~/.config/ccp/
  rules.tsv               # path<TAB>profile
  profiles.tsv            # name<TAB>type  (index)
  profiles/
    <name>/
      meta                # type + fields (base_url, models, effort)
      api_key             # 600, provider profiles only
      cc-home/            # official profiles only = CLAUDE_CONFIG_DIR
```

## 8. CLI surface

```
ccp                          menú interactivo
ccp use <perfil>             (shell) activa perfil en esta terminal
ccp default | off            (shell) vuelve al login ~/.claude
ccp run [cmd]                (shell) corre cmd/claude con el perfil del cwd
ccp on                       (shell, legacy) alias → use deepseek

ccp profile add <name> --official | --deepseek
ccp profile rm <name>
ccp profile list | show <name>
ccp profile login <name>     lanza claude con el cc-home del perfil para /login

ccp path set|rm|list|test|clear|edit
ccp resolve [ruta]           imprime nombre de perfil (exit 0=no-default,1=default)
ccp status [--json]          --json gana 'profile' + 'profile_type'
ccp install|uninstall|key|config|completion|doctor|version|help
```

`account` is an alias of `profile` (nice-to-have).

## 9. Shell function + hook

The `ccp()` function intercepts `use`/`default`/`off`/`run`/`on` (env mutation) and falls through to `command ccp "$@"` for everything else — mirroring today's `ds()`. The precmd hook (`_ccp_autocheck`) caches by `$PWD`, calls `ccp _hook "$PWD"`, and `eval`s the delta. Completion auto-loads.

> **Live-session caveat:** Claude Code reads env at startup. Changing the profile (via `cd` or `ccp use`) affects the **next** `claude` launch, not a session already running — same as today's DeepSeek behavior. Documented, not fixed.

## 10. Migration (dsctl → ccp)

On first run of `ccp`:

1. If `~/.config/dsctl` exists and `~/.config/ccp` does not: create builtin **`deepseek`** profile from the old `config` (base_url/models/effort) + `api_key`.
2. Convert old rules: `include`→assign `deepseek`, `exclude`→assign `default`.
3. Back up old `rules.tsv`.
4. `ccp install` detects the old `# >>> dsctl shell init >>>` block in the rc and offers to strip it.

Legacy verbs stay: `ccp on`→`use deepseek`, `ccp off`→`default`.

## 11. Scripting surface

- `ccp resolve [ruta]` → prints profile name; exit `0` = non-default profile active, `1` = default. (DeepSeek case preserves old `0`.)
- `ccp status --json` → existing fields + `"profile"`, `"profile_type"`.

## 12. Risks & open items

| # | Item | Status |
|---|------|--------|
| Gate 1 | Two official OAuth accounts coexist on macOS Keychain | **PASSED** (2026-06-01, verified byte-identical token + isolated identities) |
| 12.1 | `settings.json`: copy vs symlink for official dirs | Open — default **copy** |
| 12.2 | Non-interactive login: is `claude /login` a CLI flag or REPL-only? | Open — verify at build; fallback = launch REPL |
| 12.3 | Exact symlink share-list (does CC write into `plugins/`, `commands/`?) | Open — audit before symlinking writable dirs |
| 12.4 | Keychain item keying for custom dirs (4 items observed, parser imprecise) | Low risk — A proven untouched |

## 13. Acceptance criteria

- `ccp profile add work --official` + `ccp profile login work` → logged into a 2nd account without disturbing `default`.
- `ccp path set ~/repoA work && cd ~/repoA && claude` → launches as *work*; `cd ~/repoC` (deepseek) → DeepSeek; `cd ~` → `default`.
- Existing dsctl config auto-migrates; `ds`/`dsctl` rc block strippable; `ccp on/off` still work.
- `ccp resolve` / `status --json` profile-aware; CI smoke test for the path engine updated to profile names.
- `ccp help` documents the full surface in Spanish.

## 14. Phased plan (outline)

1. **Engine**: `lib/paths.sh` → profile-name resolver + tests.
2. **Storage + profiles**: `profile add/rm/list/show`, storage layout, `meta`/`api_key`.
3. **Env**: `ccp _env` / `_hook`, rewrite `ccp()` fn + hook heredoc.
4. **Official dirs**: seed cc-home + selective symlink/copy; `profile login`.
5. **Migration + rebrand**: dsctl→ccp, auto-migrate, install strips old block.
6. **Surface**: `resolve`/`status --json`/completion/menu/doctor/help.
7. **Verify**: end-to-end with `work`/`personal`/`deepseek`.

---

*ccp spec · v0.1 — 2026-06-01*
