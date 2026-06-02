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

**cc-home**:
A profile-private directory used as `CLAUDE_CONFIG_DIR`. Today only official profiles have one (seeded with symlinked plugins/skills + copied `settings.json`).
