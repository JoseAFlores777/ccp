# i18n para ccp — terminal bilingüe (EN default) + README EN/ES

**Versión** v0.1 · **Fecha** 2026-06-02 · **Status** gate pasado · listo para plan · **Scope** i18n de terminal + README/HTML bilingüe

## 01 · Overview

Hoy todo el texto de cara al usuario de `ccp` está hardcodeado en español (convención del repo: *"keep user-facing text Spanish"*). Este spec lo internacionaliza: la terminal ofrece **inglés y español**, el usuario elige, y **inglés es el default**. Lo mismo aplica al `README.md` y su companion `README.html`.

El cambio invierte la convención existente (EN pasa a ser el default), pero la mantiene como opción. No es un rebrand ni cambia comportamiento funcional — solo la capa de presentación textual.

Tres superficies se internacionalizan:

1. **Terminal** — salida CLI (help, errores, mensajes, status) y TUI (labels, hints, paneles).
2. **README.md** — EN default + `README.es.md`.
3. **README.html** — EN default + `README.es.html`.

Lo que **no** se toca: las superficies idioma-neutrales (nombres de perfil, exports de env, claves de `status --json`, word-lists de completion), el bloque shell-init / función rc, y `install.sh` (queda EN-only).

## 02 · Goals / Non-goals

### Goals

- Terminal en EN o ES, seleccionable por el usuario, **EN por defecto**.
- Traducir las ~163 strings de cara al usuario (cli + core + tui) a un catálogo `en`/`es`.
- Selección por **3 vías**: comando `ccp lang en|es` (persistente), env `CCP_LANG` (override por invocación), toggle en la TUI.
- Mantener verde el gate de paridad golden contra el oráculo bash (congelado en español).
- README + HTML bilingües, con selector de idioma entre versiones.

### Non-goals

- No localizar surfaces máquina (resolve / `_env` / `_hook` / `status --json` keys / completion).
- No localizar el bloque shell-init / función rc (plumbing de una vez, contrato congelado).
- No localizar `install.sh` (corre antes de existir config; queda EN-only).
- Sin librería i18n externa (se respeta la filosofía hand-dispatch del repo).
- Sin auto-detección de locale: EN siempre es el default; ES es explícito.
- Sin pluralización compleja ni más idiomas que EN/ES en v1.

## 03 · Resolución de idioma

Orden de precedencia, primero gana:

1. **`CCP_LANG`** (env) — `en` o `es`. Override por invocación, no persiste.
2. **`lang:`** en `ccp.yaml` — preferencia persistente del usuario.
3. **`en`** — default duro.

Cualquier valor inválido (`CCP_LANG=fr`, typo) cae a `en` silenciosamente. La resolución vive en `i18n.Resolve(cfgLang string) Lang` y se calcula una vez por invocación, cerca del top de cada comando que imprime prosa.

## 04 · Paquete `internal/core/i18n`

Motor mínimo, sin deps:

```go
type Lang string
const (
    En Lang = "en"
    Es Lang = "es"
)

// catalog[key][lang] = plantilla. Keys = IDs estables por área.
var catalog = map[string]map[Lang]string{ /* ... */ }

// T busca key→lang; si falta el lang, fallback a En; si falta la key,
// devuelve la key cruda (ruidoso, para detectar huérfanas). Aplica Sprintf.
func T(l Lang, key string, a ...any) string

// Resolve aplica la precedencia env → cfg → En.
func Resolve(cfgLang string) Lang
```

- **Keys**: IDs estables namespaced por área, p.ej. `profile.added`, `err.profile.notfound`, `status.active`, `tui.panel.profiles`. Convención: `<area>.<concepto>[.<detalle>]`.
- **Args**: las plantillas usan verbos `fmt` (`%s`, `%q`, `%d`); `T` hace `Sprintf`.
- **Fallback ruidoso**: key faltante → devuelve la key literal (visible en pantalla), no panic. Un test la caza antes de release.

Los front-ends (`internal/cli`, `internal/tui`) reciben el `Lang` resuelto y llaman `i18n.T(lang, "key", args...)` donde hoy hay literales. `internal/core` sigue devolviendo **datos/errores estructurados**, no prosa traducida: los errores de cara al usuario se mapean a keys en la capa cli/tui (se mantiene la regla "core no imprime presentación").

## 05 · Storage — `ccp.yaml`

