# ccp

**Una cuenta de Claude Code distinta en cada carpeta.** En tu repo de trabajo arranca con tu cuenta de la empresa; en tu proyecto personal, con tu cuenta personal; en tu carpeta de experimentos, con DeepSeek. El cambio es automático: ocurre solo con hacer `cd`.

---

## El modelo mental (30 segundos)

Tres ideas y ya:

1. Un **perfil** es una identidad: una cuenta oficial de Anthropic, un proveedor tipo DeepSeek, o tu login normal (`default`).
2. Una **regla** dice "esta carpeta (y sus subcarpetas) usa tal perfil".
3. Gana la regla **más específica**. Sin regla → usas tu login normal.

```
~/work          → perfil "work"      (cuenta empresa)
~/work/cliente  → perfil "default"   (carve-out: tu login normal)
~/personal      → perfil "personal"  (cuenta personal)
~/labs          → perfil "deepseek"
~               → default
```

Cuando entras a una carpeta, `ccp` aplica el perfil correcto en esa terminal. Luego corres `claude` normal.

---

## Antes de empezar

Necesitas:

- macOS o Linux, con **bash** o **zsh**.
- **Claude Code** instalado (que `claude --version` funcione).
- **git**.

---

## Instalación

Hazlo una sola vez. Un paso a la vez.

**Paso 1 — Instala el binario y las librerías.**

```bash
./install.sh
```

**Paso 2 — Asegúrate de que `~/.local/bin` está en tu PATH.** Si el paso 1 te mostró una advertencia de PATH, añade esta línea a tu `~/.zshrc` (o `~/.bashrc`):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

**Paso 3 — Instala la función de shell y el hook automático.**

```bash
ccp install
```

**Paso 4 — Recarga tu shell.** Usa el archivo de TU shell:

```bash
source ~/.zshrc
```

```bash
source ~/.bashrc
```

**Paso 5 — Confirma que quedó bien.**

```bash
ccp doctor
```

Si ves "Función 'ccp' cargada ✅", estás listo.

---

## Actualizar a una versión nueva

`install.sh` registra de qué repo instalaste, así que actualizar es un comando:

```bash
ccp upgrade
```

Eso re-instala el binario y las librerías desde ese repo, migra/regenera tus perfiles al modelo nuevo (`profile sync`), y te dice de qué versión a cuál subiste. Variantes:

```bash
ccp upgrade --pull       # hace 'git pull' en el repo antes de re-instalar
ccp upgrade --no-sync    # no re-mergea los perfiles (solo binario + libs)
```

Notas:

- Si cambió la función de shell o el hook, `ccp upgrade` te **avisa** (no toca tu rc); aplícalo con `ccp uninstall && ccp install && source ~/.zshrc`.
- Para ver completions nuevos, abre una terminal nueva o haz `source` de tu rc.
- Sin `ccp upgrade` (instancias viejas) el equivalente manual es: `bash install.sh && ccp profile sync`.

---

## Caso 1 — Tu cuenta de trabajo en tu carpeta de trabajo

El caso más común. Paso a paso.

**Paso 1 — Crea el perfil oficial "work".**

```bash
ccp profile add work --official
```

**Paso 2 — Inicia sesión en ese perfil (una sola vez).** Esto abre Claude Code con la configuración aislada de "work":

```bash
ccp profile login work
```

Dentro de Claude Code: escribe `/login`, autentícate con tu cuenta de trabajo, y sal con `/quit`.

**Paso 3 — Dile a `ccp` qué carpeta usa "work".**

```bash
ccp path set ~/work work
```

**Paso 4 — Pruébalo.** Entra a la carpeta y abre Claude:

```bash
cd ~/work && claude
```

Arranca con tu cuenta de trabajo. No tuviste que hacer nada más.

---

## Caso 2 — Añadir tu cuenta personal

Igual que el caso 1, con otro nombre y otra carpeta.

**Paso 1 — Crea el perfil.**

```bash
ccp profile add personal --official
```

**Paso 2 — Inicia sesión (una vez), con tu cuenta personal.**

```bash
ccp profile login personal
```

Dentro: `/login` con tu cuenta personal, luego `/quit`.

**Paso 3 — Asigna tu carpeta personal.**

```bash
ccp path set ~/personal personal
```

Listo. `~/work` sigue siendo trabajo, `~/personal` ahora es personal.

---

## Caso 3 — Una carpeta con DeepSeek

Para experimentos o trabajo donde prefieras DeepSeek en vez de tu cuenta Anthropic.

**Paso 1 — Crea el perfil de proveedor.**

```bash
ccp profile add deepseek --deepseek
```

**Paso 2 — Guarda tu API key de DeepSeek.** Te la pedirá oculta:

```bash
ccp key deepseek
```

