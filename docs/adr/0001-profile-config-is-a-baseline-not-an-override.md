# Profile config is a per-profile baseline, not a hard override

---
Status: accepted
---

The per-profile **profile config** (own `CLAUDE.md` + settings overlay) is applied as the **lowest-precedence** Claude Code settings source — it lives in the profile's `cc-home` (= `CLAUDE_CONFIG_DIR`, the user-level layer), which CC ranks below a repo's `.claude/settings.json` / `.claude/settings.local.json` and enterprise-managed settings. Profile instructions are injected as memory (concatenated context), not a precedence-winning override. So a profile config is a **default that applies whenever the profile is active**, and a repo's own config wins on conflict.

This contradicts the literal request ("reglas prioritarias"), so it's worth recording: CC offers no per-user-dir mechanism to outrank project config, and memory has no hard precedence at all.

## Considered options

- **Baseline (chosen)** — honest to CC's layering; zero hacks; profile config is a safety-net default per profile.
- **Hard override via enterprise-managed settings** — rejected: enterprise-managed is system-wide, not isolated per terminal, which breaks ccp's core per-terminal/per-directory model and is fragile.
- **Instructions-only** — rejected: drops hooks/permissions/MCP, less than asked.

## Consequences

A future reader expecting profile config to beat a repo's `.claude/settings.json` will be surprised; it does not. "Priority" in user-facing copy means *always-present baseline*, deliberately worded imperatively in the instructions, not a precedence win.