Nuevo campo opcional de nivel superior:

```yaml
version: 2
lang: en        # en | es ; ausente = en
defaults: { ... }
profiles: { ... }
rules: [ ... ]
```

- Se añade `Lang string \`yaml:"lang"\`` al struct `Config` y `"lang"` a `knownTopKeys`.
- **Schema sigue en version 2**: el campo es aditivo y backward-compatible. Binarios viejos lo preservan vía el catch-all `Extra` en el round-trip; binarios nuevos lo leen del campo tipado.
- Ausente o vacío ⇒ `en` (vía `Resolve`).

## 06 · Selección: comando `ccp lang` + TUI

### Comando

```
ccp lang            # muestra idioma actual + fuente (env / config / default)
ccp lang en         # persiste lang: en en ccp.yaml, confirma
ccp lang es         # persiste lang: es en ccp.yaml, confirma
```

- Se añade `case "lang":` al dispatch en `internal/cli/cli.go`.
- `ccp lang <x>` con `<x>` inválido → error traducido + exit no-cero.
- Se añade `lang` al word-list de completion en `core/shellinit.go` (`top="… lang …"`). Esto toca un string verbatim del contrato → **se actualiza el oráculo bash y se regenera el golden en lockstep** (ver §08).

### TUI

- Tecla **`L`** cicla `en ↔ es`, persiste en `ccp.yaml`, re-renderiza con el nuevo idioma.
- El footer de teclas y la línea de status muestran el binding y el idioma activo.

## 07 · Alcance de traducción

| Superficie | ¿Se traduce? | Nota |
|---|---|---|
| CLI help (`help.go`) | **Sí** | bloque grande de prosa |
| CLI errores / mensajes / status text | **Sí** | ~mayoría de las 163 strings |
| TUI labels / hints / paneles / footer | **Sí** | `internal/tui/*` |
| Errores de core de cara al usuario | **Sí** (vía keys en cli/tui) | core sigue devolviendo datos/errores estructurados |
| `resolve` (nombre de perfil) | No | idioma-neutral |
| `_env` / `_hook` (exports) | No | contrato máquina |
| `status --json` (claves) | No | contrato máquina |
| completion word-lists | No (salvo añadir `lang`) | nombres de comando en inglés |
| bloque shell-init / función rc (comentarios) | **No** | plumbing one-time, contrato congelado |
| `install.sh` mensajes | **No** (EN-only) | corre antes de existir config |

## 08 · Paridad golden / oráculo bash

El oráculo bash en `legacy/` es el contrato congelado y está en español. La estrategia:

- `testdata/golden/capture.sh` y `internal/golden/parity_test.go` corren el binario Go con **`CCP_LANG=es`** para igualar al oráculo. Las surfaces capturadas que llevan prosa (`env-missing`, etc.) salen en español y matchean.
- Las surfaces máquina (`resolve`, `_env`, `_hook`, `status --json`, completion code) son idioma-neutrales y no cambian.
- **Único cambio de contrato**: el word-list de completion gana `lang`. Se actualiza el heredoc `COMPLETION_BASH`/`COMPLETION_ZSH` del oráculo bash, se regenera el golden con `capture.sh`, y se mantiene `parity_test.go` verde.
- El oráculo bash **no se traduce**: queda como está.

## 09 · README bilingüe

- `README.md` = **EN** (default, traducido del actual). `README.es.md` = **ES** (curado del contenido actual).
- `README.html` = **EN**. `README.es.html` = **ES** (estilo Anthropic ya existente; se traduce el contenido, se conserva paleta/tipografía/layout).
- **Selector** arriba de cada archivo: en EN, `**English** · [Español](README.es.md)`; en ES, `[English](README.md) · **Español**`. Equivalente en los HTML (un par de links arriba del hero).
- `testdata/golden/README.md` es otro archivo (readme del fixture) y **no se toca**.

## 10 · Testing

- `i18n_test.go`:
  - `Resolve`: orden env → cfg → En; valores inválidos → En.
  - `T`: args/Sprintf; key faltante → devuelve la key; lang faltante → fallback En.
- **Test de completitud**: toda key referenciada existe con **ambos** `en` y `es` (no huérfanas, no incompletas). Recorre el catálogo y, si es viable, las keys usadas en el código.
- **Parity gate** verde con `CCP_LANG=es` (incluye el nuevo `lang` en completion).
- **Smoke TUI**: render del dashboard en `en` y en `es` no rompe (espejo del preview existente).
- `gofmt` / `go vet` / `golangci-lint` limpios.

