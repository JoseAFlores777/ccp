<div align="center">

# ccp

**Profiles for Claude Code.**

**English** · [Español](README.es.md)

**A different Claude Code account in every folder.**
In your work repo, your company account; in your personal project, your own; in your experiments, DeepSeek.
The switch happens on its own, just by `cd`-ing.

![version](https://img.shields.io/badge/version-2.6.1-c96442)
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
- a DeepSeek-style **compatible provider** (its `ANTHROPIC_BASE_URL` + API key), or
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

No TTY, or prefer the terminal? Everything is in the CLI, with the same palette:

<div align="center">
<img src="docs/screenshots/cli-help.png" alt="ccp help — colored CLI" width="620">
</div>

---

## Before you start

- macOS or Linux with **bash** or **zsh**.
- **Claude Code** installed (`claude --version` should work).
- **git**.

## Installation

Just once:

```bash
./install.sh          # 1. binario (descarga el release prebuilt y verifica sha256)
ccp install           # 2. función de shell + hook automático
source ~/.zshrc       # 3. recarga TU shell (o ~/.bashrc)
ccp doctor            # 4. confirma que quedó bien
```

If step 1 warns you that `~/.local/bin` is not on your PATH, add it to your rc:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

## Updating

`install.sh` records which repo you installed from, so updating is a single command:

```bash
ccp upgrade            # re-instala + re-sincroniza perfiles (profile sync)
ccp upgrade --pull     # hace 'git pull' antes de re-instalar
ccp upgrade --no-sync  # solo el binario, sin tocar perfiles
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

### Case 3 — A folder with DeepSeek

```bash
ccp profile add deepseek --deepseek   # perfil de proveedor
ccp key deepseek                      # guarda la API key (te la pide oculta)
ccp path set ~/labs deepseek
cd ~/labs && claude
```

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

## Backup and restore

Take everything to another machine, or back it up before a big change:

```bash
ccp backup export ~/ccp-backup.tar.gz                 # ccp.yaml + overlays
ccp backup export ~/ccp-backup.tar.gz --with-secrets  # + api_key + logins (chmod 600)
ccp backup restore ~/ccp-backup.tar.gz                # no pisa; fusiona reglas
ccp backup restore ~/ccp-backup.tar.gz --overwrite    # reemplaza perfiles del backup
ccp backup restore ~/ccp-backup.tar.gz --force        # borra todo y restaura limpio
```

Before restoring, `ccp` saves an automatic snapshot in `~/.config/ccp/.backup-pre-restore-<fecha>`.

## Per-profile config

Each profile has its own Claude config, applied as a **baseline layer** when it's active:

```bash
ccp profile config <perfil>                 # menú: instrucciones / settings / ambos
ccp profile config <perfil> instructions    # abre overlay/CLAUDE.md
ccp profile config <perfil> settings         # abre overlay/settings.overlay.json
ccp profile sync [<perfil>]                  # re-mergea cambios del global ~/.claude
ccp config editor "code -w"                  # editor a usar (fallback: $EDITOR)
```

- **Instructions**: `cc-home/CLAUDE.md` does an `@import` of the global `~/.claude/CLAUDE.md` and then of your overlay.
- **Settings**: `cc-home/settings.json` = global ⊕ overlay (pure-Go deep-merge).
- **Real precedence**: it's a baseline — the repo's config (`.claude/settings.json`) wins on conflict.
- `default` has no overlay: `ccp profile config default` opens your global `~/.claude` directly.

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

## Quick reference

| I want to… | Command |
|---|---|
| Create an official account | `ccp profile add <n> --official` |
| Log in to it | `ccp profile login <n>` |
| Create a DeepSeek provider | `ccp profile add <n> --deepseek` |
| Save its API key | `ccp key <n>` |
| Assign folder → profile | `ccp path set <ruta> <perfil>` |
| Remove a rule | `ccp path rm <ruta>` |
| See rules / profiles | `ccp path list` · `ccp profile list` |
| Switch by hand | `ccp use <n>` · `ccp default` |
| Status / diagnostics | `ccp status` · `ccp doctor` |
| Backup / restore | `ccp backup export\|restore` |
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

MIT — see [`LICENSE`](LICENSE). Curious how it works under the hood? Check [`CLAUDE.md`](CLAUDE.md) and `docs/superpowers/specs/ccp-profiles.html`.

## Disclaimer & trademarks

**Use at your own risk.** This software is provided "as is", without warranty of
any kind (see [`LICENSE`](LICENSE)). You are responsible for your own API keys,
accounts, and configuration.

**Not affiliated.** `ccp` (profiles for Claude Code) is an independent, community
project. It is **not affiliated with, endorsed by, or sponsored by** Anthropic
or DeepSeek. "Claude" and "Claude Code" are trademarks of Anthropic, PBC;
"DeepSeek" is a trademark of its respective owner. These names are used only to
describe interoperability. See [`NOTICE`](NOTICE).
