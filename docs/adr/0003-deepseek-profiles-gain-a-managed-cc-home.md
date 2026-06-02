# Deepseek/provider profiles gain a managed cc-home

---
Status: accepted
---

To carry per-profile **profile config** (instructions, hooks, settings, MCP), a profile needs a private `CLAUDE_CONFIG_DIR`. Deepseek/provider profiles were previously **env-only** (base URL, token, models) and rode the shared `~/.claude`. They now get a **managed `cc-home`** — seeded like official profiles (symlinked `plugins/commands/agents/skills`, `@import` `CLAUDE.md`, `jq`-merged `settings.json`) — and `lib/env.sh` now **exports `CLAUDE_CONFIG_DIR` for the deepseek case too**, not just official. The reserved `default` profile is deliberately left untouched: it remains literally the user's raw `~/.claude` login.

## Consequences

`_env`'s deepseek branch changes behavior (a new exported var); deepseek profiles no longer share `~/.claude` state. `default` is the only profile without a managed `cc-home`, so `ccp profile config default` edits the real global `~/.claude` files (with a warning) rather than an overlay.
