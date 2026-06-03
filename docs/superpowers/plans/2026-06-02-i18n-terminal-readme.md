# i18n para ccp — terminal bilingüe (EN default) + README EN/ES — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Regla del repo (override del usuario):** NUNCA correr `git commit`/`git push` sin autorización explícita del usuario en el turno. Los pasos "Commit" de abajo describen el commit a proponer; al ejecutar, **staged + pedir autorización**, no comitear autónomo. Y NUNCA añadir trailer `Co-Authored-By`.

**Goal:** Internacionalizar la salida de terminal de `ccp` (CLI + TUI) a inglés/español seleccionable con EN por defecto, y producir README.md/README.html bilingües.

**Architecture:** Un paquete nuevo `internal/core/i18n` provee `Lang`, `Resolve` (precedencia env→config→en) y `T(lang, key, args...)` sobre un catálogo `map[key]map[Lang]string`. Los front-ends (`internal/cli`, `internal/tui`) reemplazan literales por `i18n.T(...)`; `internal/core` sigue devolviendo datos/errores estructurados (la prosa se mapea a keys en cli/tui). El gate de paridad golden corre con `CCP_LANG=es` para igualar al oráculo bash congelado en español.

**Tech Stack:** Go 1.24, sin libs i18n externas (dispatch a mano, filosofía del repo). YAML store existente (`gopkg.in/yaml`). bubbletea/lipgloss para TUI. Golden harness bash + `internal/golden/parity_test.go`.

---

## File Structure

**Nuevos:**
- `internal/core/i18n/i18n.go` — `Lang`, `Resolve`, `ResolveWithSource`, `Source`.
- `internal/core/i18n/catalog.go` — motor `T` + `register` + `catalog` global.
- `internal/core/i18n/catalog_cli.go` — entradas en/es del área CLI.
- `internal/core/i18n/catalog_tui.go` — entradas en/es del área TUI.
- `internal/core/i18n/catalog_core.go` — entradas en/es de errores de core de cara al usuario.
- `internal/core/i18n/i18n_test.go` — tests de `Resolve`/`T`.
- `internal/core/i18n/catalog_test.go` — test de completitud (toda key tiene en+es).
- `internal/cli/i18n.go` — helper `currentLang()` para paths sin cfg cargada.
- `internal/cli/lang.go` — comando `ccp lang`.
- `internal/cli/lang_test.go` — tests del comando.
- `README.es.md`, `README.es.html` — versiones ES.

**Modificados:**
- `internal/core/store.go` — campo `Lang` en `Config` + `knownTopKeys`.
- `internal/cli/cli.go` — `case "lang":` en dispatch; `Comando desconocido`/`use/default/...` a i18n.
- `internal/cli/*.go` — literales → `i18n.T`.
- `internal/tui/*.go` — literales → `i18n.T`; toggle `L`.
- `internal/core/shellinit.go` — `lang` en los 3 word-lists de completion (bash + 2 zsh).
- `legacy/bin/ccp` — `lang` en los 2 word-lists del oráculo (líneas 620, 641).
- `internal/golden/parity_test.go` — `CCP_LANG=es` en `cmd.Env`.
- `README.md`, `README.html` — traducidos a EN + selector de idioma.
- `testdata/golden/basic/expected/completion-*.out` — regenerados con `lang`.

---

## Phase 1 — Motor i18n

### Task 1: Paquete i18n — Lang + Resolve

**Files:**
- Create: `internal/core/i18n/i18n.go`
- Test: `internal/core/i18n/i18n_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/core/i18n/i18n_test.go
package i18n

import "testing"

func TestResolvePrecedence(t *testing.T) {
	cases := []struct {
		name    string
		env     string // valor de CCP_LANG ("" = unset)
		cfg     string
		wantL   Lang
		wantSrc Source
	}{
		{"env gana sobre cfg", "en", "es", En, SourceEnv},
		{"cfg cuando no hay env", "", "es", Es, SourceConfig},
		{"default cuando nada", "", "", En, SourceDefault},
		{"env invalido cae a default", "fr", "", En, SourceDefault},
		{"cfg invalido cae a default", "", "klingon", En, SourceDefault},
		{"case/space insensitive", "  ES ", "", Es, SourceEnv},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.env == "" {
				t.Setenv("CCP_LANG", "")
			} else {
				t.Setenv("CCP_LANG", c.env)
			}
			gotL, gotSrc := ResolveWithSource(c.cfg)
			if gotL != c.wantL || gotSrc != c.wantSrc {
				t.Fatalf("got (%q,%q) want (%q,%q)", gotL, gotSrc, c.wantL, c.wantSrc)
			}
			if Resolve(c.cfg) != c.wantL {
				t.Fatalf("Resolve=%q want %q", Resolve(c.cfg), c.wantL)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/i18n/ -run TestResolvePrecedence`