**Paso 3 — Asigna la carpeta.**

```bash
ccp path set ~/labs deepseek
```

**Paso 4 — Úsalo.**

```bash
cd ~/labs && claude
```

---

## Caso 4 — Sacar una subcarpeta de su regla (carve-out)

Tu carpeta `~/work` usa la cuenta de trabajo, pero hay un cliente puntual donde quieres tu login normal.

**Un solo paso — Asigna esa subcarpeta al perfil `default`.**

```bash
ccp path set ~/work/cliente-x default
```

Ahora `~/work` sigue en "work", pero `~/work/cliente-x` (y lo que tenga dentro) usa tu login normal. La subcarpeta más específica siempre manda.

---

## Caso 5 — Cambiar a mano en una terminal

A veces quieres cambiar el perfil de la terminal actual sin moverte de carpeta.

**Activar un perfil aquí:**

```bash
ccp use personal
```

**Volver a tu login normal:**

```bash
ccp default
```

**Correr Claude una sola vez con el perfil de la carpeta, sin dejarlo fijo:**

```bash
ccp run claude
```

---

## Caso 6 — Ver qué está pasando

**¿Qué perfil usa esta carpeta y qué hay activo en la terminal?**

```bash
ccp status
```

**¿Qué reglas de carpeta tengo?**

```bash
ccp path list
```

**¿Qué perfiles tengo creados?**

```bash
ccp profile list
```

**¿Está todo bien (logins, keys, función de shell)?**

```bash
ccp doctor
```

---

## Caso 7 — Vengo de `dsctl` (la versión vieja)

`ccp` reemplaza a `dsctl`. La migración es automática.

**Paso 1 — La primera vez que corras `ccp`, migra solo.** Crea un perfil `deepseek` con tu configuración vieja y convierte tus reglas. No tienes que hacer nada especial; solo úsalo:

```bash
ccp status
```

**Paso 2 — Quita el bloque viejo de `dsctl` de tu shell.**

```bash
dsctl uninstall
```

Tus reglas viejas quedan respaldadas en `~/.config/ccp/rules.dsctl.bak`.

---

## Config por perfil

Cada perfil (official o deepseek) tiene su propia config de Claude que se aplica como **capa baseline** cuando el perfil está activo:

```bash
ccp profile config <perfil>                 # menú: instrucciones / settings / ambos
ccp profile config <perfil> instructions    # abre overlay/CLAUDE.md
ccp profile config <perfil> settings        # abre overlay/settings.overlay.json
ccp profile sync [<perfil>]                 # re-mergea cambios del global ~/.claude
ccp config editor "code -w"                 # editor a usar (fallback: $EDITOR)
```

- **Instrucciones**: `cc-home/CLAUDE.md` hace `@import` del global `~/.claude/CLAUDE.md` y luego de tu overlay.
- **Settings**: `cc-home/settings.json` = global ⊕ overlay (deep-merge con `jq`; sin `jq` cae a snapshot del global).
- **Prioridad real**: es una baseline — la config del repo (`.claude/settings.json`) gana en conflicto; las instrucciones se inyectan siempre como contexto. Ver `docs/adr/0001`.
- `default` no tiene overlay: `ccp profile config default` abre tu `~/.claude` global directo (con aviso).

---

## Comandos `/ccp:` — recordar y explorar artefactos

`ccp` incluye cinco comandos de Claude Code que te permiten persistir instrucciones, agents, hooks y servidores MCP directamente desde la conversación, sin editar archivos a mano.

**Comandos disponibles:**

| Comando | Qué hace |
|---|---|
| `/ccp:remember-global <texto>` | Persiste al `~/.claude` global (aplica a todos los perfiles) |
| `/ccp:remember-profile <texto>` | Persiste al overlay del perfil activo (error si es `default`) |
| `/ccp:remember-project <texto>` | Persiste al `.claude/` del repo git actual (se versiona con el código) |
| `/ccp:recall [scope]` | Lista lo que ccp gestiona (`global`, `profile`, `project`; sin argumento = los tres) |
| `/ccp:forget [scope]` | Borra por índice (lista y pide confirmación antes de borrar) |

Los comandos se instalan con `install.sh` en `~/.claude/commands/ccp/` y quedan disponibles en todos los perfiles vía el symlink `commands/` de cada cc-home.

### Qué puedes guardar (scope × tipo)

Cada comando clasifica automáticamente el artefacto según lo que pides (`rule`, `agent`, `command`, `skill`, `hook`, `mcp`) y lo escribe en la ruta oficial de Claude Code:

