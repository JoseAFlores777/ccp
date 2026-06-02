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
A unit `remember` persists into the **official Claude Code structure** Claude Code already recognizes, at a chosen scope. One of six types: *rule* (a `CLAUDE.md` instruction), *agent* (`agents/*.md`), *command* (`commands/*.md`), *skill* (`skills/`), *hook* (a `settings.json` entry), or *mcp* (a `mcpServers` / `.mcp.json` entry). `remember` is polymorphic: Claude classifies the type and writes it to its native location — ccp invents no custom container. At *profile* scope only rule/hook/mcp are writable (overlay-native); agent/command/skill are symlinked from global, so they are refused with guidance.

**Instruction** (a *rule* artifact):
A single behavioral directive for Claude ("always X", "never Y") — `CLAUDE.md` content, not a routing entry. The only artifact type stored as a line rather than a file/JSON entry; tracked via a marker block so individual rules stay addressable for `forget`. Distinct from a **path rule**, which routes a directory to a profile.
_Avoid_: rule, regla (reserved for path rule)

**Authored manifest**:
ccp's record of the artifacts it created, so `recall`/`forget` only ever touch ccp-authored items and never hand-made ones. Split by locality: global+profile entries in `~/.config/ccp/authored.tsv` (machine-local); project entries in `.claude/ccp-authored.tsv` (versioned with the repo, so the record travels with the clone).

**cc-home**:
A profile-private directory used as `CLAUDE_CONFIG_DIR`. Every non-`default` profile has one — official **and** deepseek (seeded with symlinked plugins/commands/agents/skills; `CLAUDE.md` and `settings.json` are generated, not copied).
