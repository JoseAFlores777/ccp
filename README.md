# dsctl

> Alterna **Claude Code** entre tu **login oficial de Anthropic** y los modelos de
> **DeepSeek**, controlado **por terminal** y **por carpeta** mediante reglas de
> path `include` / `exclude` administrables desde un CLI interactivo.

Las variables `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` que usa DeepSeek
**anulan tu login**. Si las pones globales, contaminan **todas** tus sesiones —
incluidas las de empresa. `dsctl` las activa **solo donde tú definas** y nunca de
forma global, así tu login oficial queda intacto.

---

## Instalación

```bash
git clone https://github.com/<tu-usuario>/dsctl.git
cd dsctl
bash install.sh          # copia binario y librería a ~/.local
dsctl install            # añade la función 'ds' + hook a tu shell
source ~/.bashrc         # o ~/.zshrc
dsctl key                # guarda tu API key DeepSeek (entrada oculta)
```

Requisitos: Node.js 18+ y Claude Code (`npm install -g @anthropic-ai/claude-code`).
Verifica con `dsctl doctor`.

---

## Reglas de PATH — el núcleo

Defines **dónde** corre DeepSeek con dos tipos de regla:

| Regla | Efecto |
|-------|--------|
| `include` | Bajo este path (y subcarpetas) DeepSeek se enciende. |
| `exclude` | Bajo este path **nunca**, aunque herede un `include`. |

**Precedencia:** gana la regla más específica (la ruta más profunda). En empate
exacto, `exclude` gana. Sin reglas que apliquen → Claude Code oficial.

```bash
dsctl path include ~/work              # todo ~/work usa DeepSeek
dsctl path exclude ~/work/cliente      # ...menos este cliente
dsctl path include ~/work/cliente/lab  # ...pero su carpeta 'lab' sí
dsctl path list                        # ver el árbol de reglas
dsctl path test ~/work/cliente/lab/x   # → 🟢 DeepSeek
```

Con el hook instalado, al hacer `cd` a una carpeta el modo se ajusta solo:

```bash
cd ~/work/proj         # 🟢 DeepSeek (auto)
cd ~/work/cliente      # ⚪ oficial   (auto)
cd ~/work/cliente/lab  # 🟢 DeepSeek (auto)
```

---

## Uso por terminal (manual)

```bash
ds on            # 🟢 DeepSeek SOLO en esta terminal
claude
ds off           # ⚪ vuelve al login oficial
ds run           # enciende, corre claude, restaura al salir
```

## Comandos

```
dsctl                       menú interactivo
dsctl path include [ruta]   incluir (Enter = carpeta actual)
dsctl path exclude [ruta]   excluir
dsctl path rm [ruta]        quitar regla
dsctl path list             listar includes/excludes + regla del cwd
dsctl path test [ruta]      ver veredicto de una ruta
dsctl path clear            borrar todas las reglas
dsctl path edit             abrir reglas en $EDITOR
dsctl status                estado de la terminal + regla del cwd
dsctl key [API_KEY]         guardar API key (oculta si se omite)
dsctl config show|set|reset modelos / URL
dsctl doctor                diagnóstico
dsctl install|uninstall     gestionar la función de shell
```

---

## ¿Por qué función de shell + binario?

Un binario corre en proceso hijo y **no puede** alterar el entorno del shell padre.
Por eso `ds on/off` vive como **función** (instalada en tu rc) y el resto de la
lógica —reglas, key, config, menú, resolución de paths— vive en el binario `dsctl`,
que la función invoca. El hook llama a `dsctl _resolve "$PWD"` en cada prompt y
enciende/apaga según el veredicto.

## Seguridad

- API key en `~/.config/dsctl/api_key` con permisos `600`; nunca en el rc ni en git.
- `status` solo muestra los últimos 4 caracteres de la key.
- Las reglas viven en `~/.config/dsctl/rules.tsv` (texto plano editable).

## Desinstalar

```bash
dsctl uninstall
rm -f ~/.local/bin/dsctl ~/.local/lib/dsctl/paths.sh
rm -rf ~/.config/dsctl
```

## Estructura

```
dsctl/
├── bin/dsctl          # CLI principal
├── lib/paths.sh       # motor de reglas include/exclude (testeable)
├── install.sh
├── .github/workflows/ci.yml
├── CHANGELOG.md · LICENSE · README.md
```

MIT.