## 11 · Riesgos & pendientes

| # | Item | Status |
|---|---|---|
| 11.1 | Completion verbatim: añadir `lang` toca string del contrato | Mitigado — oráculo + golden en lockstep (§08) |
| 11.2 | Volumen: ~163 strings a traducción manual | Mecánico pero grande — se hace por áreas/tandas |
| 11.3 | Keys huérfanas/incompletas al migrar literales | Mitigado — test de completitud (§10) |
| 11.4 | Deriva README EN/ES (se actualiza uno y no el otro) | Abierto — convención: editar ambos en lockstep, nota en CONTRIBUTING/CLAUDE.md |
| 11.5 | `version_test.go` y otros tests asertan prosa ES literal | Auditar — migrarlos a comparar contra `i18n.T(Es, key)` o fijar lang |

## 12 · Criterios de aceptación

- `ccp help` sin config previa sale en **inglés**; `CCP_LANG=es ccp help` sale en español.
- `ccp lang es` persiste; una invocación posterior (sin env) sale en español; `ccp lang` reporta `es` con fuente `config`.
- `CCP_LANG=en ccp lang` (con `lang: es` en config) reporta `en` con fuente `env`.
- TUI: tecla `L` cambia el idioma en vivo y persiste.
- Toda key tiene `en` + `es`; ninguna pantalla muestra una key cruda.
- `README.md`/`README.html` en inglés con selector a las versiones ES; `README.es.*` en español.
- `go test ./...` verde, incluido el parity gate (`CCP_LANG=es`) con `lang` en completion.

## 13 · Distribución / compartir

> Apéndice añadido a pedido: tangencial a i18n, pero deja por escrito el canal de distribución.

`ccp` es un binario Go multiplataforma (darwin/linux × amd64/arm64) ya distribuido por **GitHub Releases** (prebuilt + `checksums.txt`) e `install.sh` (`release.yml` corre en cada tag `v*`). Para compartir **solo con quien reciba el enlace**, ese es el canal elegido:

- Compartes el one-liner `curl -fsSL <url>/install.sh | sh` (o el link directo al asset del release).
- **Link-only real**: le das la URL a quien quieras; nada se publica en un registro descubrible.
- **Sin Node**: el instalador detecta OS/arch, baja el binario del release y verifica `sha256`. El recipiente no necesita toolchain.
- Versionado por tag: pinear a `vX.Y.Z` apuntando al asset de ese release.

**npm / pnpm / npx — descartado por ahora (paso futuro).** npm y pnpm son *gestores* (instalan lo mismo); npx solo *ejecuta* un paquete npm — no es un canal aparte. Publicar a npm daría `npx ccp` sin instalación, pero:

- El registro npm es **público y descubrible** → contradice el constraint link-only.
- Un binario Go en npm exige un **wrapper** que baje el release en `postinstall` (patrón esbuild) + **Node** en la máquina del recipiente.
- npm privado/scoped sería link+token gated, pero añade npm auth (de pago) y fricción.

Evolución: si más adelante se quiere distribución pública con ergonomía `npx ccp`, se añade el wrapper npm **sobre los mismos GitHub Releases** (los binarios no cambian).

## 14 · Plan por fases

1. **Paquete i18n** — `internal/core/i18n`: `Lang`, `Resolve`, `T`, catálogo vacío + tests del motor.
2. **Storage + comando** — campo `lang` en `Config`/`knownTopKeys`; `ccp lang` (show/set); `lang` en completion + oráculo + regolden.
3. **Catálogo CLI** — migrar literales de `internal/cli` (help, errores, status, mensajes) a keys en/es.
4. **Catálogo TUI** — migrar `internal/tui` (labels/hints/paneles/footer); toggle `L`.
5. **Core errors** — mapear errores de cara al usuario a keys en la capa cli/tui; auditar tests que asertan prosa.
6. **README bilingüe** — `README.md`/`README.es.md` + `README.html`/`README.es.html` + selectores.
7. **Verify** — completitud de keys, parity verde, smoke EN/ES, criterios de aceptación end-to-end.

---

*ccp spec · terminal bilingüe + README EN/ES · v0.1 — 2026-06-02*