Expected: FAIL — build error, package/identifiers no existen.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/core/i18n/i18n.go

// Package i18n provee la capa de traducción de la salida de terminal de ccp.
// Sin deps externas: un catálogo en memoria y resolución de idioma por
// precedencia env → config → en. core no imprime prosa; cli/tui llaman T.
package i18n

import (
	"os"
	"strings"
)

// Lang es un idioma soportado.
type Lang string

const (
	En Lang = "en"
	Es Lang = "es"
)

// Source dice de dónde salió el idioma efectivo (para `ccp lang`).
type Source string

const (
	SourceEnv     Source = "env"
	SourceConfig  Source = "config"
	SourceDefault Source = "default"
)

// Resolve aplica la precedencia CCP_LANG (env) → cfgLang (ccp.yaml) → En.
// Cualquier valor que no normalice a en/es cae a En.
func Resolve(cfgLang string) Lang {
	l, _ := ResolveWithSource(cfgLang)
	return l
}

// ResolveWithSource es como Resolve pero además reporta la fuente.
func ResolveWithSource(cfgLang string) (Lang, Source) {
	if l, ok := normalize(os.Getenv("CCP_LANG")); ok {
		return l, SourceEnv
	}
	if l, ok := normalize(cfgLang); ok {
		return l, SourceConfig
	}
	return En, SourceDefault
}

