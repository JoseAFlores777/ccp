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
