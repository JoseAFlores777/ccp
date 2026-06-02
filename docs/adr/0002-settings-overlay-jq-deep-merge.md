# Per-profile settings use an overlay + jq deep-merge

---
Status: accepted
---

A profile's settings are stored as a small `settings.overlay.json` containing **only its own keys**; the effective `cc-home/settings.json` is regenerated as `global ⊕ overlay` via a **`jq` deep-merge** (instructions use CC's native `@import` instead). This introduces `jq` as a **soft dependency** in a codebase that otherwise deliberately avoids dependencies and parses config rather than sourcing it (see `lib/profiles.sh` `key=value`, BSD-portable `awk`). When `jq` is absent, ccp falls back to a snapshot copy of the global settings.

Regeneration runs at edit/create time and on explicit `ccp profile sync` — never in the per-prompt activation hook, which must stay fast (no `jq` on the hot path).

## Considered options

- **Overlay + jq deep-merge (chosen)** — true "inherit global + own" with a minimal stored overlay; cost is the `jq` dependency.
- **Snapshot copy** — copy global once, edit the full file; zero deps but "inheritance" is a one-time photo that drifts (the old official-profile behavior).
- **Full replace** — profile owns a standalone file, no inheritance; simplest but least useful.

## Consequences

`jq` becomes the recommended-but-not-required tool; the snapshot fallback keeps ccp working without it (with degraded inheritance). Editing the overlay never touches the global file. See [ADR-0003](0003-deepseek-profiles-gain-a-managed-cc-home.md) for why deepseek profiles now have a `cc-home` to merge into.
