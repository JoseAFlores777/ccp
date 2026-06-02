# `remember` routes captured artifacts to official Claude Code structures

`/ccp:remember-{global,profile,project}` is **polymorphic**: the user-facing verb stays "remember"/"instructions", but Claude classifies the captured intent into one of six artifact types — *rule*, *agent*, *command*, *skill*, *hook*, *mcp* — and writes it to the **native structure Claude Code already recognizes** (`CLAUDE.md`, `agents/*.md`, `commands/*.md`, `skills/`, `settings.json` hooks, `mcpServers`/`.mcp.json`) at the chosen scope. ccp invents no custom container around typed artifacts; the official file or JSON entry *is* the unit. This amends [ADR-0004](./0004-instructions-in-a-ccp-managed-block.md), whose marker block now covers only the *rule* type.

## Why

The point of the feature is a Claude-Code-native config surface, not a ccp-private format: an agent ccp writes must be a real subagent, a command a real slash command, a hook a real `settings.json` entry. A friendly single verb on the user side, the official structure on the Claude Code side.

## Consequences

- **Profile scope is partial.** A `cc-home` symlinks `agents/ commands/ skills/` from global, so a profile cannot own those types. At profile scope only rule/hook/mcp (overlay-native) are writable; agent/command/skill are refused with guidance to use global (already shared to every profile) or project. The `cc-home` seeding model is left unchanged.
- **CRUD needs an ownership record.** Because typed artifacts land in official locations indistinguishable from hand-made ones, `recall`/`forget` operate only over an **authored manifest** of what ccp created — never over hand-authored artifacts. The manifest is split by locality: `~/.config/ccp/authored.tsv` for global+profile (machine-local), and `.claude/ccp-authored.tsv` for project (versioned with the repo so the record travels with the clone).
- **Two record mechanisms coexist.** Rules are addressable via the ADR-0004 marker block inside `CLAUDE.md`; everything else is addressable via the manifest. `forget` resolves a selection to the right mechanism per type.
- Writing typed artifacts (especially agents/commands/skills as whole files, and hook/mcp JSON edits) confirms type + target path + content before writing.
