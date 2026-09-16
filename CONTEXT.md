# ccp

`ccp` routes Claude Code to a named **profile** per terminal and per directory. This glossary fixes the language so terms don't collide.

## Language

**Profile**:
A named target Claude Code can be routed to. One of: an *official* Anthropic account (owns its `cc-home`), a *deepseek/provider* (env-only: base URL, token, models), or the reserved *default* (the user's `~/.claude` login).
_Avoid_: account (use only as a synonym for an official profile)

**Path rule**:
A `/abs/path<TAB>profile` entry in `rules.tsv` mapping a directory to a profile; deepest match wins. This is what ccp has always called a "rule".
_Avoid_: rule (unqualified — ambiguous with profile config)

**Profile config**:
A per-profile Claude Code **baseline layer** applied whenever the profile is active: own instructions (CLAUDE.md) plus a settings overlay (hooks, permissions, env, MCP). It is the lowest-precedence settings source — a repo's own `.claude/settings.json` wins on conflict — and its instructions are always-injected context, not a hard override. Stored as overlay files; edited from the CLI via the configured editor. Distinct from a path rule.
_Avoid_: rules, reglas, overrides, priority (unqualified — it's a baseline, not an override)

**Overlay**:
A profile's *own* contributions to its profile config, kept as two files (`CLAUDE.md`, `settings.overlay.json`). The effective `cc-home` config is global ⊕ overlay: the memory via `@`-import of `~/.claude/CLAUDE.md`, the settings via `jq` deep-merge.

**Scope**:
Where a captured artifact takes effect. One of *global* (`~/.claude/`, all profiles), *profile* (the active profile's `cc-home`/overlay), or *project* (the repo's `.claude/`). Orthogonal to artifact type.

**Artifact**:
A unit `remember` persists into the **official Claude Code structure** Claude Code already recognizes, at a chosen scope. One of six types: *rule* (a `CLAUDE.md` instruction), *agent* (`agents/*.md`), *command* (`commands/*.md`), *skill* (`skills/`), *hook* (a `settings.json` entry), or *mcp* (a `mcpServers` / `.mcp.json` entry). `remember` is polymorphic: Claude classifies the type and writes it to its native location — ccp invents no custom container. At *profile* scope only rule/hook are writable (overlay-native); mcp is not supported at profile (its native location was not confirmed, so it errors with guidance to use global/project); agent/command/skill are symlinked from global, so they are refused with guidance.

**Instruction** (a *rule* artifact):
A single behavioral directive for Claude ("always X", "never Y") — `CLAUDE.md` content, not a routing entry. The only artifact type stored as a line rather than a file/JSON entry; tracked via a marker block so individual rules stay addressable for `forget`. Distinct from a **path rule**, which routes a directory to a profile.
_Avoid_: rule, regla (reserved for path rule)

**Authored manifest**:
ccp's record of the artifacts it created, so `recall`/`forget` only ever touch ccp-authored items and never hand-made ones. Split by locality: global+profile entries in `~/.config/ccp/authored.tsv` (machine-local); project entries in `.claude/ccp-authored.tsv` (versioned with the repo, so the record travels with the clone).

**cc-home**:
A profile-private directory used as `CLAUDE_CONFIG_DIR`. Every non-`default` profile has one — official **and** deepseek (seeded with symlinked plugins/commands/agents/skills; `CLAUDE.md` and `settings.json` are generated, not copied).

**Backup**:
A portable, restorable snapshot of ccp's state. Two tiers by what they carry: a *config backup* (profiles, path rules, defaults, overlays — no secrets, safe to share/version) and a *full backup* (config **plus** secrets: provider api_keys and official-profile login credentials). Re-seedable, machine-local symlinks are never captured; they are rebuilt on restore. Distinct from the *pre-restore snapshot* and *pre-migration backup*, which are automatic safety copies ccp takes before a restore or a format migration so the prior state stays recoverable.
_Avoid_: export (use only as the verb that writes a backup), dump.

**Desktop instance**:
One isolated Claude Desktop for a profile: its own `--user-data-dir` (account, tokens, MCP, Cowork) **and** its own `CLAUDE_CONFIG_DIR` (the Code tab). Both halves or neither — a window with one and not the other wears one profile's identity while writing another's history, which is the failure mode the whole design exists to prevent. `default`'s instance is the user's normal Claude, in its normal location; it is never relocated.
_Avoid_: window (a UI thing; an instance can have several), copy of Claude (nothing is copied).

**Launcher**:
`~/Applications/Claude (<profile>).app`: the bundle that gives a Desktop instance its own name and icon colour in the Dock, Cmd-Tab and Spotlight. Two layers — an outer one carrying the identity (patched `Info.plist`, own bundle id, the ccp binary as its executable) and an inner *mirror*. Derived state: it is regenerated whole and can be deleted without losing anything. Distinct from the **instance**, which holds the account and survives `ccp desktop app rm`.
_Avoid_: alias, shortcut (it is a real app bundle that executes ccp, not a pointer).

**Mirror** (of Claude.app):
`<launcher>/Contents/ccp/Claude`: a pristine copy of `/Applications/Claude.app` made of real directories and hard-linked files — byte-identical to the signed original, which is what keeps the Keychain trusting the process and therefore keeps the session. Costs no extra disk while the links hold; an update that replaces the source bundle orphans them, which `DesktopAppStale` detects by inode. Distinct from `MirrorForDesktop`, which mirrors a profile's **cc-home**, not the app.

**Bundle identity**:
Which app macOS believes a running process belongs to — the bundle id LaunchServices registers, and therefore what the Dock shows, what `open -a` activates and where a `claude://` link lands. A launcher's instance normally registers its own; it can *collapse* onto Claude's after a relaunch, and from then on opening Claude activates that window instead of the user's. Not something ccp can guarantee (see [ADR 0009](docs/adr/0009-desktop-identity-is-not-durable.md)), only detect.
_Avoid_: name, icon (those are consequences of the identity, not the identity).