| tipo | `global` | `project` | `profile` |
|---|---|---|---|
| `rule` | `~/.claude/CLAUDE.md` (bloque gestionado) | `.claude/CLAUDE.md` (bloque gestionado) | `overlay/CLAUDE.md` (bloque gestionado) |
| `hook` | `~/.claude/settings.json` | `.claude/settings.json` | `overlay/settings.overlay.json` (+ regen) |
| `mcp` | `~/.claude.json` | `.mcp.json` | ❌ no soportado — usa `global` o `project` |
| `agent` | `~/.claude/agents/` | `.claude/agents/` | ❌ se comparten desde global vía symlink |
| `command` | `~/.claude/commands/` | `.claude/commands/` | ❌ se comparten desde global vía symlink |
| `skill` | `~/.claude/skills/` | `.claude/skills/` | ❌ se comparten desde global vía symlink |

Notas importantes:

- **Reglas (`rule`)**: viven dentro de un bloque con marcadores en `CLAUDE.md` (lo gestiona `ccp`; nunca edites ese bloque a mano). El bloque se `@import`a automáticamente en cada cc-home, así que los cambios no requieren regen.
- **Agents, commands, skills**: Claude escribe el archivo en la ruta que devuelve `ccp instruct dest <scope> <type>` y lo registra con `ccp instruct record`. A nivel `profile` no se soportan (se heredan desde global vía symlink).
- **Hooks y MCP**: requieren `jq`. El **borrado de hooks es manual** — `forget`/`rm` quitan la entrada del manifest, pero el bloque en `settings.json` hay que eliminarlo a mano (o via `ccp profile config <perfil> settings`).
- **CRUD seguro**: `recall`/`forget` solo ven lo que `ccp` creó — reglas (bloque gestionado) y un **manifest de artefactos** (global/profile en `~/.config/ccp/authored.tsv`; project en `.claude/ccp-authored.tsv`, versionado con el repo).

### Superficie CLI (`ccp instruct`)

Los comandos `/ccp:` son wrappers sobre esta API que también puedes usar desde scripts:

```bash
ccp instruct add <scope> <type> <texto>          # añade rule/hook/mcp/agent/command/skill
ccp instruct add <scope> mcp 'name={"command":"...","args":[...]}'  # server MCP
ccp instruct list <scope>                         # lista lo gestionado
ccp instruct rm <scope> <index>                   # borra por índice
ccp instruct dest <scope> <type>                  # imprime la ruta destino oficial
ccp instruct record <scope> <type> <ref> <desc>   # registra un artefacto ya escrito
```

`scope` = `global` | `profile` | `project`. Ver `docs/adr/0004` (bloque de reglas) y `docs/adr/0005` (routing + manifest).

---

## Solución de problemas

**`ccp: command not found`**
`~/.local/bin` no está en tu PATH. Vuelve al Paso 2 de Instalación.

**`ccp use ...` no cambia nada / "Función 'ccp' no cargada"**
Falta cargar la función de shell. Corre `ccp install` y luego `source ~/.zshrc` (o `~/.bashrc`).

**Cambié de carpeta y el perfil no cambió.**
El hook recuerda la última carpeta. Tras editar reglas, refresca con:

```bash
cd .
```

**Al abrir Claude dice "Not logged in".**
Ese perfil oficial todavía no tiene sesión. Inícialo una vez:

```bash
ccp profile login work
```

**Quiero ver el perfil de una carpeta sin entrar en ella.**

```bash
ccp resolve ~/ruta/que/sea
```

---

## Desinstalar

**Paso 1 — Quita la función de shell.**

```bash
ccp uninstall
```

**Paso 2 — (Opcional) Borra tu configuración y perfiles.**

```bash
rm -rf ~/.config/ccp
```

---

## Referencia rápida

Cuando ya le agarraste la onda, esto es todo el mapa:

| Quiero… | Comando |
|---|---|
| Crear cuenta oficial | `ccp profile add <n> --official` |
| Iniciar sesión en ella | `ccp profile login <n>` |
| Crear proveedor DeepSeek | `ccp profile add <n> --deepseek` |
| Guardar su API key | `ccp key <n>` |
| Asignar carpeta → perfil | `ccp path set <ruta> <perfil>` |
| Quitar una regla | `ccp path rm <ruta>` |
| Ver reglas / perfiles | `ccp path list` · `ccp profile list` |
| Cambiar a mano | `ccp use <n>` · `ccp default` |
| Estado / diagnóstico | `ccp status` · `ccp doctor` |
| Ayuda completa | `ccp help` |

> **Para scripting:** `ccp resolve [ruta]` imprime el perfil (exit `0` = no-default, `1` = default), y `ccp status --json` devuelve un objeto JSON con `active`, `profile`, `profile_type`, `cwd` y `repo`.

---

MIT. ¿Curioso de cómo funciona por dentro? Mira `CLAUDE.md` y `docs/superpowers/specs/ccp-profiles.html`.
