<div align="center">

# ccp

**Profiles for Claude Code.**

**English** · [Español](README.es.md)

**A different Claude Code account in every folder.**
In your work repo, your company account; in your personal project, your own; in your experiments, DeepSeek.
The switch happens on its own, just by `cd`-ing.

![version](https://img.shields.io/badge/version-2.18.0-c96442)
![platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-c96442)
![shell](https://img.shields.io/badge/shell-bash%20%7C%20zsh-8a8378)
![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![license](https://img.shields.io/badge/license-MIT-8a8378)

<img src="docs/screenshots/tui.png" alt="ccp interactive dashboard" width="760">

</div>

---

## What it is

`ccp` routes Claude Code to a **profile** per terminal and per folder — never global. A profile is one of three things:

- an **official** Anthropic account (its own isolated `CLAUDE_CONFIG_DIR`),
- a **compatible provider** (its `ANTHROPIC_BASE_URL` + API key) — built-in presets for **DeepSeek**, **Kimi** (Moonshot) and **GLM** (Z.ai), or
- the reserved **`default`**: your normal `~/.claude` login.

So: repo A → *work* account, repo B → *personal* account, repo C → *deepseek*. Without touching anything by hand.

## The mental model (30 seconds)

1. A **profile** is an identity (official, provider, or `default`).
2. A **rule** says "this folder (and its subfolders) uses such-and-such profile".
3. The **most specific** rule wins. No rule → your normal login.

```text
~/work          → perfil "work"      (cuenta empresa)
~/work/cliente  → perfil "default"   (carve-out: tu login normal)
~/personal      → perfil "personal"  (cuenta personal)
~/labs          → perfil "deepseek"
~               → default
```

When you enter a folder, `ccp` applies the right profile in that terminal via a hook. Then you run `claude` as usual.

## The interface

Run `ccp` with no arguments (with a TTY) and you get the **interactive dashboard** from the screenshot above: three panels (Profiles · Rules · Status) with keyboard navigation, health indicators (`✓` login / key), and a `:` command bar with **autocompletion** (Tab). Every action has its equivalent CLI command.

Press `c` (or `:config`) for the **Config view**, which takes over the body rather than adding a fourth panel — at 80 columns the three existing ones are already tight. Five sections: Defaults, Auto-handoff, Chain (reorder with `J`/`K`), allow_from, and Sensors. `e` opens the whole config in your graphical editor. The view never reimplements a rule: editing the chain from here goes through the same `core` functions as `ccp auto chain`, gate included.

Press `e` on a profile for its **profile view**: what configuration actually applies to it, and which layer each value comes from (global, overlay, or the auto-handoff sensor layer). Three boxes — Instructions, Env, and Effective (permissions, hooks, plugins, sensors, folded to their counts; `enter` expands one). `a`/`d` edit rules and variables through the same `core` functions the CLI uses; hooks are added the same way but can't be deleted from here — they live in arrays with no stable id, so the key explains why instead of pretending. `e` inside the view opens just that box's own file in your editor.

No TTY, or prefer the terminal? Everything is in the CLI, with the same palette:

<div align="center">
<img src="docs/screenshots/cli-help.png" alt="ccp help — colored CLI" width="620">
</div>

Prefer a window? There is also a **desktop app** (`gui/`, Tauri, beta): the same accounts, folders, conversations, rotation and Desktop windows, a **Configuration** screen where every layer of your Claude config is read and edited in one place, a **Snapshots** screen with the history of that configuration — including the plan of any restore, before it writes — a **Cloud** screen where you confirm what the portal proposed that runs code, and an **account map** where fallbacks are wired by dragging arrows and nothing is written until you review and apply. It runs the engine you already have (`ccp serve --stdio`), shows the CLI equivalent of every screen, and opens a Terminal for the things only a terminal can do (a `/login`, a handoff, a supervised session). Build and development notes: [gui/README.md](gui/README.md).

---

## Before you start

- macOS or Linux with **bash** or **zsh**.
- **Claude Code** installed (`claude --version` should work).
- **git**.

## Installation

One line, nothing to clone:

```bash
curl -fsSL https://raw.githubusercontent.com/JoseAFlores777/ccp/main/install.sh | bash
```

Then, just once:

```bash
ccp install           # 2. shell function + automatic hook
source ~/.zshrc       # 3. reload YOUR shell (or ~/.bashrc)
ccp doctor            # 4. confirm it landed well
```

If step 1 warns you that `~/.local/bin` is not on your PATH, add it to your rc:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

<details>
<summary>What that one line actually does — and how to pin, redirect or read it first</summary>

The installer **downloads the prebuilt binary** for your OS/arch (darwin/linux ×
amd64/arm64) from the GitHub Release and **verifies its sha256** against the
release's `checksums.txt`; a mismatch aborts the install. Because there is no
checkout around it, it also fetches the repo into `~/.config/ccp/src`
(`git clone --depth 1`, or a tarball if you have no git) — that copy is what the
`/ccp:` slash commands are installed from and what `ccp upgrade` re-runs later.

Prefer to read before you run, which is always the right instinct with `curl | bash`:

```bash
curl -fsSL https://raw.githubusercontent.com/JoseAFlores777/ccp/main/install.sh -o install.sh
less install.sh && bash install.sh
```

Env knobs (all optional, all also valid from a clone):

| Variable | Default | What it does |
|---|---|---|
| `CCP_RELEASE` | `latest` | Install a specific tag: `curl … \| CCP_RELEASE=v2.18.0 bash` |
| `CCP_BIN_DIR` | `~/.local/bin` | Where the binary lands |
| `CCP_SRC_DIR` | `~/.config/ccp/src` | Where the source copy lands |
| `CCP_NO_SOURCE` | `0` | `1` = binary only, no source copy (no `/ccp:` commands, no `ccp upgrade` source) |
| `CCP_FROM_SOURCE` | `0` | `1` = build with `go build` instead of using the release (needs Go) |
| `CCP_REPO` | `JoseAFlores777/ccp` | Install from a fork |

From a clone it's the same script and the same result — the only difference is
that `ccp upgrade` is then registered against **your** checkout:

```bash
git clone https://github.com/JoseAFlores777/ccp.git && cd ccp && ./install.sh
```

</details>

## Updating

`install.sh` records which source it installed from (`~/.config/ccp/src` with the
one-liner, your checkout with a clone), so updating is a single command:

```bash
ccp upgrade              # reinstall + resync profiles (profile sync)
ccp upgrade --pull       # 'git pull' before reinstalling
ccp upgrade --no-sync    # binary only, leave profiles alone
ccp upgrade --from-source  # build the registered repo instead of the release
```

`upgrade` installs the **latest release** published on GitHub; `--from-source`
builds whatever is in your checkout (needs Go), which is what you want to try a
change before tagging it.

When a version changes the **shell function** (the block in your rc), the upgrade
warns you and it has to be refreshed — a new binary alone cannot touch your
shell:

```bash
ccp install && source ~/.zshrc   # rewrites the stale block in place
```

If you're coming from the Bash version (`dsctl`) or an old `ccp`, **migration is automatic and lazy**: the first time you run any command that touches the config, it converts your state to `ccp.yaml` (schema v2), backing it up first in `~/.config/ccp/.backup-pre-go-<fecha>`.

---

## Usage

### Case 1 — Your work account in your work folder

```bash
ccp profile add work --official    # 1. crea el perfil oficial
ccp profile login work             # 2. una vez: dentro, /login y /quit
ccp path set ~/work work           # 3. asigna la carpeta
cd ~/work && claude                # 4. arranca con tu cuenta de trabajo
```

### Case 2 — Add your personal account

Same as case 1, with a different name and a different folder:

```bash
ccp profile add personal --official
ccp profile login personal
ccp path set ~/personal personal
```

### Case 3 — A folder with a compatible provider (DeepSeek / Kimi / GLM)

```bash
ccp profile add deepseek --deepseek   # perfil de proveedor (preset DeepSeek)
ccp profile add kimi     --kimi       # Kimi (Moonshot): base_url + modelos preconfigurados
ccp profile add glm      --glm        # GLM (Z.ai): base_url + modelos preconfigurados
ccp key deepseek                      # guarda la API key (te la pide oculta)
ccp path set ~/labs deepseek
cd ~/labs && claude
```

Each preset fills in the right `ANTHROPIC_BASE_URL`, default models and the
provider-recommended tuning vars (Kimi: `ENABLE_TOOL_SEARCH`,
`CLAUDE_CODE_AUTO_COMPACT_WINDOW`; GLM: `API_TIMEOUT_MS`,
`CLAUDE_CODE_AUTO_COMPACT_WINDOW`). Override any field with
`--base-url --pro --flash --effort`.

### Case 4 — Carve a subfolder out of its rule (carve-out)

Your `~/work` uses the work account, but there's one specific client where you want your normal login:

```bash
ccp path set ~/work/cliente-x default
```

`~/work` stays on "work", but `~/work/cliente-x` uses your normal login. The most specific subfolder always rules.

### Case 5 — Switch by hand in a terminal

```bash
ccp use personal      # activa un perfil aquí
ccp default           # vuelve a tu login normal
ccp run claude        # corre Claude una vez con el perfil del cwd, sin fijarlo
```

### Case 6 — See what's going on

```bash
ccp status            # perfil activo + perfil del cwd
ccp path list         # tus reglas de carpeta
ccp profile list      # tus perfiles
ccp doctor            # logins, keys, función de shell
```

---

## Handoff — continue a session under another profile

A live `claude` process freezes its credentials at startup, so `ccp use` can't hot-swap them. When a profile runs out of tokens/quota mid-conversation, `ccp handoff` does the only clean thing: it **persists the context → switches profile → resumes the same conversation** in a fresh process with the target profile's tokens.

You can keep **several handoffs in flight at once** — one per session, across as many repos as you like.

```bash
ccp handoff                       # manager panel: see what's in flight, pick an action
ccp handoff <to>                  # new handoff towards <to> (session picker)
ccp handoff <to> --session <uuid> # skip both pickers (scriptable)
ccp handoff resume  [<uuid>]      # re-enter a live handoff without closing it
ccp handoff end     [<uuid>]      # bring the updated context back to the origin and resume there
ccp handoff discard [<uuid>]      # drop a marker without bringing anything back
ccp handoff status  [--all]       # what's in flight here (or everywhere)
ccp handoff list                  # active + history (archived)
```

The mental model: **you borrow another profile's tokens for a session, and return the work when you come back.** `handoff end` back-syncs the updated context to the origin profile as a **new session** (non-destructive); the returned session shows `[from <profile>]` in its title. `handoff resume` is the opposite: it re-enters a handoff that is still alive without copying anything or closing it — that's what makes having several in flight useful.

**How `end`/`resume`/`discard` pick which handoff:** by the current directory. Exactly one active handoff for this repo → it acts on that one, no questions asked. Several → it asks (marker picker with a TTY; without a TTY it fails asking for `--session <uuid>`). None here → it fails and tells you which repos do have one. Passing the `<uuid>` explicitly skips the resolution entirely.

**When the transcript is gone — `handoff discard`:** if the session's jsonl no longer exists in the target profile (you cleaned `~/.claude`, deleted the profile…), `end` and `resume` have nothing to work with and always fail, and the marker would stay active forever — hijacking this repo's directory resolution, counting towards the five-handoff warning and blocking a new handoff from the target profile. `ccp handoff discard` archives that marker **without any back-sync**: no transcript is copied or rewritten, nothing is deleted from the target profile (whatever is left there can still be reached with `claude --resume <uuid>` from that profile), and your shell's profile doesn't change. It is not the normal way to close a handoff — that's `end`, which actually brings the work back.

**Skipping permission prompts:** all three launching commands (`handoff`, `handoff resume`, `handoff end`) accept `--dangerously-skip-permissions`, aliased `--yolo`: the resumed session starts without Claude Code's permission prompts. It is **not remembered** between invocations — it lives neither in the marker nor in `ccp.yaml`, so you ask for it every time (or toggle it with `y` in the panel).

`ccp handoff` with no arguments and a TTY opens the **manager panel**: active handoffs with this repo's first, `enter` resume · `e` end (asks for confirmation) · `n` new · `y` toggle skip-permissions · `q` quit. With nothing in flight the panel doesn't open at all: you go straight into the new-handoff wizard, profile → session.

Chained handoffs **by hand** are still refused (`A → B → C` on the same session — finish that one with `end` first; lending a *different* session of the same repo onwards is fine), and so is lending one session to two profiles at once. The auto-handoff supervisor *does* chain (see below), but it does it without stacking levels. Past five handoffs without closing, a new one warns you (it doesn't block). Entering a repo that has a live handoff prints a one-line reminder. `status`, `list` and `discard` work anywhere without launching anything — `handoff status` exits `0` when this repo has an active handoff and `1` when it doesn't; the commands that resume a session (`handoff`, `resume`, `end`) run through the ccp shell function, so `ccp install` must be active.

Two housekeeping subcommands round it off: `ccp handoff sessions [--json]` lists this directory's sessions in the active profile (that's the picker's data, scriptable), and `ccp handoff prune [--keep N]` trims the archived history — it grows one entry per closed handoff and nothing ever removed them (`--keep` defaults to 50; `--keep 0` wipes it). Both are read-only-ish and don't need a TTY, but a shell function installed *before* they existed forwards them as if they were a target profile — if you get an error saying so, run `ccp install` and open a new terminal.

---

## Auto-handoff — rotate profiles on their own when usage runs out

`ccp handoff` is the manual answer to "this account ran out". `ccp session` is the automatic one.

The problem it solves: a long session (a big refactor, an overnight batch) dies when the account hits its 5-hour, weekly or Opus limit. And a live `claude` **cannot** change profile — `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` and `CLAUDE_CONFIG_DIR` are read once at startup. So the only clean fix is a **supervisor** that runs `claude` as a child, watches for the limit, hands the session off to another profile, and relaunches it there.

```bash
ccp auto init                     # seed the auto_handoff block in ccp.yaml
ccp auto install                  # install the sensors in every non-default profile
ccp session --dry-run             # what would it do here? (chain, thresholds, samples)
ccp session                       # interactive: claude under the supervisor
ccp session -p --policy overnight -- "refactor the parser"   # headless (cron/CI)
ccp auto status [--json]          # resolved policy, sensors, last samples, cooldowns
ccp auto test [--profile <n>]     # is the detection path actually wired?
```

`ccp session` reaches the binary through the `*) command ccp "$@"` branch the shell function already has, so **no `ccp install` refresh is needed** for it.

In practice you can skip the first two lines: run `ccp session` in a repo that isn't set up and it lists what's missing — the rule, the policy, the sensors — shows the exact path it would write, and asks once. Answer and it applies each gap through the same code path as the command you'd have typed. It asks **once per repo**, whatever you answer; `--setup` offers it again, `--no-setup` skips it.

It never asks, and never writes, unless **both ends of the conversation are a terminal**. With `-p`, without a TTY, or with the output redirected to a file, it just says why and carries on as before. `ccp session -p` runs from cron: a prompt there hangs the job, and a prompt written to a log is worse — you would never see the question your Enter answered.

### The cycle: primary → loan → back to the primary

The **primary** is whatever `ccp resolve $PWD` returns — the natural owner of the directory. Every other profile in the chain is a **temporary loan**. The supervisor is a pendulum, not a round-robin: it swings out when the primary is exhausted and comes back the moment the primary's window reopens, even if fresh loans are still available. Working for hours in the wrong account is worse than waiting.

```
personal-1  ──[limit]──→  work-2  ──[limit]──→  personal-deepseek
                                      │
                                      └──[primary's window reopened]──→ personal-1
```

The trip home is not a reverse handoff: it is `handoff end`, which back-syncs the conversation to the primary **as a new session with a new uuid** (non-destructive, the old transcript stays where it is). The supervisor prints that new uuid — it's what you'd type into `claude --resume` to continue by hand. When the chain runs dry it stops with exit code **75** (`EX_TEMPFAIL`, "retry later") and a table of when each profile frees up; the marker is left alive so nothing is lost.

There is one loan that has no marker to close: a rotation that fired *before* the conversation existed (the proactive sensor can trip on a session that hasn't had its first turn) has nothing to lend, so no handoff is opened. Coming back from one of those doesn't run `handoff end` at all — if a conversation was born during the loan it is **adopted** into the primary as a new session, and if none was, the primary simply starts fresh. Either way the trace says which happened. What never happens is a marker pointing the wrong way (`{from: loan, to: primary}`), which would send the next clean exit — and you — to the wrong account.

While the session is on loan, a **`return_check` timer** asks on its own, every N minutes, whether the primary's window has reopened — nobody has to hit a limit for you to come home. That is the whole point: the primary's 5-hour window reopens at 3am, and no sensor fires when an *other* account frees up, so without the timer the run would spend the night in the loan. The trip home still honours `min_dwell` (you are not yanked out of a loan you arrived at 30 seconds ago), and the profile you leave is **not** marked exhausted — you left it voluntarily, it keeps its credit:

```
p2 (2h 00m) ──[return_check: personal-1 ya liberó su ventana]──→ volviendo a personal-1 (vuelta a casa, no gasta préstamo: siguen 1/6)
```

**The trip home waits for silence (`return_idle`, default 90s).** This is the only move the supervisor makes for its own reasons: rotating on a limit kills a child that *cannot work any more* (its account is answering 429), but coming home would kill one that works fine. With the defaults alone (`min_dwell: 20m` + `return_check: 10m`) any loan longer than twenty minutes would end the instant the primary's cooldown expired — while you are typing, mid-turn, or with a tool call in flight, and an interrupted tool call is re-run by `--resume` and may not be idempotent. So the timer additionally requires the session to be **idle**: the transcript (which grows with every turn and every tool result) must not have been touched for `return_idle`. If it never goes quiet, nothing is forced — the timer just keeps offering; you come home when you stop, when the child exits on its own, or at the next limit. Missing a chance to come home is cheap; taking the terminal away mid-sentence is not. A transcript that does not exist does **not** count as idle (we know nothing, and killing out of ignorance is the thing being avoided), and `return_idle: 0s` is an explicit opt-out. The rule applies in headless (`-p`) too: nobody watching does not make a half-finished tool call any less fragile.

```
ccp session: p1 ya liberó su ventana; la sesión sigue activa, se volverá cuando lleve 1m30s en silencio
```

**`max_hops` counts loans, not moves.** Coming home is the *closing* of a loan, so it neither consumes budget nor is blocked by an exhausted one — otherwise `max_hops: 6` would mean "three round trips" and a spent budget would strand the conversation in someone else's account with the primary sitting free. It is still a hard anti-loop backstop: every trip home has to be preceded by a loan, and that one does pay, so a run can never make more than `2 × max_hops + 1` launches.

### Configuring it (`auto_handoff` in `ccp.yaml`)

`ccp auto init` seeds this from your existing profiles; edit it by hand afterwards.

```yaml
auto_handoff:
  enabled: true              # master switch: false = ccp session refuses to run
  hooks: [personal-1, work-2]    # profiles with the sensor layer installed
                                 # (managed by `ccp auto install/uninstall`)
  policies:
    default:
      # Loans, in order of preference. The primary is IMPLICIT (ccp resolve $PWD)
      # and is silently dropped if you list it here.
      fallback: [work-2, personal-deepseek]
      threshold: 90          # % of the usage window that triggers a proactive hop
      min_dwell: 20m         # min time before rotating (full only for the proactive sensor)
      max_hops: 6            # hard cap on LOANS per run (anti-loop backstop;
                             # coming home is free, see above)
      return_check: 10m      # how often to reconsider the primary while on loan
      return_idle: 90s       # …and how long the session must have been SILENT
                             # before that proactive trip home may kill the child
                             # (0s opts out: come home even mid-turn)
      cooldown:
        strategy: resets_at  # use the resets_at the API reports (subscriptions)
        fallback: 1h         # …or this fixed wait when there is no resets_at

    overnight:               # `ccp session -p --policy overnight`
      fallback: [personal-1, work-2]
      threshold: 85
      max_hops: 12

    work:
      fallback: []           # no loans at all: if the primary dies, the run stops

  allow_from:                # compliance gate (see below)
    work-1: [work-1]                                   # never rotates
    work-2: [work-2, personal-1]
    personal-1: [personal-1, work-2, personal-deepseek]
```

> **What the `return_check` timer actually does — it is not a passive clock.** It is armed only while the session is on **loan**: there has to be a live handoff marker whose origin is the primary (a *degraded* rotation — one that happened before any transcript existed and so never opened a marker — does not count), plus `--no-return` off and `return_check > 0`. While armed it re-decides once per period, and each decision is cheap: a few comparisons plus one `stat` of the transcript. What is *not* cheap is what happens when the decision comes out "go home" — its only action is to **`SIGTERM` the running `claude`** (10s of grace so it flushes its `.jsonl` and runs its `SessionEnd` hooks), close the loan, and **relaunch** `claude --resume` in the primary under the new uuid. That is a killed process and a fresh child, not a timestamp comparison. Hence the four conditions it demands before firing, all of them: a live loan · the primary's cooldown expired · `min_dwell` already served in the current profile · the session **idle** for `return_idle`. A limit event already queued from a sensor also beats it (rotating instead preserves the cooldown of the profile being left). If any condition is missing it simply waits and asks again next period — it never forces the move. Disable it with `return_check: 0s` (you then come home at the next limit event, as before) or `--no-return`.
>
> `--no-return` switches off the **mid-session** trip home — the timer and the pendulum rule in `Next()` — not the cleanup at the end of the run. When the child finally exits 0 with a loan still open, the loan is always closed and the conversation lands back in the primary under a new uuid, `--no-return` or not. That is deliberate: the alternative is a finished conversation stranded in a borrowed account behind a marker you have to remember to `ccp handoff end` tomorrow, while your repo keeps resolving to the profile that lent it. If you *want* it to stay there, end the run with Ctrl-C (exit 130 leaves the marker alive on purpose) or drop the marker later with `ccp handoff discard`.

**Why `allow_from` exists:** rotating on its own, at 3am, with nobody watching, means a client's conversation could end up in a personal account — or in a third-party provider's API. Path rules are *geographic* (which folder belongs to whom), not a statement of trust, so the gate is separate and explicit. The exact rule:

| `allow_from` | Effect |
|---|---|
| absent or empty | **no gate** — the whole `fallback` chain is allowed |
| declared, with an entry for the primary | only the profiles listed in that entry are allowed; the rest show up as *denied* |
| declared, **without** an entry for the primary | **total deny** — no loans at all |

That last row is the point: declaring the map is declaring the intent to govern loans, so a profile you forgot to add stays put instead of inheriting a free pass. `ccp session --dry-run` and `ccp auto status` both print what was denied and why.

> **The row you'll hit first.** `ccp auto init` seeds `allow_from` with one entry per *named* profile, and `default` is never one of them. So in a directory with no path rule the primary is `default`, there is no entry for it, and the whole chain shows up as denied — `ccp session` still runs, it just has nowhere to hop when the limit lands. Either set a rule (`ccp path set . <profile>`) or add a `default:` entry to `allow_from` yourself. `--dry-run` shows this immediately: `chain: (empty)` with everything under *denied*.

### Editing the chain — add, reorder or remove a loan

```bash
ccp auto chain                        # the EFFECTIVE chain for this directory
ccp auto chain add personal-deepseek  # append it, and authorise the loan
ccp auto chain add work-2 --at 1      # insert at a 1-based position
ccp auto chain mv work-2 2            # reorder — the order IS the preference
ccp auto chain rm personal-deepseek   # take it out, and withdraw the authorisation
ccp auto chain set work-1,work-2      # replace the whole chain
```

All of them take `--policy <name>` (default: `default`) and act on the primary of the **current directory**, which is the one whose `allow_from` entry they are allowed to touch.

**`add` writes two keys, and that is the whole point.** Whoever types "add it to the chain" wants the profile to *be used*, and a `fallback` entry without its `allow_from` is never used — silently. Leaving the two keys apart in the CLI would just reproduce the trap the YAML already sets. But `allow_from` is a **compliance gate** and may be there on purpose, so the widening has three hard limits:

1. It touches **only** the entry of the primary that the current directory resolves to. Never anyone else's — a blanket rewrite would widen permissions for repos you are not even standing in.
2. It prints **exactly what changed in each key, separately**. `+name` authorises, `-name` withdraws.
3. `--no-allow` turns it off.

`rm` (and the profiles that `set` drops) goes the other way and **withdraws** the authorisation, again only from that one entry. That is what makes `add` reversible: without it the gate could only ever grow, and undoing a mistaken `add` would mean hand-editing the YAML — exactly what these commands exist to avoid.

```console
$ ccp auto chain add personal-deepseek

[ok] fallback   work-1 → work-2 → personal-deepseek
[ok] allow_from personal-1: +personal-deepseek

effective chain from this repo:
  work-1 → work-2 → personal-deepseek
```

Every mutation ends by re-reading the effective chain **from disk**, so you see what the engine will see — including anything `allow_from` is still blocking.

| What you want | Command |
|---|---|
| Add a loan | `ccp auto chain add <profile>` (writes `fallback` **and** `allow_from[<primary>]`) |
| Change the preference order | `ccp auto chain mv <profile> <pos>` — the **order is the preference** |
| Remove a loan | `ccp auto chain rm <profile>` (withdraws the authorisation too; `--no-allow` keeps it) |
| Stop rotating in a directory | `ccp auto chain rm` every loan, or give that primary a policy with `fallback: []` |
| A different chain for a different job | add another policy under `policies:` and use `--policy <name>` |

Editing the file by hand is still perfectly fine, and is how you add a whole new policy. `ccp` rewrites `~/.config/ccp/ccp.yaml` atomically while **preserving your comments and any keys it doesn't know**, so an annotation about why a profile is in the list survives every later `ccp path set`, `profile add` or `auto install`.

```yaml
auto_handoff:
  policies:
    default:
      # Order IS preference: work-2 first, the provider only as a last resort.
      fallback: [work-2, personal-deepseek]
  allow_from:
    personal-1: [personal-1, work-2, personal-deepseek]   # ← the primary's entry
```

The rules the resolver applies to `fallback`, in one pass: the **primary is implicit** and is dropped silently if you list it; duplicates are removed (they don't buy an extra loan); `default` is a legitimate target (it's your normal `~/.claude` login); and the order you wrote is kept verbatim.

> **The one command that gets you out of a total deny.** In a directory whose primary has no `allow_from` entry, `ccp auto chain show` says `chain (none)` while every profile is already sitting in `fallback` (that is what `ccp auto init` seeds). `ccp auto chain add <profile>` handles that: a profile already in the chain but blocked by the gate is *not* treated as a duplicate — it creates the entry, tells you the chain itself did not change, and the repo starts rotating.

Then, the two verification steps — chain membership is **not** the same as detection:

```bash
ccp auto install personal-deepseek    # sensors for the newly added profile
ccp session --dry-run                 # what the chain resolves to, here
```

```
política: default
primario: work (cwd /home/me/repo)
cadena de préstamos: work-2 → deepseek
umbral 90% · min_dwell 20m0s · max_hops 6 · return_check 10m0s · return_idle 1m30s · cooldown resets_at (respaldo 1h0m0s)
```

If you only edited `fallback` and forgot `allow_from`, `--dry-run` says so instead of silently doing nothing — this is the single most common mistake:

```
cadena de préstamos: (vacía)
denegados por allow_from: work-2, deepseek
```

**A name that doesn't exist is a hard error, not a silent skip.** A typo — or a profile you removed — stops `ccp session` before it launches anything, with exit `1`:

```
[error] política "default": el perfil de fallback "app" no existe
```

> **`ccp profile rename` renames the profile inside `auto_handoff` too; `ccp profile rm` does not touch it.** Three places carry profile names — `policies.*.fallback`, `allow_from` (**both** the keys and the lists) and `hooks` — and a rename rewrites all three in the same write as your path rules, comments included. Deleting a profile leaves its name dangling in all three, and the next `ccp session` in that directory fails with the error above until you fix the YAML: grep for the name before you close the file. Those leftovers are also why a rename **refuses** a new name that `auto_handoff` already mentions — otherwise the renamed account would silently inherit chains and an `allow_from` gate that were never its own.

> **`ccp auto init --force` regenerates the block from scratch**, so it discards your hand edits *and* your comments. Use it to start over, not to refresh. Plain `ccp auto init` is idempotent: with a block already present it does nothing.

Note that `ccp auto init` deliberately leaves provider profiles (DeepSeek/Kimi/GLM) out of every *other* profile's seeded `allow_from`. Sending a conversation to a third-party API is exactly the decision the gate exists to make explicit, so adding one to a chain is always a manual act.

### The sensors — three at a time, four in total

Three sensors run at once in each mode, because none of them covers everything. Redundancy is deliberate: detecting the same limit twice is free (the supervisor de-duplicates by content), being blind is not.

| Sensor | What it reads | Proactive? | Interactive | Headless |
|---|---|---|---|---|
| **statusLine** (`ccp _statusline`) | `rate_limits.{five_hour,seven_day}.used_percentage` that Claude Code feeds the status bar; `<cc-home>/.claude.json` as a stale-sample fallback | ✅ fires at `threshold` % **before** a turn fails | ✅ | ❌ (no TTY ⇒ no status bar) |
| **stream-json** | the `api_retry` / `error: "rate_limit"` events of `claude -p --output-format stream-json` | ❌ reactive | ❌ (nothing to parse) | ✅ |
| **transcript tail** | the session's `.jsonl`, looking for `"error":"rate_limit"` / `apiErrorStatus: 429` | ❌ reactive | ✅ | ✅ |
| **StopFailure hook** (`ccp _limit-hook`) | the hook payload Claude Code sends when a turn dies; leaves a sentinel in `~/.config/ccp/state/auto/sentinels/` | ❌ reactive | ⚠️ unverified (see below) | ✅ |

So: **interactive** runs statusLine + transcript + sentinel; **headless** runs stream-json + transcript + sentinel. Only the statusLine sensor is proactive, and it is the one that does *not* work headless — which is why the headless path leans on the (very clean) `api_retry` event instead.

⚠️ **Not yet verified empirically:** whether the `StopFailure` hook fires at all in interactive with a *subscription* limit (Claude Code doesn't end the turn there — it offers `/rate-limit-options`). If it doesn't, interactive detection rests on the statusLine percentage plus the transcript tail. `ccp auto test` verifies the wiring, not Claude Code's behaviour.

### `ccp auto install` touches your profiles' `settings.json`

The two in-process sensors can only be turned on from `cc-home/settings.json`, and that file is **generated** (global ⊕ overlay). So `ccp auto install <profile>` adds the profile to `auto_handoff.hooks` and regenerates: the merge becomes global ⊕ overlay ⊕ **auto layer**, which adds

- `hooks.StopFailure` → `ccp _limit-hook`
- `statusLine` → `ccp _statusline -- <your original statusLine>` (yours is **wrapped**, not replaced — it still renders your status bar; ccp only samples the stdin it gets)

If you had **no** statusLine of your own, ccp paints a minimal one instead: the profile plus both usage windows, each with a gauge and a countdown to its reset — `work-1  5h ▏█░░░░░░░░░▏ 2% ·2h13m  7d ▏██████░░░░▏ 59% ·3d`. Both are shown because both are watched independently (whichever crosses `threshold` first triggers the hop), and a bare percentage wouldn't say whether you have hours or days left.

The gauge is tinted green below 70%, amber from 70 to 89, and red at 90 and above — the red starts exactly at `threshold`'s default, so the bar and the engine never tell different stories. `NO_COLOR` drops the tint and the gauge still reads.

Three things the bar deliberately won't say. A window **with no data** is omitted rather than drawn as `0%`. A `resets_at` that has already passed prints **no countdown**: stale data must not claim your quota is back. And days round **up** — `·3d` with 2d23h left, never `·2d`, because the one thing a countdown must never do is promise the quota returns sooner than it will.

The line sizes itself: ccp renders it at three levels of detail, measures each, and prints the widest one that fits. A long profile name costs gauge cells, not correctness — if nothing fits, you get the compact form uncut, because the first thing truncation would eat is the profile name.

Any `StopFailure` hook you already had is preserved alongside ours. It is fully reversible: `ccp auto uninstall <profile>` drops it from the list and regenerates back to global ⊕ overlay. Your overlay is never modified either way — the source of truth for "who has the sensors" is `auto_handoff.hooks` in `ccp.yaml`, not the generated file.

> The layer records the **absolute path** of the `ccp` binary. If you move it without running `ccp upgrade` (or `ccp auto install` / `ccp profile sync`), the sensors keep pointing at the old path.

### A session, and its trace

```
$ ccp session
# primary = personal-1 (path rule for ~/Documents/Personal)

▶ personal-1 · new session 3f9c1a2b                                     (stderr)
personal-1 (4h 03m) ──[uso 94% ≥ umbral 90% (ventana session) · statusline]──→ handoff a work-2 (préstamo 1/6)
▶ work-2 · reanudando 3f9c1a2b                                          (stderr)
work-2 (1h 12m) ──[You've hit your session limit · transcript]──→ volviendo a personal-1 (vuelta a casa, no gasta préstamo: siguen 1/6)
sesión devuelta a personal-1 como 7d0e44f1
▶ personal-1 · reanudando 7d0e44f1                                      (stderr)
personal-1 ──[termina]──→ ✅ exit 0
```

**Reading the counter.** Every movement ends in the same budget, worded one way only: an outbound move is `préstamo N/6` (this is loan N of your `max_hops`), and a trip home is `vuelta a casa, no gasta préstamo: siguen N/6`. The number does *not* rise on the way home — coming back is the closing of a loan, not a new one (see `max_hops` above) — so the line says so out loud rather than leaving you to wonder why two consecutive moves show `1/6`. The trip home is never called a *préstamo*, whichever of the two things caused it (a limit in the loan profile, or the `return_check` timer): it's the same event, so it gets the same words.

The trace (hops, the trip home, the cooldown table) goes to **stdout** because it *is* the command's output — it's what ends up in the cron log. The operational chatter (`▶ launching…`, "waiting for min_dwell") goes to **stderr**, so that in headless mode stdout stays parseable: there it carries the child's `stream-json` verbatim.

Exit codes: `0` ok · `1` usage/config error · `2` handoff I/O failure · `75` every profile exhausted (retry later) · anything else is claude's own exit code, so `ccp session -p …` drops into a script where `claude -p …` used to be.

**Some sharp edges worth knowing.** `--yolo` (`--dangerously-skip-permissions`) is effectively required for unattended runs, and it is never persisted — you ask for it every time. A hop is a `SIGTERM` at a turn boundary, so an interrupted tool call gets re-run by `--resume` and may not be idempotent. `Ctrl-C` (exit 130) never rotates: the loan is left open and told to you. And `min_dwell` applies **in full only to the proactive sensor** — the one that warns before anything has failed. A reactive event means the turn already failed, so sitting out twenty minutes inside an account that is answering 429 would be pure harm: those rotate after a short 30s floor, which still keeps three profiles from burning in one minute. The trip home is the exception and honours the full value: nobody is in a hurry there.

---

## Claude Desktop — one window per profile

`ccp desktop` opens an isolated **Claude Desktop** instance per profile: account, MCP servers, Cowork
environment… and the Code tab. You can keep work and personal open side by side.

```bash
ccp desktop open work-1            # launch the 'work-1' profile's instance
ccp desktop open                   # no profile: whichever the current directory resolves to
ccp desktop list                   # instances on disk and their size
ccp desktop open work-1 --dry-run  # print the plan without launching or touching anything
ccp desktop doctor                 # audit launchers, instances and their identity
```

If a window ever shows up empty, or opening Claude from the Dock brings up the wrong one, run
`ccp desktop doctor` before assuming anything was lost. It reports which windows are alive, whether
one is running under the main Claude's identity, whether any of them is writing its Code history into
the global `~/.claude`, and — the one that matters most — whether a data dir holds sessions from more
than one account. Sessions are indexed per account: the ones you cannot see have **not** been deleted,
they come back when you sign in with that account again. It diagnoses; it never repairs.

It works **without reinstalling the rc** (`ccp install` is only needed for completion).

### The isolation is two things, and the second one is the invisible one

`--user-data-dir` moves the app's identity: session, tokens, `claude_desktop_config.json` (MCP) and
Cowork state. That is half of it.

The other half is the **Code tab**, which does *not* read that directory: it reads `CLAUDE_CONFIG_DIR`
from the Desktop process's environment, falling back to `~/.claude` when empty. Without injecting it,
two windows on different accounts would share CLI credentials, `projects/`, history and agents.
`ccp desktop` emits both, so the whole window — chat and Code tab — sits on the same profile as your
terminal.

### What it changes in the profile's `cc-home`

Desktop **refuses symlinked directories** under its config root, and `ccp profile add` seeds exactly
that (`cc-home/commands -> ~/.claude/commands`). The first time you open an instance, `ccp` converts
those entries into **real** directories whose files are symlinks to the global one:

- commands and plugins stay shared live with `~/.claude`;
- `agents/` and `skills/` become the profile's own, so **each profile can have its own agents**;
- a file of yours inside the profile wins over the global one and is never overwritten on re-mirror.

When you add new files to the global config, re-mirror with `ccp desktop prepare <profile>`.

### A launcher per profile: its own name and icon colour in the Dock

`ccp desktop open` isolates the *account*, but every window is still the same `Claude.app`: the Dock,
Cmd-Tab and Spotlight show N identical "Claude" icons. `ccp desktop app` fixes that with a **launcher**
per profile in `~/Applications`:

```bash
ccp desktop app                              # one launcher per official profile, colours assigned automatically
ccp desktop app work-1 --color purple        # or pick: blue green purple pink teal yellow red gray orange #rrggbb
ccp desktop app work-1 --label "Claude Work" # the name shown (it is also the .app's file name)
ccp desktop open work-1                      # from now on it launches through the launcher (--plain skips it)
ccp desktop app rm work-1                    # remove the launcher; the instance and its session stay
```

`Claude (work-1).app` shows up in Spotlight and Launchpad, can be kept in the Dock, and opens the profile's
instance with the Claude icon tinted in that colour and its own name in the Dock, Cmd-Tab and the menu bar.
A click on it does exactly what `ccp desktop open work-1` does: the launcher *is* `ccp`, which sets
`--user-data-dir` and `CLAUDE_CONFIG_DIR` and then hands the process over to Claude.

You rarely need to run `ccp desktop app` by hand: if a profile has no launcher, `ccp desktop open` builds one
before launching. That is not cosmetic. Without a launcher the window runs from `/Applications/Claude.app`
itself, so macOS cannot tell it apart from your main Claude: the Dock, Cmd-Tab, `open -a` and `claude://`
links treat both as the same app, and opening Claude brings up whichever window it feels like.

**Your session is kept.** The launcher is not a modified copy of the app — that loses the account on the
first launch, because the Keychain (where Claude keeps the key to its session) stops trusting the process.
Inside the launcher there is a mirror of the real `Claude.app` made of hard links: byte-for-byte the original,
no extra disk (`du` will still count it), and the Keychain keeps trusting it.

**Updates keep flowing.** The mirror pins the version it was built from, so a `Claude.app` update leaves a
launcher running the previous version until its next launch, when `ccp` notices — by version *and* by inode,
because the updater replaces the bundle wholesale instead of patching it — and rebuilds the mirror (about a
second). Same after `ccp upgrade`: the launchers embed the binary and re-link the new one on their next
launch.

**A profile instance can never update your Claude.** It could, and once did: an instance updated a user's
`/Applications/Claude.app` from a window opened for a different account. Every instance other than `default`
now starts with its updater switched off, and `ccp desktop doctor` checks the guard is actually there rather
than assuming it. `default` is exempt on purpose: that instance **is** your Claude, and muting its updates
would be hijacking it — updates reach you through it, as always.

### Moving a conversation to another profile's window

Each window keeps its own Code-tab sessions, so a conversation started under one account does not show up
in another profile's window. `ccp desktop copy` takes it there: it copies the conversation into the
destination profile and asks that window to import it, so it appears in its sidebar with the same title and
you carry on where you left off.

```bash
ccp desktop sessions                               # every window's sessions: uuid, title, folder
ccp desktop sessions work-1                        # only that profile's (all of them)
ccp desktop copy "loan type validator" default     # by title, or a piece of it…
ccp desktop copy 37b541db work-1                   # …or by uuid, or its first characters
ccp desktop copy 37b541db work-1 --from personal-1 # when the same session lives in two windows
ccp desktop copy 37b541db work-1 --dry-run         # say what it would do, touch nothing
ccp desktop copy 37b541db work-1 --no-open         # copy only (then continue it in the terminal)
```

What it does:

1. **Finds the session.** Titles come from each window's own index — the ones you see in its sidebar.
   Without `--from` it searches every window except the destination; if the name matches more than one
   session it lists them (with their full uuid) and stops.
2. **Copies the conversation**: the Claude Code transcript (`<cc-home>/projects/<folder>/<uuid>.jsonl`) and
   the folder next to it with subagent and workflow files, into the destination profile, under the same
   folder and the same uuid. The copy is a file of its own and private (`0600`). The original is never
   touched.
3. **Asks the destination window to import it**, with Claude Desktop's own link,
   `claude://resume?session=<uuid>`, sent to *that* window (to its launcher; to your Claude for `default`).
   `ccp` never writes Desktop's index by hand: it waits until Desktop adds the session to it, and only then
   says it's done. If the window was closed, the link opens it.

The rules that keep it safe:

- **It never overwrites a conversation.** If the destination already has the session and you kept going
  there, it is left alone. If the destination has an older copy nobody continued — the return trip of a
  loan — it is brought up to date, but not while that window is open with the session in its sidebar: it
  could have the old version loaded and would fork the conversation, so `ccp` asks you to close it first.
  If both sides kept going separately, nothing is written.
- **The link only goes where it should.** Before sending it, `ccp` checks with `ps` and `lsappinfo` that the
  destination window holds its own identity. A profile without a launcher (its window runs as your main
  Claude) or a window whose identity collapsed (see *Limits*) gets the copy but not the link, and you are
  told why and what to do. If it cannot check, it does not send.
- **The title travels with it.** Desktop only reads a session's title from the last 256 KB of its
  conversation; for a long one titled at the start, `ccp` adds the title at the end of the copy.
- **It is idempotent.** Running it again duplicates nothing: a window that already lists the session is not
  asked to import it again.
- Only sessions with a local transcript can be copied (remote ones cannot), and only into `official` or
  `default` windows. A CLI session works too: pass its full uuid. The import step is macOS only; elsewhere
  you get the copy and the command to continue it in the terminal.

The original stays in the source window. Continue in one place only: if you write in both, they drift
apart — and from then on no copy can keep both. Exit code: `0` when the session is in the destination's
sidebar (or, with `--no-open`, copied); `1` otherwise, including "copied but not imported", which always
comes with what to do next.

### Limits

- **`official`** and **`default`** profiles only. Claude Desktop does not read `ANTHROPIC_BASE_URL`, so
  a DeepSeek/Kimi/GLM profile is refused rather than leaving you with a half-configured window.
- **`default` is never relocated**: its instance is the usual one, in the usual place.
- Each instance downloads its own copy of the embedded Claude Code (~190 MB), plus the Cowork
  environment if you use it. `ccp desktop list` tells you how much each one takes.
- **Sign in one at a time**: `claude://` links go to whichever instance registered the scheme last, so
  with several windows open a login can land on the wrong one. `ccp` reminds you on each instance's
  first launch.
- `ccp desktop rm <profile> --yes` deletes the instance: it is a logout that takes session, tokens and
  MCP config with it.
- **Launchers are macOS only**: they are app bundles in `~/Applications` (`CCP_DESKTOP_APPS_DIR` moves
  them). A launcher never claims `claude://`, so the login callback always reaches the normal app: sign in
  to a **new** profile with `ccp desktop open <profile> --plain` and the other windows closed, then use the
  launcher. That is the one flow where `--plain` is the right answer, and `ccp` will still warn you that the
  window is indistinguishable from your main Claude — which is true, and is why you close the others.
  macOS privacy permissions (files, screen, microphone…) are granted per app, so a launcher may ask once more.
- **A window's identity can collapse.** macOS identifies a running process by the path it was executed
  through, and Claude re-executes itself by its *resolved* path when it restarts — so after a relaunch the
  window may come back wearing your main Claude's identity. While that lasts, opening Claude from the Dock
  activates *that* window instead of yours. `ccp desktop doctor` tells you when it has happened;
  `open -n -a /Applications/Claude.app` gets you back to yours right away.

## Backup and restore

Take everything to another machine, or back it up before a big change:

```bash
ccp backup export ~/ccp-backup.tar.gz                 # ccp.yaml + overlays
ccp backup export ~/ccp-backup.tar.gz --with-secrets  # + api_key + logins (chmod 600)
ccp backup restore ~/ccp-backup.tar.gz                # no pisa; fusiona reglas
ccp backup restore ~/ccp-backup.tar.gz --overwrite    # reemplaza perfiles del backup
ccp backup restore ~/ccp-backup.tar.gz --force        # borra todo y restaura limpio
```

Before restoring, `ccp` saves an automatic snapshot in `~/.config/ccp/.backup-pre-restore-<fecha>`, and also a
[safety snapshot](#snapshots--the-history-of-your-whole-configuration) of everything else.

## Snapshots — the history of your whole configuration

`ccp backup` is one file you have to remember to make. `ccp snapshot` is a **history**: every snapshot records
the whole configuration, not just ccp's, and only what changed takes up space. They live in
`~/.config/ccp/snapshots`.

```bash
ccp snapshot create -m "before trying the new MCP"   # take one now (a label keeps it from pruning)
ccp snapshot list                                  # history, newest first
ccp snapshot diff latest                           # what changed since then
ccp snapshot restore 3f2a9c                        # only SHOWS the plan (exit 1): nothing is written
ccp snapshot restore 3f2a9c --yes                  # applies it
ccp snapshot restore 3f2a9c --only claude/agents --yes   # just one part
ccp snapshot export latest ~/config.ccpsnap --with-secrets   # one file for another machine
ccp snapshot import ~/config.ccpsnap
```

What a snapshot captures:

| What | Class | Captured |
|---|---|---|
| `ccp.yaml`, each profile's overlay (`CLAUDE.md`, `settings.overlay.json`) | config | always |
| `~/.claude`: `settings.json`, `CLAUDE.md`, `keybindings.json`, `agents/`, `commands/`, `skills/`, `output-styles/`, `hooks/`, the plugin lists | config | always |
| `.claude/settings.local.json` and `CLAUDE.local.md` of every folder with a rule | config | always |
| A provider's `api_key`, the MCP part of each `.claude.json`, each Desktop window's `claude_desktop_config.json` | secret | always, sealed with the store's key |
| Conversations (`cc-home/projects`) and `handoffs.yaml` | state | only with `--with-state` |
| Generated files, caches, logins, tokens, `machineID` | — | never: they are regenerated or recreated |

- **Restoring never starts blind.** Without `--yes` it prints the plan and changes nothing. With `--yes`, it
  first takes a snapshot of the current state; if that fails, nothing is written. It only writes what the
  snapshot holds, never deletes what exists only on disk, and then regenerates the affected profiles. From a
  `.claude.json` only the configuration travels: restoring merges it into the live file, leaving your session
  alone.
- **Automatic snapshots.** A safety one before `ccp profile rm` and `ccp backup restore`: if it can't be saved,
  the command doesn't run. And a daily one, taken by the first management command after 20 hours (never by
  `ccp status`, `resolve` or the prompt hook). `CCP_NO_AUTO_SNAPSHOT=1` turns both off.
- **Pruning.** `ccp snapshot prune` keeps 7 days, 4 weeks and 6 months, plus the latest and everything pinned
  (`ccp snapshot pin <id>`) or labelled. `--dry-run` shows what it would delete.
- **Secrets in an exported file** only travel with `--with-secrets`, sealed with a passphrase of at least 12
  characters that `ccp` asks for without echo (`CCP_SNAPSHOT_PASSPHRASE` gives it in scripts). Without it,
  the file carries no secrets, and importing it says which items came without data.

#### The **Snapshots** screen — the history, in the app

The desktop app shows that same history as a timeline: label, what triggered it, when, how much it captured and
whether it is pinned. Picking one shows **what it captures**, grouped by part (ccp, `~/.claude`, each Desktop
window, each project), and from there you pin it, label it, export it or compare it.

- **Restoring takes two steps and the first one writes nothing.** The app asks for the plan, shows step by step
  what it would write, what it would merge, what already matches and what it skips — and why — and applies only
  once you tick the parts you want and type the confirmation word. It is the same as the terminal, where without
  `--yes` a restore is just a plan. When it finishes it names the prior snapshot, which is the way back.
- **The checkboxes are the `--only` parts**, so the CLI line it offers to copy does exactly what the button does.
- **Comparing against "what is there right now"** answers the real question — what changed since then — without
  restoring anything.
- **Pruning shows first what it would take with it**: how many remain, how many blobs are freed and the ids that
  are going.
- A snapshot's size is **what it captures**, not what it occupies: two similar snapshots share the same blobs and
  only what is new is stored.
- The `.tar.gz` copies of `ccp backup` are still in **Settings**: they are the old format, good for moving a
  configuration to another machine by hand.

## Cloud — the same history, on your own server

`ccp snapshot` is the history on this machine. `ccp cloud` is that same history on a server of yours, so a
second Mac can pick it up. **Everything is encrypted here before it leaves**: the server stores sealed text,
opaque ids and signatures it cannot make, and it never sees the key.

```bash
# first machine
ccp cloud login https://ccp.example.com   # device code: approve it in the browser
ccp cloud init                            # create the vault; WRITE DOWN the recovery code
ccp snapshot create -m "first upload"
ccp cloud push                            # uploads what the cloud does not have

# the other machine
ccp cloud login https://ccp.example.com
ccp cloud unlock                          # the vault passphrase (not your account password)
ccp cloud restore latest                  # downloads the newest one IN THE CLOUD and shows the plan:
                                          # what it would write, where each project lands here and
                                          # what is left to do by hand. Add --yes to apply it
ccp cloud restore <id> --only claude/settings.json --yes   # …or just those paths

ccp cloud status      # server, account, machine, vault, how many are pending
ccp cloud list        # snapshots in the cloud, from every machine
ccp cloud verify      # the whole signed history: nobody removed, reordered or rewrote a link
ccp cloud devices     # your machines; `ccp cloud revoke <id>` throws one out
ccp cloud groups      # device groups; `groups add "all my Macs" mac-a mac-b`, `set`, `rm --yes`
ccp cloud groups status "all my Macs"   # how the last order went on each machine of the group
ccp cloud audit       # who did what and when; --device, --action, --since, --json
ccp cloud rotate      # new vault passphrase and recovery code (the account key does not change)
ccp cloud logout      # revoke this machine and delete its token and local vault
```

**Two secrets, and they are not the same one.** Your account password lives in the login page and answers
*who are you*. The **vault passphrase** never reaches the server and answers *can you read this*. The
**recovery code** is shown once, when you create the vault: keep it off this machine (a password manager,
paper). Lose the passphrase *and* the code and the cloud copy cannot be recovered — your local snapshots are
still the primary source.

**What the server sees, and what it does not:**

| It sees | It does not see |
|---|---|
| When each snapshot was made, how big it is, which machine sent it and which one it follows | What is inside any of them: blobs and manifests arrive sealed |
| Opaque ids (an HMAC of the local hash), enough to store each thing once | Whether you have a particular file: it cannot check an id it did not receive |
| Your machines: name, platform, last contact | Your API keys, MCP tokens, hooks or instructions |

- **Signatures are checked against your own key.** Every snapshot is signed with a key derived from the vault
  key, and `pull` verifies it with the public half derived here — never with one the server hands over. A
  compromised server can refuse to serve you; it cannot slip in, alter or reorder a snapshot.
- **The history is a chain, and `ccp cloud verify` walks all of it.** Each snapshot's signature covers the id
  of its parent, so `verify` catches a link that was rewritten (`bad_signature`), one taken out of the middle
  (`broken_link`), one taken off the head (`dropped` — it knows what this machine uploaded), a date that
  contradicts the parents (`out_of_order`) and ids that repeat or loop. It exits 1 if anything does not add
  up, which is what a cron wants.
- **Retention never breaks the chain.** A server may be configured to prune old snapshots
  (`CCP_CLOUD_RETENTION_DAILY` / `_WEEKLY` / `_MONTHLY`; with none of them it keeps everything, the default).
  Pruning takes the manifest and the blobs — all it occupies — and leaves the link: id, parent, date, digest
  and signature. That is deliberate: a hole left by retention would look exactly like a hole left by a
  compromised server, and then `verify` would be a warning you learn to ignore. A pruned snapshot cannot be
  downloaded (the CLI says so); yours is still on the machine that made it. **Every device always keeps its
  most recent snapshot with content**, because the sweep covers the whole account and an idle machine would
  otherwise lose its only cloud copy right when it is needed. **Pinned snapshots are never
  pruned**: `ccp snapshot pin <id>` (or giving one a label) travels up on the next `push`.
- **Restoring reaches this machine by three roads, and all three end in the same engine** (the `snapshot
  restore` of §8.3: it plans, takes a safety snapshot first, applies selectively and regenerates the
  projection). From the app (Cloud → History: every machine's snapshots, the diff against what is live here,
  all of it or single items); from the portal («Restore on <machine>», which publishes a signed revision that
  this machine applies when its agent next checks in, confirming anything executable locally); and on a brand
  new machine, `ccp cloud restore`. The portal never restores by itself: there is no inbound connection to
  your machines.
- **A group is a name and some machines, and it does not command.** «All my Macs» saves you ticking the same
  boxes in the portal's «Apply to…»; what goes out is still one **signed revision per machine**, and the group
  tag is deliberately **outside** the signature. What the signature binds is the destination device — that is
  what stops an order being redirected — so editing a group later cannot change who obeys an order that was
  already signed. `ccp cloud groups status <group>` says how the last order of that group went on each
  machine, and it says two things carefully: a member with no order of that group reads *no orders*, not
  *pending* (there is no order of its own pending anything), and a machine you removed from the group keeps
  showing while it still has a live order — taking it out of the group does not withdraw what was published
  to it.
- **The audit says who did what and when, and nothing about what.** `ccp cloud audit` and the portal's
  Audit screen read the server's insert-only log: an entry is an action, a machine, a date and a detail made
  of ids and counters. It cannot say more, because the server cannot read your configuration — and the store
  prunes the detail as it is written (a nested object or a long string is dropped) so that one careless
  caller cannot turn the log into the leak the encryption exists to prevent.
- **Revoking a machine also closes the order it had pending.** A revoked machine never asks again, so an
  order left open would read *pending* for ever; it is closed as `device revoked`, which is neither *failed*
  (it did nothing wrong) nor *superseded* (no other order replaced it).
- **Rotating the access keys is not rotating the account key.** `ccp cloud rotate` gives you a new vault
  passphrase and a new recovery code over the **same** account key: nothing has to be re-encrypted and
  everything you already pushed still opens. It does not ask for the old passphrase — that is the one that
  may have been lost — and it says the part that matters: a machine that was already unlocked stays
  unlocked, including one you revoked that kept its copy. Taking that copy back is AK rotation, which
  re-encrypts everything and is not implemented yet.
- **A new machine maps its own paths.** A snapshot from another machine names projects by their normalised
  git remote, so `ccp cloud restore` looks for each repo here: the path the snapshot carried (translated to
  this HOME), then any folder your rules, your `.claude.json` projects or the usual roots (`~/code`, `~/src`,
  `~/Documents/GitHub`…) hold with that same remote. What it cannot find is **skipped, never guessed**, and
  `--map <key>=<absolute dir>` says where it lives (the app has a folder picker). It also prints what is left
  to do by hand — OAuth tokens never travel, so each official profile needs `ccp profile login`, and an MCP or
  hook whose command is not on this machine is named before anything is written.
- **A snapshot can also come down as a file**, without touching this machine's store — which may not even
  have one: `ccp cloud pull <id> -o copia.ccpsnap` writes the portable archive (`ccp snapshot import` and its
  passphrase open it on the other side), and `-o copia.tar.gz --decrypted` writes the files themselves, in
  the clear, readable with any `tar`. If the snapshot holds keys, the plain one refuses to be written until
  you repeat it with `--yes`: the warning comes *before* the file exists, because a file with your keys
  inside is not un-written by a message.
- **Blobs never pass through the API.** They go straight between this machine and the storage, with
  pre-signed URLs. Anything over 64 MiB is left behind and `push` says which.
- **Paths are translated between machines.** A snapshot taken under `/Users/ana` and restored where HOME is
  `/Users/jose` rewrites the HOME inside folder rules, hook and MCP commands and each project's path — the
  restore announces it («Paths from … rewritten to …»). Conversations are history: their paths describe where
  something happened, so they are left alone.
- **Revoking a machine** throws it out of the API on its very next request. It does not erase the key that
  machine already has: if you fear a leak, the answer is rotating the account key, which is not here yet.
- **This machine's cloud files** live in `~/.config/ccp/cloud` (0700, every file 0600): the session, the
  device token, the unlocked key and which snapshots are already uploaded.

### The portal proposes, this machine applies

Nothing ever connects *into* your Mac. The portal publishes a **desired revision** — «reach this snapshot» —
signed with the account key, which the server does not have; this machine pulls it, checks that signature
against its own key, and decides what to write.

```bash
ccp cloud agent --once                # one pass: fetch, reconcile, apply
ccp cloud agent                       # stay watching (every 5 min; --interval 30s)
ccp cloud review                      # confirm what runs code here (--yes / --reject / --json)
ccp cloud policy manual               # this machine: nothing is applied without confirming it
```

- **Three-way merge, per logical path**: base (the revision's own `base`, the last applied snapshot), what is
  live here, and what the revision wants. What only changed up there is applied; what changed on both sides is
  a **conflict** and is left exactly as it is; what only changed here is kept. A revision with no base is an
  absolute order («reach this snapshot») and is applied as a restore.
- **A snapshot before writing**, always — the same engine as `ccp snapshot restore`, so the previous state has
  an id you can go back to. The agent prints it.
- **What runs code is never applied on its own** (`auto` is the default policy): hooks, an MCP server's
  `command`/`args`, `statusLine`, plugins, a skill with a script, and permissions that **widen**
  (`permissions.allow`, `defaultMode`) wait for `ccp cloud review` on this machine. A stolen account is not
  enough to run code on your Macs. What only *describes* configuration — instructions, rules, env, permissions
  that restrict — applies on its own; a `settings.json` that only changes `model` does not ask, because asking
  for everything teaches you to say yes without reading.
- **The revision stays open while it waits for you.** A result can be reported only once, so the agent does not
  close it with «partial, waiting»; `ccp cloud review` is what reports the final state (`applied`, `partial`,
  `conflict`, `failed`) once you have answered. Until then the portal shows it as pending, which is what it is.
- **The policy lives here, not in the cloud** (`~/.config/ccp/cloud/config.json`): it is this machine's defence
  against its own account, and a defence you could flip from where an attacker would be is no defence.

**Running it in the background is optional and you install it yourself.** ccp never writes to your
`LaunchAgents`: something that wakes up and rewrites your configuration is your decision, not an installer's.
The plist is in [`docs/launchagent-cloud-agent.md`](docs/launchagent-cloud-agent.md).

Setting up the server (Postgres + Keycloak + S3 storage + the `ccp-cloud` API) is a separate job; the pieces
live in `deploy/ccp-cloud/`. **The public deployment is pending the owner's authorization**, so until then
`ccp cloud` points at whatever server you run yourself.

### The web portal

`ccp-cloud` serves the portal itself, at the root of the same host as the API. You log in with Keycloak and it
asks for the **vault passphrase**: the account key is derived from it **in the tab**, with Argon2id, and never
leaves the browser — the server keeps holding things it cannot open. It forgets the key when you close the tab
or after 15 minutes without touching anything.

- **Devices**: last contact, ccp version, the profiles each machine has and its state against the revision
  published for it, with how many paths differ.
- **Timeline** of each machine, with the signature of every snapshot checked against the key derived here, and
  a **diff between any two of them**, grouped by area and filterable by path.
- **Editor** of a snapshot's configuration, in the same model as the app's Configuration screen — layer, type,
  where each item applies and, when it cannot be edited there, why — and **«Apply to…»**, which publishes the
  edit as a signed desired revision to the machines you pick. It does not restore and it does not create or
  delete items: the portal proposes, and inventing a logical path from a browser is how you get a file nobody
  can place.
- **Download** of any snapshot, assembled in the tab: a `.ccpsnap` sealed with a passphrase you type there
  (the same file `ccp snapshot import` opens) or a readable `.tar.gz`. The plain one is only offered once you
  tick a box that says, in those words, that your keys go in the clear — and without a passphrase the
  `.ccpsnap` leaves the secrets out rather than carry them in the clear inside something called «encrypted».
- **«Restore on <machine>»**, from the timeline: it publishes a signed revision **with no base**, which is
  what makes it a restore instead of a merge. It uploads nothing, because the snapshot is already up there,
  and the machine applies it as soon as its agent checks in, confirming anything executable right there.
- Signatures have three answers, not two: valid, altered, and *this browser cannot verify Ed25519* — which is
  not the same as valid.

There is no build step: plain ES modules embedded in the binary, a strict CSP and no third-party script, so
deploying the API deploys the portal. How it is built, what Keycloak needs and how to look at it without
deploying anything are in [`docs/portal.md`](docs/portal.md).

The other end of that is the app's **Cloud** screen: the account, the vault, your devices and — the point —
what the agent left waiting here because it runs code. Nothing comes pre-checked: you approve path by path,
and whatever you leave unchecked is rejected and reported back. Signing in and opening the vault are not done
from the app: it opens a Terminal, because the vault passphrase unwraps the account key and end-to-end
encryption is worth exactly as much as the place that passphrase travels through.

## Detect the machine — `ccp scan` and `ccp adopt`

`ccp scan` lists everything Claude-related on this machine: your global `~/.claude`, each profile, the MCP
servers of every Desktop window, project files, and `CLAUDE_CONFIG_DIR`s that ccp doesn't manage yet. For each
item it says **where it applies**: CLI, the Code tab, or the Desktop chat. Secret values never appear, only the
fact that they exist.

`ccp adopt` turns that into a plan and applies it only with `--yes`, after a safety snapshot:

```bash
ccp adopt              # shows the plan (exit 1 if there is something to apply)
ccp adopt --yes        # applies the checked steps
ccp adopt --only <id> --yes
```

- An MCP that only lives in your main Desktop window is offered as **global** (`~/.claude.json`), so the CLI
  sees it too. The same MCP with **different credentials** in two windows is never lifted: it would give one
  account's token to every profile.
- A `~/.claude-xyz` you used by hand is adopted as a new profile by **copying** its configuration (never its
  tokens, never `env`); the original is left alone and you run `/login` once.
- Logins, API keys, missing MCP commands and Desktop launchers are listed as things to do by hand.

The desktop app shows the same in the **Detect this machine** screen.

## Per-profile config

Each profile has its own Claude config, applied as a **baseline layer** when it's active:

```bash
ccp profile config <perfil>                 # menú: instrucciones / settings / ambos
ccp profile config <perfil> instructions    # abre overlay/CLAUDE.md
ccp profile config <perfil> settings         # abre overlay/settings.overlay.json
ccp profile sync [<perfil>]                  # re-mergea cambios del global ~/.claude
ccp profile rename <old> <new>               # rename: rules, chains, markers and key included (official: log in again)
ccp config editor "code -w"                  # editor a usar (fallback: $EDITOR)
```

- **Instructions**: `cc-home/CLAUDE.md` does an `@import` of the global `~/.claude/CLAUDE.md` and then of your overlay.
- **Settings**: `cc-home/settings.json` = global ⊕ overlay (pure-Go deep-merge).
- **`/config` inside a profile is kept**: Claude Code writes it to `cc-home/settings.json`, and before regenerating it `ccp` moves what you added or changed into the profile's overlay (`ccp profile sync` says what). What you removed is only warned about, the overlay wins if it changed that key too, and an invalid file is copied to `profiles/<name>/state/` instead of adopted.
- **Real precedence**: it's a baseline — the repo's config (`.claude/settings.json`) wins on conflict.
- `default` has no overlay: `ccp profile config default` opens your global `~/.claude` directly.

### MCP servers, agents and skills per profile

A profile no longer has to borrow everything from the global `~/.claude`. What it declares for itself lives in its overlay, and every regeneration **projects** it into the files the apps actually read ([ADR 0011](docs/adr/0011-una-fuente-declarada-varias-proyecciones.md)):

```bash
ccp instruct add profile mcp 'obsidian-vault={"command":"npx","args":["-y","obsidian-mcp"]}'
ccp instruct add profile rule "..."       # plain text; a hook goes as 'id={json}'
ccp instruct dest profile skill           # the directory to write an agent/command/skill into
ccp profile sync <perfil>                 # re-project everything by hand
ccp profile sync --check [<perfil>]       # what it WOULD change; exits 1 if anything is stale
```

| Layer | File | Reaches |
|---|---|---|
| Global | `~/.claude.json` (`mcpServers`) | every profile, through its projection |
| Profile | `profiles/<n>/overlay/mcp.json` (the shape of `.mcp.json`) | that profile only |
| Project | `<repo>/.mcp.json` | any profile, inside that repo (Claude Code reads it; `ccp` does not project it) |

**Effective = global ⊕ profile − disabled**, and the profile wins a name clash. Where each server goes is declared in `ccp.yaml`:

```yaml
mcp:
  targets:                     # per server; the default is both
    obsidian-vault: [cli, desktop]
    jira: [cli]
  disabled:                    # this profile turns an inherited one off without deleting it
    work: [finance-os]
```

- **`cli`** writes into the profile's `cc-home/.claude.json`, which is what your terminal `claude` **and** the Code tab of that profile's window read.
- **`desktop`** writes into that window's `claude_desktop_config.json`, which is the **chat** half. Two limits are the app's, not ours: only `stdio` entries (an `http`/`sse` one is discarded on start-up, so it is reported instead of written), and the file is not re-read while the window runs — so with the window open the change is left **pending** and applied on the next launch. `ccp` never closes a window on you.
- **`ccp` only touches the names it registered as its own.** Anything you added by hand to those files stays; if a name you declared already existed there by hand, it is reported as a conflict rather than overwritten.
- Your **main Desktop window (`default`) is never written to** — it is your own Claude, same reason its updates are left alone.
- `overlay/{agents,commands,skills,output-styles}/` work the same way: as soon as the profile has something of its own, its `cc-home` directory becomes global ∪ profile (real directories, symlinks only at the file leaves — the shape Desktop requires), and on a clash the profile wins.
- **Permissions merge by replacing arrays**, as they always have. To add instead of replace, say so in the overlay: `"permissions": {"$merge": "union", "allow": ["Bash(make:*)"]}`. The mark never reaches the generated `settings.json`.
- `ccp doctor` reports what is out of step: `projection_stale`, `desktop_restart_pending`, `mcp_command_missing`, `mcp_unmanaged_only_desktop` and `cc_home_symlink_nonleaf`.

#### `ccp mcp` — the same thing without editing files

```bash
ccp mcp list [--scope <layer>] [--json]              # what the layer declares, and where it goes
ccp mcp add fs -- npx -y @modelcontextprotocol/server-filesystem ~/code
ccp mcp add linear --url https://mcp.linear.app/sse --transport sse --header 'Authorization=Bearer ${LINEAR}'
ccp mcp add jira '{"command":"npx","args":["-y","jira-mcp"]}'   # the raw JSON, if you prefer
ccp mcp rm <name> [--scope <layer>]
ccp mcp enable|disable <name> [--profile <n>]        # an inherited server, in one profile
ccp mcp targets [<name> [cli|desktop|cli,desktop|none]]
```

The layer is `global`, `profile[:<name>]`, `project[:<path>]` or `desktop[:<name>]` (read-only), and **without `--scope` it is the terminal's active profile**. `--env KEY=value` goes with the stdio form, `--header KEY=value` with the remote one, and the three forms (the command after `--`, `--url`, the JSON) never mix. A secret goes in as the **literal** `${VARIABLE}` — hence the single quotes: with double ones your shell expands it first and the token is written in the clear (and the `project` layer refuses it).

Two refusals are the point of the command: the **Desktop window** receives a profile's MCPs but does not declare them — writing there would be undone by the next sync, so it tells you to declare it in the profile instead; and a **secret in the clear in a project's `.mcp.json`** — a file that travels in the repo — is refused, pointing at `${VARIABLE}`. After each write it says who it regenerated and which window keeps the previous MCPs until you restart it.

#### The **Configuration** screen — the same layers, in the app

The desktop app edits all of this from one screen: the layer on top (global · profile · project · window), the types on the left (instructions, MCP, skills, agents, commands, hooks, permissions, env, plugins, styles, status line and the rest of `settings.json`), and in the middle every item with **where it came from** and **where it applies** (CLI · Code · Chat). An **Effective** toggle shows one account's merged result, with whatever is shadowed marked as such.

- What you see but cannot edit says why, in the same words as the terminal: a plugin brings it, `managed-settings` fixes it, `ccp` projects it from the overlay.
- MCP secrets are shown **masked**, and whatever you leave masked is restored on save — the editor never has to reveal a token to let you change the argument next to it.
- **Layer actions** ("take it to…") move an item to the global layer, to another profile or to a project. It is the same item with another layer, so it lands in the file `core` picks; copying does not delete the original, because the more specific layer still wins.
- After each write it reports where it landed, who it regenerated and which Desktop window keeps the previous MCP servers until you restart it. Every screen shows its CLI equivalent, which is exactly the commands above.

### Editing `ccp.yaml` — `ccp config edit`

```bash
ccp config edit                     # opens ~/.config/ccp/ccp.yaml, then re-reads and validates it
ccp config edit --profile personal-1   # opens that profile's overlay instead
ccp config edit --terminal          # force the terminal editor ($EDITOR / nano)
ccp config gui-editor "code -w"     # pin the GUI editor for good
```

The editor is the **first rung that exists**: `--editor <cmd>` → `defaults.gui_editor` → `$VISUAL` → VS Code and family on the `PATH` (`code` → `cursor` → `code-insiders`, invoked with `-w`) → an OS launcher (`open -W -t`, `notepad`, `xdg-open`) → `defaults.editor` / `$EDITOR` / `nano`. The command tells you which rung won.

**The `-w` is the decision, not a detail.** `code file` returns instantly, and without waiting for the editor to close there is no way to do the one thing that gives this command value: **re-read the YAML and validate it**. A `ccp.yaml` broken by a graphical edit does not show up when you save — it shows up in the next `ccp session`, mid-hop, as a parse error at 3am. So when the editor blocks, `ccp` reloads the file and checks the semantics of `auto_handoff` (every policy through `Effective()`, every fallback profile actually existing) and names the offending key and value if it fails.

When the editor **does not** block (`xdg-open`, or `code` without `-w`) that validation is impossible, and `ccp` says so instead of pretending otherwise — including on the `--profile` path, where it also skips regenerating the `cc-home` and points you at `ccp profile sync <name>` for when you finish.

## `/ccp:` commands — remember and explore artifacts

`ccp` includes five Claude Code commands to persist instructions, agents, hooks and MCP servers straight from the conversation, without editing files by hand:

| Command | What it does |
|---|---|
| `/ccp:remember-global <texto>` | Persists to the global `~/.claude` (all profiles) |
| `/ccp:remember-profile <texto>` | Persists to the active profile's overlay |
| `/ccp:remember-project <texto>` | Persists to the current git repo's `.claude/` (versioned) |
| `/ccp:recall [scope]` | Lists what ccp manages (`global` · `profile` · `project`) |
| `/ccp:forget [scope]` | Deletes by index (lists and confirms first) |

They install with `install.sh` into `~/.claude/commands/ccp/` and become available across all profiles. The equivalent CLI surface is `ccp instruct <add\|list\|rm\|dest\|record>`.

---

## Language

`ccp` speaks English by default and also Spanish. Pick whichever you like; the choice persists in `ccp.yaml`.

```bash
ccp lang              # muestra el idioma actual + de dónde sale (env/config/default)
ccp lang en           # cambia a inglés y lo persiste en ccp.yaml
ccp lang es           # cambia a español y lo persiste en ccp.yaml
```

- `CCP_LANG=en|es` — environment override; it takes precedence over the config. The default is English.
- In the interactive TUI, press **`L`** to toggle the language live (it persists). Note: in the TUI, profile **login** is on lowercase **`l`**, and **`L`** (uppercase) toggles the language.

### TUI keyboard shortcuts

| Key | Action |
|---|---|
| `j` / `k` (or ↓/↑) | Move the selection |
| `Tab` / `Shift+Tab` | Switch panel (Profiles / Rules / Status) |
| `Enter` | Toggle detail (Profiles) |
| `a` | Add |
| `d` | Delete |
| `s` | Set key (DeepSeek) |
| `e` | Open profile view |
| `e` | Edit focused box's file (profile view) |
| `l` | Login (official) |
| `L` | Toggle language (EN/ES) |
| `:` | Command bar |
| `q` / `Ctrl+C` | Quit |

---

## Configure by hand (`ccp.yaml`)

Everything lives in `~/.config/ccp/ccp.yaml` (or `$CCP_HOME/ccp.yaml`). You can touch it with commands or by hand:

```yaml
version: 2
defaults:                  # plantilla para perfiles deepseek NUEVOS
  base_url: https://api.deepseek.com/anthropic
  model_pro: deepseek-chat
  model_flash: deepseek-chat
  effort: high
  editor: nano
profiles:
  work:                    # oficial: solo 'type'
    type: official
  deepseek:                # proveedor: los 4 campos, explícitos
    type: deepseek
    base_url: https://api.deepseek.com/anthropic
    model_pro: deepseek-chat
    model_flash: deepseek-chat
    effort: high
rules:                     # carpeta → perfil (ruta absoluta)
  - path: /Users/tu/work
    profile: work
  - path: /Users/tu/work/cliente-x
    profile: default       # carve-out
authored: []
```

- `default` is **implicit**: never put it in `profiles`. For an exception, use `profile: default` in a rule.
- The API key does **not** go here: it lives in `~/.config/ccp/profiles/<n>/api_key` (`chmod 600`). Edit it with `ccp key <n>`.
- `ccp` writes atomically under a `flock`, preserves your comments, and aborts if `version` is higher than the one it knows.

With commands: `ccp config show` · `ccp config set <clave> <valor>` · `ccp config reset`.

---

## Troubleshooting

| Symptom | Fix |
|---|---|
| `ccp: command not found` | `~/.local/bin` is not on your PATH (Installation step 2). |
| `ccp use …` does nothing | The shell function is missing: `ccp install` and then `source ~/.zshrc`. |
| I changed folders and the profile didn't change | The hook remembers the last folder; refresh with `cd .` |
| Opening Claude says "Not logged in" | That official profile has no session: `ccp profile login <n>`. |
| See a folder's profile without entering it | `ccp resolve ~/ruta/que/sea` |
| How do I switch the output language? | `ccp lang en\|es`, `CCP_LANG=es`, or press `L` in the TUI. |
| Does ccp change anything inside Claude Code? | No. It only points Claude Code at a per-folder profile (its own config dir / provider). Your accounts and settings are untouched. |
| Where are my API keys stored? | Under `~/.config/ccp/profiles/<n>/api_key`, `chmod 600`. Never in `ccp.yaml`, the shell rc, or git. |
| How do I rename a profile? | `ccp profile rename <old> <new>` (or `r` in the TUI). Its rules, rotation chains (`fallback`, `allow_from`, `hooks`), handoff markers and API key move with it; if that terminal had it active, run `ccp use <new>`. An official profile has to log in again (`ccp profile login <new>`): Claude Code names its Keychain credential after the profile's folder, and ccp says so instead of touching the Keychain. If it had a Desktop launcher, ccp prints the two commands that replace it. |
| How do I update ccp? | `ccp upgrade` (re-runs the installer + `profile sync`). |
| How do I uninstall? | `ccp uninstall` (removes the shell block); optionally `rm -rf ~/.config/ccp`. |

## Quick reference

| I want to… | Command |
|---|---|
| Create an official account | `ccp profile add <n> --official` |
| Log in to it | `ccp profile login <n>` |
| Create a DeepSeek provider | `ccp profile add <n> --deepseek` |
| Create a Kimi provider | `ccp profile add <n> --kimi` |
| Create a GLM provider | `ccp profile add <n> --glm` |
| Save its API key | `ccp key <n>` |
| Assign folder → profile | `ccp path set <ruta> <perfil>` |
| Remove a rule | `ccp path rm <ruta>` |
| See rules / profiles | `ccp path list` · `ccp profile list` |
| Switch by hand | `ccp use <n>` · `ccp default` |
| Continue a session under another profile | `ccp handoff [<n>]` |
| Re-enter a live handoff | `ccp handoff resume [<uuid>]` |
| Return a handoff to its origin | `ccp handoff end [<uuid>]` |
| Drop a stale handoff marker | `ccp handoff discard [<uuid>]` |
| See what's in flight | `ccp handoff status [--all]` |
| Trim the handoff history | `ccp handoff prune [--keep N]` |
| Rotate profiles automatically on a limit | `ccp session` (`-p` for headless) |
| See what `ccp session` would do | `ccp session --dry-run` |
| Set up auto-handoff | `ccp auto init` then `ccp auto install` |
| Auto-handoff policy / sensors | `ccp auto status [--json]` |
| Open Claude Desktop with a profile | `ccp desktop open [<n>]` |
| Give each Desktop instance its own name and icon colour | `ccp desktop app [<n>]` |
| See the Code-tab sessions of each Desktop window | `ccp desktop sessions [<n>]` |
| Move a conversation to another profile's Desktop window | `ccp desktop copy <uuid\|title> <n>` |
| Status / diagnostics | `ccp status` · `ccp doctor` |
| Backup / restore | `ccp backup export\|restore` |
| Snapshots | `ccp snapshot create\|list\|diff\|restore\|export\|import` |
| Cloud | `ccp cloud login\|init\|unlock\|push\|pull\|restore\|list\|verify\|devices\|agent\|review\|policy` |
| Add or remove an MCP server | `ccp mcp add\|rm <n>` · `ccp mcp list` |
| Turn an inherited MCP off in one profile | `ccp mcp disable <n> --profile <perfil>` |
| Update | `ccp upgrade` |
| Full help | `ccp help` |

> **For scripting:** `ccp resolve [ruta]` prints the profile (exit `0` = non-default, `1` = default), and `ccp status --json` returns `active`, `profile`, `profile_type`, `cwd` and `repo`.

## Interactive guide

Prefer a visual step-by-step guide? Open [`README.html`](README.html) in your browser — it's a single-page app with a routing playground, tooltips and all the cases:

```bash
open README.html        # macOS
xdg-open README.html    # Linux
```

## Uninstall

```bash
ccp uninstall           # quita la función de shell del rc
rm -rf ~/.config/ccp    # (opcional) borra config y perfiles
```

---

MIT — see [`LICENSE`](LICENSE). Curious how it works under the hood? Check [`CLAUDE.md`](CLAUDE.md) and the [source on GitHub](https://github.com/JoseAFlores777/ccp).

## Disclaimer & trademarks

**Use at your own risk.** This software is provided "as is", without warranty of
any kind (see [`LICENSE`](LICENSE)). You are responsible for your own API keys,
accounts, and configuration.

**Not affiliated.** `ccp` (profiles for Claude Code) is an independent, community
project. It is **not affiliated with, endorsed by, or sponsored by** Anthropic,
DeepSeek, Moonshot AI or Z.ai. "Claude" and "Claude Code" are trademarks of
Anthropic, PBC; "DeepSeek", "Kimi" (Moonshot) and "GLM" (Z.ai) are trademarks of
their respective owners. These names are used only to describe interoperability.
See [`NOTICE`](NOTICE).
