# Behavioral instructions live in a ccp-managed marker block

> Status: accepted — scope narrowed by [ADR-0005](./0005-remember-routes-to-official-structures.md). The marker block applies **only to the *rule* artifact type** (instructions in `CLAUDE.md`); all other types (`agent`, `command`, `skill`, `hook`, `mcp`) go to their native Claude Code structures and are tracked by the authored manifest, not a block.

We let users persist behavioral **instructions** (`CLAUDE.md` content like "always X") at three scopes — global (`~/.claude/CLAUDE.md`), profile (the active profile's overlay), and project (the repo's `.claude/CLAUDE.md`) — via `/ccp:remember-*`, `/ccp:recall`, `/ccp:forget` slash commands backed by a new non-interactive `ccp instruct add|list|rm <scope>` CLI surface. Each target file gets a single managed region delimited by `<!-- >>> ccp instructions >>> -->` / `<!-- <<< ccp instructions <<< -->` (mirroring the existing `# >>> ccp shell init >>>` convention); ccp owns all mechanical insert/parse/delete inside it, while Claude only supplies the well-phrased instruction text and chooses the scope.

## Considered Options

- **Freeform edits, Claude owns the file** — Claude places each instruction in whatever section reads best and merges semantically. Rejected: nothing is machine-locatable, so `list`/`rm` can't be deterministic or unit-tested, and CRUD was a hard requirement.
- **Managed block, Claude owns it** — keeps semantic grouping but `list`/`rm` depend on Claude re-parsing prose; not testable in `tests/run.sh`.
- **Managed block, ccp owns the mechanics** (chosen) — deterministic, covered by the test harness, consistent with the shell-init marker idiom. Cost: instructions are corralled into one block rather than woven through the document.

## Consequences

- ccp now writes into files it does **not** own — `~/.claude/CLAUDE.md` (global) and a repo's `.claude/CLAUDE.md` (project) — not just its own profile overlays. It confines every mutation to the marker block so the rest of those user/repo files is never touched.
- The block format is a compatibility surface: changing the markers or the per-entry layout later requires a migration for files already carrying a block.
- `remember-profile` (and `recall`/`forget` at profile scope) is undefined when the active profile is `default`, which has no overlay; the command refuses with guidance to use global scope or activate a profile.