// normalize trim+lowercasea s y lo acepta solo si es en/es.
func normalize(s string) (Lang, bool) {
	switch Lang(strings.ToLower(strings.TrimSpace(s))) {
	case En:
		return En, true
	case Es:
		return Es, true
	}
	return En, false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/i18n/ -run TestResolvePrecedence`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/i18n/i18n.go internal/core/i18n/i18n_test.go
git commit -m "feat(i18n): Lang + Resolve con precedencia env→config→en"
```

---

### Task 2: Motor de catálogo — T + register + test de completitud

**Files:**
- Create: `internal/core/i18n/catalog.go`
- Test: `internal/core/i18n/catalog_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/core/i18n/catalog_test.go
package i18n

import "testing"

// catálogo de prueba inyectado por el test (no contamina el real).
func TestT(t *testing.T) {
	register(map[string]map[Lang]string{
		"test.hello": {En: "Hello %s", Es: "Hola %s"},
		"test.plain": {En: "Plain", Es: "Liso"},
		"test.only_en": {En: "only-en"}, // falta es a propósito
	})

	if got := T(En, "test.hello", "world"); got != "Hello world" {
		t.Fatalf("T(En)=%q", got)
	}
	if got := T(Es, "test.hello", "mundo"); got != "Hola mundo" {
		t.Fatalf("T(Es)=%q", got)
	}
	if got := T(Es, "test.only_en"); got != "only-en" { // fallback a En
		t.Fatalf("fallback=%q", got)
	}
	if got := T(En, "no.such.key"); got != "no.such.key" { // key cruda
		t.Fatalf("missing key=%q", got)
	}
}

// TestCatalogComplete exige que TODA key tenga en+es. Es la red de seguridad
// contra traducciones huérfanas al migrar literales. La key de prueba
// "test.only_en" se excluye porque solo existe dentro de TestT.
func TestCatalogComplete(t *testing.T) {
	for key, m := range catalog {
		if strings_HasPrefix(key, "test.") {
			continue
		}
		for _, l := range []Lang{En, Es} {
			if _, ok := m[l]; !ok {
				t.Errorf("key %q sin traducción para %q", key, l)
			}
		}
	}
}

func strings_HasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/i18n/ -run 'TestT|TestCatalogComplete'`
Expected: FAIL — `T`, `register`, `catalog` no existen.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/core/i18n/catalog.go
package i18n

import "fmt"

// catalog[key][lang] = plantilla fmt. Lo pueblan los catalog_*.go vía register.
var catalog = map[string]map[Lang]string{}

// register fusiona un sub-catálogo de área en el global. Pánico si una key se
// duplica entre áreas (señala colisión de namespacing en build/test).
func register(sub map[string]map[Lang]string) {
	for k, v := range sub {
		if _, dup := catalog[k]; dup {
			panic("i18n: key duplicada: " + k)
		}
		catalog[k] = v
	}
}

// T traduce key al idioma l y aplica fmt.Sprintf con a.
//   - falta el lang concreto → fallback a En.
//   - falta la key entera → devuelve la key cruda (ruidoso; lo caza el test
//     de completitud y se ve en pantalla en dev).
func T(l Lang, key string, a ...any) string {
	m, ok := catalog[key]
	if !ok {
		return key
	}
	tmpl, ok := m[l]
	if !ok {
		tmpl = m[En]
	}
	if len(a) == 0 {
		return tmpl
	}
	return fmt.Sprintf(tmpl, a...)
}
```

> Nota: el test usa `register` dos veces si TestT corre tras otra suite que ya registró las mismas keys → ajustar TestT para usar keys `test.*` únicas (ya lo hace). El panic de duplicado solo aplica a keys reales.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/i18n/ -run 'TestT|TestCatalogComplete'`
Expected: PASS (catalog real vacío ⇒ completitud trivialmente verde).

- [ ] **Step 5: Commit**

```bash
git add internal/core/i18n/catalog.go internal/core/i18n/catalog_test.go
git commit -m "feat(i18n): motor T + register + test de completitud"
```

---

## Phase 2 — Storage + comando `lang` + completion/golden

### Task 3: Campo `lang` en Config

**Files:**
- Modify: `internal/core/store.go:58-72` (struct `Config`), `:76` (`knownTopKeys`)
- Test: `internal/core/store_test.go` (añadir caso)

- [ ] **Step 1: Write the failing test**

```go
// añadir a internal/core/store_test.go
func TestConfigLangRoundTrip(t *testing.T) {
	home := t.TempDir()
	c := &Config{Version: SchemaVersion, Lang: "es",
		Profiles: map[string]Profile{}, Rules: []Rule{}}
	if err := Save(home, c); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(home)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Lang != "es" {
		t.Fatalf("Lang=%q want es", got.Lang)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestConfigLangRoundTrip`
Expected: FAIL — `Config` no tiene campo `Lang`.

- [ ] **Step 3: Write minimal implementation**

En `internal/core/store.go`, dentro de `type Config struct`, añadir el campo tras `Version`:

```go
	Version  int                `yaml:"version"`
	Lang     string             `yaml:"lang,omitempty"` // en|es ; vacío = en
	Defaults Defaults           `yaml:"defaults"`
```

Y añadir `"lang"` a `knownTopKeys`:

```go
var knownTopKeys = map[string]struct{}{
	"version":  {},
	"lang":     {},
	"defaults": {},
	// ... resto sin cambios
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestConfigLangRoundTrip`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/store.go internal/core/store_test.go
git commit -m "feat(store): campo opcional lang en Config (schema v2, aditivo)"
```

---

### Task 4: Helper `currentLang()` en cli

**Files:**
- Create: `internal/cli/i18n.go`

- [ ] **Step 1: Write the implementation** (sin test propio; lo ejercita Task 5)

```go
// internal/cli/i18n.go
package cli

import (
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// currentLang resuelve el idioma efectivo para la salida del CLI cuando el
// comando no tiene ya un *core.Config a mano (help, version, comando
// desconocido). Best-effort: si no hay home o ccp.yaml aún, Resolve("") cae a
// CCP_LANG o en sin fallar. Los comandos que YA cargaron cfg deben llamar
// i18n.Resolve(cfg.Lang) directo en vez de esto.
func currentLang() i18n.Lang {
	home, err := ccpHome()
	if err != nil {
		return i18n.Resolve("")
	}
	cfg, err := core.Load(home) // no migra: leer lang no justifica disparar migración
	if err != nil {
		return i18n.Resolve("")
	}
	return i18n.Resolve(cfg.Lang)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/cli/`
Expected: sin output (compila).

- [ ] **Step 3: Commit**

```bash
git add internal/cli/i18n.go
git commit -m "feat(cli): helper currentLang() best-effort"
```

---

### Task 5: Comando `ccp lang`

**Files:**
- Create: `internal/cli/lang.go`, `internal/cli/lang_test.go`
- Create/extend: `internal/core/i18n/catalog_cli.go` (keys `lang.*`)
- Modify: `internal/cli/cli.go:91` (añadir `case "lang":`)

- [ ] **Step 1: Write the failing test**

```go
// internal/cli/lang_test.go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdLangSetAndShow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "") // sin override de env

	// set es
	var out, errb bytes.Buffer
	if code := cmdLang([]string{"es"}, &out, &errb); code != 0 {
		t.Fatalf("set code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "español") && !strings.Contains(out.String(), "es") {
		t.Fatalf("set out=%q", out.String())
	}

	// show => es / config
	out.Reset(); errb.Reset()
	if code := cmdLang(nil, &out, &errb); code != 0 {
		t.Fatalf("show code=%d", code)
	}
	if !strings.Contains(out.String(), "es") || !strings.Contains(out.String(), "config") {
		t.Fatalf("show out=%q", out.String())
	}

	// invalido => exit 1
	out.Reset(); errb.Reset()
	if code := cmdLang([]string{"fr"}, &out, &errb); code != 1 {
		t.Fatalf("invalid code=%d want 1", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestCmdLangSetAndShow`
Expected: FAIL — `cmdLang` no existe.

- [ ] **Step 3a: Add catalog keys**

```go
// internal/core/i18n/catalog_cli.go
package i18n

func init() {
	register(map[string]map[Lang]string{
		"lang.current": {
			En: "Language: %s (source: %s)",
			Es: "Idioma: %s (fuente: %s)",
		},
		"lang.set": {
			En: "Language set to %s.",
			Es: "Idioma cambiado a %s.",
		},
		"lang.invalid": {
			En: "Unknown language %q. Use 'en' or 'es'.",
			Es: "Idioma desconocido %q. Usa 'en' o 'es'.",
		},
	})
}
```

- [ ] **Step 3b: Write cmdLang**

```go
// internal/cli/lang.go
package cli

import (
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// cmdLang maneja `ccp lang [en|es]`. Sin arg muestra el idioma efectivo y su
// fuente; con arg valida, persiste en ccp.yaml y confirma.
func cmdLang(args []string, stdout, stderr io.Writer) int {
	home, err := ccpHome()
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	cfg, err := core.Load(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	if len(args) == 0 {
		l, src := i18n.ResolveWithSource(cfg.Lang)
		fmt.Fprintln(stdout, i18n.T(l, "lang.current", string(l), string(src)))
		return 0
	}

	want := args[0]
	if want != string(i18n.En) && want != string(i18n.Es) {
		l := i18n.Resolve(cfg.Lang)
		fmt.Fprintln(stderr, i18n.T(l, "lang.invalid", want))
		return 1
	}
	cfg.Lang = want
	if err := core.Save(home, cfg); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, i18n.T(i18n.Lang(want), "lang.set", want))
	return 0
}
```

- [ ] **Step 3c: Wire dispatch** — en `internal/cli/cli.go`, tras `case "doctor":` (línea ~91-92) añadir:

```go
	case "lang":
		return cmdLang(rest, stdout, stderr)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestCmdLangSetAndShow && go test ./internal/core/i18n/`
Expected: PASS ambos.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/lang.go internal/cli/lang_test.go internal/cli/cli.go internal/core/i18n/catalog_cli.go
git commit -m "feat(cli): comando 'ccp lang' (show/set persistente)"
```

---

### Task 6: `lang` en completion + paridad golden

**Files:**
- Modify: `internal/core/shellinit.go:52` (bash), `:72` (zsh array)
- Modify: `legacy/bin/ccp:620` (bash oracle), `:641` (zsh oracle)
- Modify: `internal/golden/parity_test.go` (`CCP_LANG=es` en `cmd.Env`)
- Regenerate: `testdata/golden/basic/expected/completion-*.out`

- [ ] **Step 1: Add `lang` to the Go completion word-lists**

En `internal/core/shellinit.go`, en las DOS líneas con la lista `top` (bash const línea ~52 y zsh array línea ~72), insertar `lang` tras `resolve`:

```
... completion resolve lang version help use default on off run ...
```

(bash: dentro de `local top="..."`; zsh: dentro de `top=(... resolve lang version ...)`.)

- [ ] **Step 2: Mirror in the bash oracle**

En `legacy/bin/ccp`, líneas 620 (`local top="..."`) y 641 (`top=(...)`), insertar `lang` en la misma posición (tras `resolve`).

- [ ] **Step 3: Inject `CCP_LANG=es` in the parity harness**

En `internal/golden/parity_test.go`, en el bloque `cmd.Env = append(os.Environ(), ...)` (línea ~108), añadir la línea:

```go
		cmd.Env = append(os.Environ(),
			"CCP_HOME="+work,
			"NO_COLOR=1",
			"PWD="+pwd,
			"CCP_LANG=es", // igualar al oráculo bash (prosa en español)
		)
```

- [ ] **Step 4: Regenerate the golden from the oracle**

Run:
```bash
bash testdata/golden/capture.sh
bash testdata/golden/capture.sh --check
```
Expected: el primero reescribe `completion-bash.out`/`completion-zsh.out` con `lang`; el `--check` sale 0 (oráculo reproduce lo commiteado).

- [ ] **Step 5: Run the parity gate**

Run: `go test ./internal/golden/`
Expected: PASS — Go (con `CCP_LANG=es`) == oráculo bash sobre el contrato, ahora con `lang` en completion.

- [ ] **Step 6: Commit**

```bash
git add internal/core/shellinit.go legacy/bin/ccp internal/golden/parity_test.go testdata/golden/basic/expected/
git commit -m "feat(completion): comando lang en word-lists; parity en CCP_LANG=es"
```

---

## Phase 3 — Traducción del CLI

### Task 7: Migrar literales de `internal/cli` al catálogo

**Files:**
- Modify: todos los `internal/cli/*.go` (no `_test.go`) con prosa de cara al usuario.
- Extend: `internal/core/i18n/catalog_cli.go`.
- Modify: `internal/cli/present.go` (`humanType` → keys).

Esta tarea es un **barrido mecánico** guardado por tres redes: `TestCatalogComplete` (toda key en+es), el parity gate (prosa golden en ES exacta), y `go vet`/build. Se hace archivo por archivo.

**Procedimiento por cada literal de cara al usuario:**

1. Inventar una key namespaced `cli.<área>.<concepto>` (p.ej. `cli.profile.added`, `cli.err.unknown_cmd`).
2. Añadir la entrada `{En: "...", Es: "<el literal español actual>"}` a `catalog_cli.go`. **El ES = el texto español actual byte-a-byte.** Traducir el EN.
3. Resolver el `Lang` del comando: si ya hay `cfg` cargada, `lang := i18n.Resolve(cfg.Lang)`; si no, `lang := currentLang()`.
4. Reemplazar el literal por `i18n.T(lang, "key", args...)`, moviendo los `%s`/`%d` de `Fprintf` a la plantilla.

**Constraint de paridad (crítico):** las strings que el golden captura deben calcar el oráculo en ES. Hoy la única prosa golden es el warning de `_env` perfil-faltante (Task 7d). El resto del CLI no está en el golden.

- [ ] **Step 1: Dispatch-level strings (`cli.go`)**

Añadir keys:
```go
// en catalog_cli.go register(...)
"cli.err.shell_only": {
	En: "[error] '%s' only works via the 'ccp' shell function (run 'ccp install' and reload your shell).",
	Es: "[error] '%s' solo funciona vía la función shell 'ccp' (corre 'ccp install' y recarga tu shell).",
},
"cli.err.unknown_cmd": {
	En: "Unknown command: '%s'",
	Es: "Comando desconocido: '%s'",
},
```
En `cli.go`, reemplazar los dos `Fprintf` (líneas ~108 y ~112):
```go
	case "use", "default", "off", "on", "run":
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.err.shell_only", cmd))
		return 1
	default:
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.err.unknown_cmd", cmd))
		return 1
```
Añadir el import `"github.com/JoseAFlores777/ccp/internal/core/i18n"` a `cli.go`.

- [ ] **Step 2: `present.go` humanType → keys**

```go
// catalog_cli.go
"cli.ptype.official": {En: "official", Es: "oficial"},
"cli.ptype.deepseek": {En: "DeepSeek (provider)", Es: "DeepSeek (proveedor)"},
"cli.ptype.default":  {En: "default", Es: "por defecto"},
```
Reescribir `humanType(t string)` para recibir lang:
```go
func humanType(l i18n.Lang, t string) string {
	switch t {
	case "official":
		return i18n.T(l, "cli.ptype.official")
	case "deepseek":
		return i18n.T(l, "cli.ptype.deepseek")
	default:
		return i18n.T(l, "cli.ptype.default")
	}
}
```
Actualizar todos sus call-sites para pasar `lang`. (Grep: `grep -rn humanType internal/cli`.)

- [ ] **Step 3: Sweep the remaining cli files**

Para cada archivo en la lista de abajo, aplicar el procedimiento a cada literal de cara al usuario (mensajes de éxito, errores, encabezados de `status`/`doctor`/`config`/`profile`/`instruct`/`backup`/`key`/`path`/`help`). Lista de archivos (de `grep -rlE '[áéíóúñ¿¡]' internal/cli --include="*.go" | grep -v _test`):

```
cli.go  present.go  status.go  config.go  doctor.go  install.go
instruct.go  help.go  path.go  key.go  profile.go  backup.go  upgrade.go  ...
```

`help.go` es el bloque más grande (todo el texto de ayuda) — keys `cli.help.*` por sección, o una sola key `cli.help.body` con el texto completo en/es si se prefiere un blob (más simple para un bloque grande y estable). **Recomendado: una key por línea/sección lógica** para que el diff sea legible, pero un blob `cli.help.body` es aceptable si el texto es monolítico.

- [ ] **Step 4: `_env`/`_hook` warning (paridad golden)**

El warning de perfil-faltante en el delta de `_env` es prosa DENTRO del contrato golden. Su ES debe calcar `testdata/golden/basic/expected/env-missing.out` exactamente:

```
⚠️  ccp: perfil nope no existe; usando default
```

Localizar dónde se emite (grep: `grep -rn 'no existe; usando default' internal`). Añadir key:
```go
"cli.env.profile_missing": {
	En: "⚠️  ccp: profile %s not found; using default",
	Es: "⚠️  ccp: perfil %s no existe; usando default",
},
```
(Nótese el **doble espacio** tras el emoji ⚠️, igual que el oráculo.) Reemplazar el literal por `i18n.T(i18n.Resolve(cfg.Lang), "cli.env.profile_missing", name)`. Como parity corre con `CCP_LANG=es`, el ES se usa y debe matchear byte-a-byte.

- [ ] **Step 5: Run the guards**

Run:
```bash
gofmt -l internal/cli internal/core/i18n
go vet ./internal/cli/ ./internal/core/i18n/
go test ./internal/core/i18n/ ./internal/cli/ ./internal/golden/
```
Expected: gofmt vacío; vet limpio; tests PASS (completitud verde ⇒ no hay keys huérfanas; parity verde ⇒ prosa golden ES intacta).

- [ ] **Step 6: Manual smoke**

Run:
```bash
go build -o /tmp/ccp ./cmd/ccp
CCP_HOME=$(mktemp -d) /tmp/ccp help | head -3            # inglés (default)
CCP_HOME=$(mktemp -d) CCP_LANG=es /tmp/ccp help | head -3 # español
```
Expected: la primera en inglés, la segunda en español; ninguna muestra keys crudas (`cli.*`).

- [ ] **Step 7: Commit**

```bash
git add internal/cli/ internal/core/i18n/catalog_cli.go
git commit -m "i18n(cli): migrar salida del CLI al catálogo en/es"
```

---

## Phase 4 — Traducción de la TUI

### Task 8: Migrar `internal/tui` + toggle `L`

**Files:**
- Modify: `internal/tui/*.go` (dashboard, forms, status, tui).
- Create: `internal/core/i18n/catalog_tui.go`.
- Test: `internal/tui/lang_test.go` (smoke render en/es).

- [ ] **Step 1: Thread lang into the model**

En `internal/tui/tui.go`, añadir `lang i18n.Lang` al struct `model`, inicializándolo al construir el modelo desde la cfg cargada:
```go
m.lang = i18n.Resolve(m.cfg.Lang)
```
(Localizar el constructor del modelo; grep `func.*model` / donde se carga `m.cfg`.)

- [ ] **Step 2: Add the `L` toggle**

En el `Update` del dashboard (donde se manejan teclas, `internal/tui/dashboard.go`), añadir un caso para `"L"`/`"l"`:
```go
	case "L", "l":
		if m.lang == i18n.Es {
			m.lang = i18n.En
		} else {
			m.lang = i18n.Es
		}
		m.cfg.Lang = string(m.lang)
		_ = core.Save(m.home, m.cfg) // persistir; error no debe romper la TUI
		return m, nil
```
(Confirmar que `m.home`/`m.cfg` existen en el modelo; ya se usan en `dashboard.go`.)

- [ ] **Step 3: Sweep TUI literals**

Aplicar el procedimiento de Task 7 (keys `tui.*`) a labels, hints, títulos de panel, footer de teclas y línea de status en `dashboard.go`, `forms.go`, `status.go`, `tui.go`. Cada `Render("texto")`/string literal de cara al usuario → `i18n.T(m.lang, "tui.key", ...)`. Añadir el binding `L` al footer de teclas con su label traducido:
```go
"tui.footer.lang": {En: "L lang", Es: "L idioma"},
```

- [ ] **Step 4: Smoke test**

```go
// internal/tui/lang_test.go
package tui

import (
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

func TestDashboardRendersBothLangs(t *testing.T) {
	for _, l := range []i18n.Lang{i18n.En, i18n.Es} {
		m := newTestModel(t) // helper que monta un model mínimo con cfg vacía
		m.lang = l
		out := m.viewDashboard()
		if strings.TrimSpace(out) == "" {
			t.Fatalf("lang %q: render vacío", l)
		}
		if strings.Contains(out, "tui.") { // key cruda filtrada
			t.Fatalf("lang %q: key sin traducir en render:\n%s", l, out)
		}
	}
}
```
> Si no existe un helper `newTestModel`, créalo en el test montando el `model` con `home := t.TempDir()` y `cfg` vacío (espejo de cómo `tui.go` construye el modelo). Mantenerlo mínimo: solo lo que `viewDashboard` necesita.

- [ ] **Step 5: Run guards**

Run:
```bash
gofmt -l internal/tui internal/core/i18n
go vet ./internal/tui/ ./internal/core/i18n/
go test ./internal/tui/ ./internal/core/i18n/
```
Expected: limpio + PASS.

- [ ] **Step 6: Manual smoke**

Run: `go run ./cmd/ccp` en una TTY → pulsar `L` alterna idioma en vivo; reabrir confirma persistencia.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/ internal/core/i18n/catalog_tui.go
git commit -m "i18n(tui): migrar la TUI al catálogo en/es + toggle L"
```

---

## Phase 5 — Errores de core + auditoría de tests

### Task 9: Mapear errores de core de cara al usuario; auditar tests con prosa

**Files:**
- Create: `internal/core/i18n/catalog_core.go`.
- Modify: call-sites en cli/tui que formatean errores de core.
- Audit: `internal/**/*_test.go` que asertan prosa española literal.

- [ ] **Step 1: Inventory user-facing core errors**

Run: `grep -rnE 'errors\.New\("|fmt\.Errorf\("' internal/core --include='*.go' | grep -E '[áéíóúñ¿¡]'`
Para cada error que **llega al usuario** (se imprime en cli/tui), decidir: mantener el `error` en core como está (es un valor/log técnico) y traducir SOLO en el punto donde cli/tui lo presentan, vía una key `core.err.*`. core sigue sin imprimir prosa traducida; cli/tui mapean.

- [ ] **Step 2: Add keys + map at presentation**

Para los errores que se muestran textualmente, añadir keys `core.err.<x>` a `catalog_core.go` y, en el call-site cli/tui, presentar `i18n.T(lang, "core.err.x", ...)` en vez de `err.Error()`. (Donde el error es puramente técnico/diagnóstico, dejarlo: no toda cadena de error es "de cara al usuario".)

- [ ] **Step 3: Audit tests asserting Spanish prose**

Run: `grep -rnE '[áéíóúñ¿¡]' internal --include='*_test.go'`
Para cada test que compara contra prosa española literal (p.ej. `version_test.go`, asserts de help/status), una de:
- fijar `t.Setenv("CCP_LANG","es")` y comparar contra el ES (que = el literal viejo), o
- comparar contra `i18n.T(i18n.Es, "key", ...)`.
Elegir lo que deje el test legible. No cambiar asserts de surfaces máquina (resolve/json/env), que no se tradujeron.

- [ ] **Step 4: Run full suite**

Run: `go test ./...`
Expected: PASS (incluye parity con `CCP_LANG=es`).

- [ ] **Step 5: Commit**

```bash
git add internal/core/i18n/catalog_core.go internal/cli/ internal/tui/ internal/**/*_test.go
git commit -m "i18n(core): presentar errores de cara al usuario vía catálogo; auditar tests"
```

---

## Phase 6 — README bilingüe

### Task 10: README.md/.html EN default + README.es.md/.html

**Files:**
- Modify: `README.md` (→ EN + selector), `README.html` (→ EN + selector).
- Create: `README.es.md` (ES + selector), `README.es.html` (ES + selector).

- [ ] **Step 1: Snapshot the current Spanish README as the ES version**

```bash
git mv README.md README.es.md
cp README.html README.es.html   # README.html original es español
```
(Si `git mv` complica el diff de traducción, alternativamente `cp README.md README.es.md` y luego sobrescribir `README.md` con la traducción.)

- [ ] **Step 2: Add the language selector**

Arriba del todo de `README.es.md` (tras el título H1):
```markdown
[English](README.md) · **Español**
```
En `README.md` (la versión EN, Step 3):
```markdown
**English** · [Español](README.es.md)
```
En los HTML, un par de links equivalente arriba del `<header class="hero">`:
```html
<p class="eyebrow"><strong>English</strong> · <a href="README.es.html">Español</a></p>
```
(y el inverso en `README.es.html`).

- [ ] **Step 3: Translate README.md to English**

Traducir el contenido de `README.es.md` a inglés en `README.md`, conservando estructura, screenshots, bloques de código y comandos (los comandos NO se traducen). Actualizar cualquier `ccp lang` / mención de idioma para documentar la nueva feature (sección breve "Language / Idioma": `ccp lang en|es`, `CCP_LANG`, tecla `L` en la TUI).

- [ ] **Step 4: Translate README.html to English**

Traducir el contenido de `README.es.html` a inglés en `README.html`, conservando paleta/tipografía/layout Anthropic. Mismo añadido de la sección de idioma.

- [ ] **Step 5: Verify links + render**

Run:
```bash
grep -n "README.es.md\|README.md" README.md README.es.md
open README.html   # revisar selector + contenido EN
open README.es.html
```
Expected: selectores cruzados correctos; HTML renderiza en su idioma.

- [ ] **Step 6: Commit**

```bash
git add README.md README.es.md README.html README.es.html
git commit -m "docs: README/HTML bilingües (EN default) + selector de idioma"
```

---

## Phase 7 — Verificación final

### Task 11: Gate completo + criterios de aceptación

**Files:** ninguno (verificación).

- [ ] **Step 1: Static gates**

Run:
```bash
gofmt -l internal cmd
go vet ./...
golangci-lint run
```
Expected: gofmt vacío; vet/lint limpios.

- [ ] **Step 2: Full test + golden**

Run:
```bash
go test ./...
bash legacy/tests/run.sh
bash testdata/golden/capture.sh --check
```
Expected: todo PASS — incluye `TestCatalogComplete`, parity (`CCP_LANG=es`), oráculo bash.

- [ ] **Step 3: Acceptance criteria (manual)**

Run:
```bash
go build -o /tmp/ccp ./cmd/ccp
H=$(mktemp -d)
CCP_HOME=$H /tmp/ccp help | head -1                 # EN (default)
CCP_HOME=$H CCP_LANG=es /tmp/ccp help | head -1      # ES (env override)
CCP_HOME=$H /tmp/ccp lang es                         # persiste
CCP_HOME=$H /tmp/ccp lang                            # => es / config
CCP_HOME=$H /tmp/ccp help | head -1                  # ahora ES (config)
CCP_HOME=$H CCP_LANG=en /tmp/ccp lang                # => en / env
```
Expected: cada línea cumple el criterio anotado; ninguna pantalla muestra keys `cli.*`/`tui.*`.

- [ ] **Step 4: Completeness self-check**

Run: `grep -rnoE '"(cli|tui|core|lang)\.[a-z0-9_.]+"' internal --include='*.go' | grep i18n.T` (spot-check) — y confiar en `TestCatalogComplete` como red dura.

- [ ] **Step 5: Final commit (si quedó algo suelto)**

```bash
git status
# si hay cambios de verificación (p.ej. golden re-tocado), stagear + commit
git commit -m "test: verificación final i18n (gates verdes)"
```

---

## Self-Review (autor del plan)

- **Cobertura del spec:** §03 Resolución → Task 1; §04 i18n → Tasks 1-2; §05 storage → Task 3; §06 comando+TUI → Tasks 5, 8; §07 alcance → Tasks 7-9 (IN) con OUT respetado (no se tocan resolve/_env-exports/json/completion-words salvo `lang`, ni shell-init prose, ni install.sh); §08 paridad → Task 6 + constraint en 7d; §09 README → Task 10; §10 testing → Tasks 2, 8, 11; §13 distribución → documental, sin código (no genera tasks). Cubierto.
- **Placeholders:** ninguno "TBD/TODO"; los barridos (7, 8, 9) dan procedimiento exacto + keys de ejemplo + redes de test que fuerzan completitud (no son "implement later", son sweeps deterministas guardados).
- **Consistencia de tipos:** `Lang`, `Source`, `Resolve`/`ResolveWithSource`, `T(l,key,a...)`, `register`, `currentLang()`, `cmdLang`, `Config.Lang` — usados con la misma firma en todas las tasks.

---

*ccp plan · terminal bilingüe + README EN/ES · 2026-06-02*
