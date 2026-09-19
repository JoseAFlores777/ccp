# Fase 0: mediciones de Desktop y defectos B1-B5 — plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** que nada de lo que viene después (la proyección de MCP, el editor, la
nube) se construya sobre suposiciones sobre Claude Desktop, y arreglar los cinco
defectos que el análisis del 2026-09-18 encontró de camino.

**Architecture:** son dos bloques independientes.
- **Task 1: seis mediciones manuales** (M1-M6), con procedimiento reproducible y
  un perfil desechable. El resultado va a un ADR y a la tabla §3 del spec.
  Necesita al usuario: iniciar sesión en Desktop y en Claude Code no se
  automatiza.
- **Tasks 2-6: cinco arreglos con TDD** en `core`, `cli`, `tui` y `gui`. Ninguno
  depende de las mediciones ni de los planes de snapshots y nube; pueden ir en
  paralelo con la Task 1.

**Tech Stack:** Go 1.24 · bubbletea (TUI) · React/TS (GUI) · bash (oráculo `legacy/`).

**Spec:** `docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md`
§1 (defectos B1-B5), §3 (qué llega a cada destino) y §4 (Fase 0).

## Global Constraints

- **Commits: nunca autónomos.** «Commit» = dejar el árbol listo, mostrar el
  mensaje propuesto y esperar autorización explícita del usuario en ese turno.
  Sin trailers `Co-Authored-By` ni atribución a herramientas.
- **Gates de CI, todos verdes:** `gofmt -l internal cmd` vacío · `go vet ./...` ·
  `go test ./...` · `go run
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --timeout 5m` ·
  `bash legacy/tests/run.sh` · `bash testdata/golden/capture.sh --check`. En
  `gui/`: `npm run typecheck`.
- **Bilingüe:** textos nuevos de la CLI en su catálogo con `En` y `Es`; los de la
  TUI en `catalog_tui.go`; los de la GUI en español dentro de `t('…')` y en
  inglés en `gui/src/lib/i18n_en.ts`.
- **Tests nunca tocan el estado real:** `CCP_HOME`, `CCP_CLAUDE_SRC` y `HOME` en
  temporales.
- **Las mediciones no tocan los perfiles reales del usuario.** Todo se hace con
  el perfil desechable `probe-desktop`, en `/tmp/ccp-probe` y con marcadores que
  empiezan por `ccp-probe`. Se limpia al final (Task 1, Step 9).
- **El contrato golden no se toca.** Si un texto que también está en el oráculo
  bash cambia (B1), se cambia en ambos sitios.

---

### Task 1: mediciones M1-M6 (con el usuario)

Cada medición deja por escrito:
- la versión de Claude.app (`defaults read /Applications/Claude.app/Contents/Info.plist CFBundleShortVersionString`);
- la de Claude Code (`claude --version`);
- el método;
- lo observado **literalmente** (qué dijo, qué archivo apareció, cuántos procesos).

Una celda del spec pasa de «?» a ✓ o ✗ solo con una observación directa. Sin
ella, se queda en «?» con una nota de por qué no se pudo medir.

**Files:**
- Create: `docs/adr/0016-what-desktop-reads-from-a-profile.md`
- Modify: `docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md` (§3, §4, D7)

- [ ] **Step 1: El perfil desechable**

```bash
ccp profile add probe-desktop            # official
mkdir -p /tmp/ccp-probe
ccp desktop prepare probe-desktop        # espejo: cc-home/{agents,commands,skills,plugins} pasan a directorios reales
ccp desktop open probe-desktop           # ventana nueva: el usuario inicia sesión con una cuenta suya
```

`HOME_CCP=~/.config/ccp/profiles/probe-desktop` es la raíz de los pasos
siguientes.

- [ ] **Step 2: Marcadores en el cc-home del perfil (con la ventana cerrada)**

```bash
P=~/.config/ccp/profiles/probe-desktop
# Instrucción (overlay → cc-home/CLAUDE.md por @import)
printf '\n- Termina cada respuesta con la palabra CCP-PROBE-CLAUDEMD.\n' >> "$P/overlay/CLAUDE.md"
# env, hook SessionStart y permiso (overlay → cc-home/settings.json)
python3 - "$P/overlay/settings.overlay.json" <<'EOF'
import json, sys
p = sys.argv[1]
d = json.load(open(p)) if open(p).read().strip() else {}
d.setdefault("env", {})["CCP_PROBE_ENV"] = "CCP-PROBE-ENV"
d.setdefault("hooks", {})["SessionStart"] = [{"hooks": [{"type": "command", "command": "touch /tmp/ccp-probe/hook-ran"}]}]
d.setdefault("permissions", {}).setdefault("allow", []).append("Bash(echo CCP-PROBE*)")
json.dump(d, open(p, "w"), indent=2)
EOF
ccp profile sync probe-desktop
# skill y agente del perfil (directorios reales tras el espejo)
mkdir -p "$P/cc-home/skills/ccp-probe"
printf -- '---\nname: ccp-probe\ndescription: Skill de prueba de ccp. Úsala si te preguntan por CCP-PROBE-SKILL.\n---\nResponde CCP-PROBE-SKILL.\n' > "$P/cc-home/skills/ccp-probe/SKILL.md"
printf -- '---\nname: ccp-probe-agent\ndescription: Subagente de prueba de ccp.\n---\nResponde CCP-PROBE-AGENT.\n' > "$P/cc-home/agents/ccp-probe-agent.md"
# MCP de scope user en el .claude.json del perfil
python3 - "$P/cc-home/.claude.json" <<'EOF'
import json, os, sys
p = sys.argv[1]
d = json.load(open(p)) if os.path.exists(p) else {}
d.setdefault("mcpServers", {})["ccp-probe-cli"] = {"command": "/bin/sleep", "args": ["3600"]}
json.dump(d, open(p, "w"), indent=2)
EOF
```

`/bin/sleep` no es un servidor MCP: el cliente fallará al negociar. Da igual,
porque se trata de ver si **lo intenta** (aparece en `/mcp` y hay un proceso
`sleep 3600`).

- [ ] **Step 3: M1 — ¿qué lee la pestaña Code del cc-home?**

Abre la ventana (`ccp desktop open probe-desktop`) y, en la pestaña Code, una
sesión nueva en `/tmp/ccp-probe`. Pídele:

> Dime qué skills, subagentes y servidores MCP tienes disponibles, y ejecuta `echo $CCP_PROBE_ENV`.

Anota, por fila:

| Elemento | Cómo se ve |
|---|---|
| CLAUDE.md (con `@import` fuera del root) | la respuesta termina en `CCP-PROBE-CLAUDEMD` |
| env de settings.json | `echo` imprime `CCP-PROBE-ENV` |
| hook | existe `/tmp/ccp-probe/hook-ran` |
| permiso | el `echo` no pide confirmación |
| skill | lista `ccp-probe` |
| agente | lista `ccp-probe-agent` |
| MCP de `.claude.json` | `/mcp` (si la pestaña lo ofrece) o la respuesta lista `ccp-probe-cli`; `pgrep -fl 'sleep 3600'` |
| plugins (`enabledPlugins` del global) | lista algún plugin del usuario |

- [ ] **Step 4: M2 — ¿la pestaña Code hereda los MCP del chat? ¿Y si se repite el nombre?**

Con la ventana cerrada:

```bash
D=~/.config/ccp/profiles/probe-desktop/desktop/claude_desktop_config.json
python3 - "$D" "$P/cc-home/.claude.json" <<'EOF'
import json, os, sys
dcfg, cj = sys.argv[1], sys.argv[2]
d = json.load(open(dcfg)) if os.path.exists(dcfg) else {}
d.setdefault("mcpServers", {})["ccp-probe-desktop"] = {"command": "/bin/sleep", "args": ["3601"]}
d["mcpServers"]["ccp-probe-both"] = {"command": "/bin/sleep", "args": ["3602"]}
json.dump(d, open(dcfg, "w"), indent=2)
c = json.load(open(cj))
c["mcpServers"]["ccp-probe-both"] = {"command": "/bin/sleep", "args": ["3602"]}
json.dump(c, open(cj, "w"), indent=2)
EOF
```

Abre la ventana y una sesión Code nueva. Anota:
- si aparece `ccp-probe-desktop`;
- cuántas veces aparece `ccp-probe-both`;
- `pgrep -fl 'sleep 360[12]'`: cuántos procesos de cada uno.

- [ ] **Step 5: M3 — el chat de Desktop y las entradas remotas; ¿se relee en caliente?**

Con la ventana **abierta**, añade al `claude_desktop_config.json` del paso
anterior:
- `"ccp-probe-http": {"type": "http", "url": "https://example.com/mcp"}`;
- `"ccp-probe-hot": {"command": "/bin/sleep", "args": ["3603"]}`.

Mira en Ajustes → Desarrollador (o donde la versión liste los conectores
locales) si aparecen **sin reiniciar**. Reinicia y vuelve a mirar. Anota:
- si acepta la entrada `http` o la marca como error;
- si hizo falta reiniciar.

- [ ] **Step 6: M4 — ¿con qué nombre guarda Claude Code las credenciales?**

```bash
before=$(security dump-keychain 2>/dev/null | grep -o '"svce"<blob>="Claude Code[^"]*"' | sort -u)
mkdir -p /tmp/ccp-probe-cfg
CLAUDE_CONFIG_DIR=/tmp/ccp-probe-cfg claude     # el usuario hace /login y sale con /exit
after=$(security dump-keychain 2>/dev/null | grep -o '"svce"<blob>="Claude Code[^"]*"' | sort -u)
diff <(echo "$before") <(echo "$after")
```

`dump-keychain` sin `-d` lista atributos, **nunca** secretos. Anota:
- el nombre del servicio nuevo;
- si depende de la ruta. Para eso repite con `/tmp/ccp-probe-cfg2` y compara
  los dos nombres.

- [ ] **Step 7: M5 — ¿Desktop reescribe `claude_desktop_config.json` mientras corre?**

```bash
stat -f '%m %z' "$D"          # antes
# en la ventana abierta: cambia una preferencia cualquiera (tema, notificaciones…)
stat -f '%m %z' "$D"          # después
# cierra la ventana
stat -f '%m %z' "$D"          # al cerrar
```

Anota si el mtime cambia y **si las entradas añadidas a mano siguen ahí**
(`python3 -c "import json;print(list(json.load(open('$D'))['mcpServers']))"`).

- [ ] **Step 8: M6 — ¿Claude Code conserva un `mcpServers` escrito desde fuera mientras corre?**

```bash
CLAUDE_CONFIG_DIR=$P/cc-home claude    # terminal 1: sesión interactiva abierta
```

```bash
# terminal 2, con la sesión de la 1 abierta:
python3 - "$P/cc-home/.claude.json" <<'EOF'
import json, sys
p = sys.argv[1]
d = json.load(open(p))
d["mcpServers"]["ccp-probe-external"] = {"command": "/bin/sleep", "args": ["3604"]}
json.dump(d, open(p, "w"), indent=2)
EOF
```

En la terminal 1, manda un mensaje, cambia algo con `/config` (para forzar que
guarde su estado) y sal con `/exit`. Después:

```bash
python3 -c "import json;print('ccp-probe-external' in json.load(open('$P/cc-home/.claude.json'))['mcpServers'])"
```

`True` significa que se puede proyectar a `.claude.json` (técnica A del spec).
`False` significa que Claude Code lo pisa y hay que ir al plugin local
(técnica B).

- [ ] **Step 9: Limpieza**

```bash
ccp desktop rm probe-desktop --yes
ccp profile rm probe-desktop
rm -rf /tmp/ccp-probe /tmp/ccp-probe-cfg /tmp/ccp-probe-cfg2
pkill -f 'sleep 36(00|01|02|03|04)' || true
```

Las entradas de Keychain que creó M4 las borra **el usuario**, desde Acceso a
Llaveros; el agente no toca credenciales.

- [ ] **Step 10: El ADR y el spec**

`docs/adr/0016-what-desktop-reads-from-a-profile.md`:
- con el formato de 0008/0009: **Contexto**, **Mediciones** (una tabla por M, con
  versión, método y resultado literal), **Decisión** y **Consecuencias**;
- en «Decisión», lo que se sigue de M1-M6 para la técnica de proyección del MCP
  (D7) y para el destino chat (§6.1);
- enlaza y enmienda 0008/0009 donde contradigan algo.

En el spec:
- §3: cada «?» pasa a ✓ o ✗ con referencia a la M;
- §4: la tabla gana una columna «Resultado»;
- D7: decidida.

- [ ] **Step 11: Commit**

Mensaje propuesto: `docs: mediciones de Desktop M1-M6 (ADR 0016) y spec actualizado`

---

### Task 2: B1 — la pista del MCP «global» deja de mentir

`ccp instruct add profile mcp` falla con el código 5 y sugiere `global` para
«todos los perfiles». No es verdad:
- el MCP global va a `~/.claude.json`, que solo lee `default`;
- cada perfil `official` lee **su** `cc-home/.claude.json`.

El arreglo de fondo, el MCP por capas y proyectado, es el subproyecto B. Aquí
solo se corrige lo que se le dice al usuario, en Go **y** en el oráculo bash, para
que no diverjan.

**Files:**
- Modify: `internal/core/instruct.go` (texto de `Hint` del código 5)
- Modify: `internal/core/instruct_test.go` (o el test de `InstructDest` que exista)
- Modify: `legacy/bin/ccp` (línea del `info` del caso 5, ~358)

- [ ] **Step 1: El test**

```go
func TestInstructDestProfileMCPHintIsHonest(t *testing.T) {
	_, err := InstructDest("profile", "mcp", t.TempDir(), "work", t.TempDir(), "")
	var de *DestError
	if !errors.As(err, &de) || de.Code != 5 {
		t.Fatalf("err = %v, quiero DestError código 5", err)
	}
	if strings.Contains(de.Hint, "todos los perfiles") {
		t.Fatalf("la pista vuelve a prometer que global llega a todos los perfiles: %q", de.Hint)
	}
	if !strings.Contains(de.Hint, "default") || !strings.Contains(de.Hint, "project") {
		t.Fatalf("la pista debe decir a quién llega global y ofrecer project: %q", de.Hint)
	}
}
```

Run: `go test ./internal/core/ -run TestInstructDestProfileMCPHintIsHonest`
Expected: FAIL.

- [ ] **Step 2: El arreglo**

`internal/core/instruct.go`, caso `mcp` de scope `profile`:

```go
		case "mcp":
			return "", &DestError{
				Code: 5,
				Msg:  "scope 'profile': MCP por-perfil no está soportado todavía.",
				// «global» va a ~/.claude.json, que solo lee el perfil default: cada
				// perfil official lee su propio cc-home/.claude.json. Hasta que llegue
				// el MCP por perfil (spec 2026-09-18 §6.1), lo único que alcanza a
				// todos es el scope project.
				Hint: "Usa 'project' (el .mcp.json de este repo; lo ven todos los perfiles). 'global' solo llega a default.",
			}
```

`legacy/bin/ccp`, la línea del caso 5:

```bash
       info "Usa 'project' (el .mcp.json de este repo; lo ven todos los perfiles). 'global' solo llega a default." >&2; return 5 ;;
```

- [ ] **Step 3: Comprobar**

Run: `go test ./internal/core/ ./internal/cli/ && bash legacy/tests/run.sh && bash testdata/golden/capture.sh --check`
Expected: PASS. Ningún test afirmaba el texto viejo, y el golden no incluye
`instruct`.

- [ ] **Step 4: Commit**

Mensaje propuesto: `fix(instruct): la pista del MCP no promete que global llega a todos los perfiles (B1)`

---

### Task 3: B2 — `ccp backup restore` regenera los perfiles

`applyProfile` siembra el cc-home, pero nunca llama a `CfgRegenerate`. Tras
restaurar, `cc-home/settings.json` y `CLAUDE.md` no reflejan el overlay
restaurado hasta que alguien ejecuta `ccp profile sync`.

**Files:**
- Modify: `internal/core/backup.go` (`RestoreReport.Regenerated`, regeneración tras `Save`)
- Modify: `internal/core/backup_test.go`
- Modify: `internal/cli/cli.go` (muestra los regenerados), `internal/cli/serve_ops.go` (`"regenerated"` en la respuesta)
- Modify: `internal/core/i18n/catalog_cli.go` (clave `cli.backup.restore_regenerated`)

- [ ] **Step 1: El test (añadir a `backup_test.go`)**

```go
// Tras restaurar, el cc-home ya refleja el overlay restaurado, sin esperar a un
// `ccp profile sync` (spec B2).
func TestBackupRestoreRegeneratesProfiles(t *testing.T) {
	home := setupHome(t)
	archive := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := BackupExport(home, archive, false, fixedTime); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	rep, err := BackupRestore(dst, archive, RestoreOpts{SnapshotDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Regenerated) == 0 {
		t.Fatalf("Regenerated vacío: %+v", rep)
	}
	for _, name := range rep.Regenerated {
		cch := ccHomePath(dst, name)
		md, err := os.ReadFile(filepath.Join(cch, "CLAUDE.md"))
		if err != nil || !bytes.Contains(md, []byte("@")) {
			t.Errorf("%s: cc-home/CLAUDE.md no generado: %q %v", name, md, err)
		}
		if _, err := os.Stat(filepath.Join(cch, "settings.json")); err != nil {
			t.Errorf("%s: cc-home/settings.json no generado: %v", name, err)
		}
	}
}
```

Run: `go test ./internal/core/ -run TestBackupRestoreRegeneratesProfiles`
Expected: FAIL (`rep.Regenerated undefined`).

- [ ] **Step 2: El arreglo — `internal/core/backup.go`**

Añade a `RestoreReport`:

```go
	Regenerated []string // perfiles cuyo cc-home se regeneró tras restaurar
```

Y en `BackupRestore`, sustituye el final (`if err := Save(home, cur)…return rep, nil`) por:

```go
	if err := Save(home, cur); err != nil {
		return rep, err
	}
	// Regenera lo que sale del overlay restaurado (spec B2). Sin esto,
	// cc-home/settings.json y CLAUDE.md seguían viejos hasta el próximo sync.
	src, err := claudeSrc()
	if err != nil {
		return rep, err
	}
	for _, name := range append(append([]string{}, rep.Created...), rep.Overwritten...) {
		if name == "default" {
			continue
		}
		if err := CfgRegenerate(home, name, src); err != nil {
			return rep, fmt.Errorf("se restauró %q pero no se pudo regenerar su cc-home: %w", name, err)
		}
		rep.Regenerated = append(rep.Regenerated, name)
	}
	return rep, nil
```

- [ ] **Step 3: Enseñarlo**

`internal/core/i18n/catalog_cli.go`:

```go
	"cli.backup.restore_regenerated": {
		En: "Profiles regenerated (cc-home): %s",
		Es: "Perfiles regenerados (cc-home): %s",
	},
```

`internal/cli/cli.go`, en la rama `restore`, tras el bloque de `RulesAdded`:

```go
		if len(rep.Regenerated) > 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.backup.restore_regenerated", strings.Join(rep.Regenerated, ", ")))
		}
```

`internal/cli/serve_ops.go`, en el `map` que devuelve `srvBackupRestore`, añade
`"regenerated": nz(rep.Regenerated)`.

- [ ] **Step 4: Comprobar**

Run: `go test ./internal/core/ ./internal/cli/ ./internal/core/i18n/`
Expected: PASS.

- [ ] **Step 5: Commit**

Mensaje propuesto: `fix(backup): restaurar regenera el cc-home de los perfiles restaurados (B2)`

---

### Task 4: B3 — «tiene login» deja de significar «existe `.claude.json`»

Claude Code crea `.claude.json` en su primer arranque, **antes** de `/login`, así
que su existencia no dice nada. Lo que sí lo dice es la cuenta registrada en el
archivo:
- `oauthAccount`, en una suscripción;
- `primaryApiKey`, con una clave de consola.

**Files:**
- Modify: `internal/core/doctor.go` (`HasLogin`, `DefaultHasLogin`, `claudeJSONHasLogin`; `Doctor` usa `HasLogin`)
- Modify: `internal/core/doctor_test.go`
- Modify: `internal/cli/serve_methods.go` (`profileLoggedIn` para `default`)

- [ ] **Step 1: Los tests (`doctor_test.go`)**

En el fixture existente de `Doctor`, sustituye el `{}` del perfil «casa» por una
cuenta, porque `{}` pasa a significar «sin login»:

```go
	if err := os.WriteFile(claudeJSON, []byte(`{"oauthAccount":{"emailAddress":"casa@example.com"}}`), 0o644); err != nil {
```

Y añade:

```go
func TestHasLoginNeedsAnAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	for body, want := range map[string]bool{
		"":                                         false, // sin archivo
		`{}`:                                       false, // primer arranque, antes de /login
		`{"numStartups":3,"machineID":"x"}`:        false,
		`{"oauthAccount":null}`:                    false,
		`{"oauthAccount":{}}`:                      false,
		`{"oauthAccount":{"emailAddress":"a@b"}}`:  true,
		`{"primaryApiKey":"sk-ant-api03-xxx"}`:     true,
		`no es json`:                               false,
	} {
		os.Remove(cj)
		if body != "" {
			os.WriteFile(cj, []byte(body), 0o600)
		}
		if got := HasLogin(home, "work"); got != want {
			t.Errorf("HasLogin con %q = %v, quiero %v", body, got, want)
		}
	}
}
```

Run: `go test ./internal/core/ -run 'HasLogin|Doctor'`
Expected: FAIL en `TestHasLoginNeedsAnAccount` (`{}` da `true`).

- [ ] **Step 2: El arreglo — `internal/core/doctor.go`**

```go
// HasLogin dice si un perfil oficial tiene sesión iniciada: su .claude.json
// registra una cuenta (oauthAccount) o una clave de consola (primaryApiKey).
// Que el archivo exista no basta: Claude Code lo crea en su primer arranque,
// antes del /login (spec B3). Read-only.
func HasLogin(home, name string) bool {
	return claudeJSONHasLogin(filepath.Join(ccHomePath(home, name), ".claude.json"))
}

// DefaultHasLogin es HasLogin para la cuenta de siempre: el .claude.json junto a
// la fuente global (src + ".json", igual que InstructDest).
func DefaultHasLogin(src string) bool { return claudeJSONHasLogin(src + ".json") }

func claudeJSONHasLogin(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var v struct {
		OAuthAccount  json.RawMessage `json:"oauthAccount"`
		PrimaryAPIKey string          `json:"primaryApiKey"`
	}
	if json.Unmarshal(data, &v) != nil {
		return false
	}
	acct := strings.TrimSpace(string(v.OAuthAccount))
	return (acct != "" && acct != "null" && acct != "{}") || v.PrimaryAPIKey != ""
}
```

Añade `"encoding/json"`, `"os"` y `"strings"` a los imports. En `Doctor`,
sustituye `if fileExists(claudeJSON) {` por `if HasLogin(home, name) {` y borra
la variable `claudeJSON` si queda sin uso.

`internal/cli/serve_methods.go`, `profileLoggedIn`:

```go
func profileLoggedIn(home, name string) bool {
	if name == "default" {
		src, err := claudeSrc()
		if err != nil {
			return false
		}
		return core.DefaultHasLogin(src)
	}
	return core.HasLogin(home, name)
}
```

Si `os` o `filepath` quedan sin uso en ese archivo, quítalos de sus imports.

- [ ] **Step 3: Comprobar**

Run: `go test ./internal/... && go vet ./...`
Expected: PASS. Si algún test de la TUI o de `serve` simulaba el login con `{}`,
cámbialo por una cuenta, como el fixture del Step 1.

- [ ] **Step 4: Commit**

Mensaje propuesto: `fix(doctor): el login se decide por la cuenta registrada, no por que exista .claude.json (B3)`

---

### Task 5: B4 — sembrar también `output-styles/`, `hooks/` y `keybindings.json`

`seedCCHome` solo enlaza `plugins/`, `commands/`, `agents/` y `skills/`. Por eso
un perfil no hereda los estilos de salida globales, ni los scripts de hooks que
`settings.json` invoca por ruta relativa, ni los atajos de teclado. Se añaden
los tres:
- los dos directorios, como symlink de directorio (la forma del contrato) y
  también en el espejo de Desktop;
- `keybindings.json`, como symlink de archivo, que es hoja y Desktop lo admite.

**Files:**
- Modify: `internal/core/profile.go` (`seedCCHome`)
- Modify: `internal/core/desktop.go` (`desktopMirrorItems` y su comentario)
- Modify: `internal/core/profile_test.go` (o el test de `seedCCHome` que exista), `internal/core/desktop_test.go`
- Modify: `legacy/lib/profiles.sh` (la lista de `_seed_cc_home`, para que el oráculo no diverja) y, si hay una aserción de la lista, `legacy/tests/run.sh`

- [ ] **Step 1: El test**

```go
func TestSeedCCHomeLinksStylesHooksAndKeybindings(t *testing.T) {
	home, src := t.TempDir(), t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	for _, d := range []string{"output-styles", "hooks", "commands"} {
		os.MkdirAll(filepath.Join(src, d), 0o755)
	}
	os.WriteFile(filepath.Join(src, "keybindings.json"), []byte(`[]`), 0o644)
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	cch := ccHomePath(home, "work")
	for _, item := range []string{"output-styles", "hooks", "keybindings.json", "commands"} {
		fi, err := os.Lstat(filepath.Join(cch, item))
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s: no es un symlink a la fuente global (%v)", item, err)
		}
	}
}
```

Y en el test del espejo de Desktop que ya existe, añade `output-styles/estilo.md`
a la fuente y afirma que, tras `MirrorForDesktop`, `cc-home/output-styles` es un
**directorio real** con `estilo.md` como symlink de archivo.

Run: `go test ./internal/core/ -run 'SeedCCHome|Mirror'`
Expected: FAIL.

- [ ] **Step 2: El arreglo**

`internal/core/profile.go`, en `seedCCHome`:

```go
	// Solo crea symlink si la entrada existe en src y aún no existe en cch.
	// output-styles/ y hooks/ (los scripts que settings.json invoca por ruta) y
	// keybindings.json se añadieron en la Fase 0 (spec B4): sin ellos un perfil
	// no heredaba estilos ni atajos, y un hook global con ruta relativa fallaba.
	for _, item := range []string{"plugins", "commands", "agents", "skills", "output-styles", "hooks", "keybindings.json"} {
```

`internal/core/desktop.go`:

```go
// desktopMirrorItems son los DIRECTORIOS que seedCCHome symlinkea y que por eso
// hay que convertir antes de que Desktop escriba en el config root.
// keybindings.json no está: es un archivo, una hoja, y Desktop admite symlinks
// en las hojas. Si mañana seedCCHome siembra otro directorio, este es el otro
// sitio que hay que tocar.
var desktopMirrorItems = []string{"plugins", "commands", "agents", "skills", "output-styles", "hooks"}
```

`legacy/lib/profiles.sh`: añade `output-styles hooks keybindings.json` a la lista
de `_seed_cc_home`. Busca la lista con
`grep -n 'plugins commands agents skills' legacy/lib/*.sh`.

- [ ] **Step 3: Comprobar**

Run: `go test ./internal/... && bash legacy/tests/run.sh && bash testdata/golden/capture.sh --check`
Expected: PASS.

- [ ] **Step 4: Commit**

Mensaje propuesto: `fix(profile): sembrar output-styles, hooks y keybindings.json en cada perfil (B4)`

---

### Task 6: B5 — la vista efectiva ve MCP, deny/ask y los ajustes sueltos

`ProfileEffective` (lo que enseñan la TUI y la pantalla Config) no enseñaba:
- los MCP que carga el perfil;
- `permissions.deny` y `permissions.ask`;
- `permissions.defaultMode`, `model`, `outputStyle` ni `statusLine`.

Se añaden cuatro secciones **al final** de `Sections`, para que ningún
consumidor que dependa del orden cambie:

| sección | qué lista | origen |
|---|---|---|
| `EffMCP` | los `mcpServers` del `.claude.json` que lee el perfil: `cc-home/.claude.json`, o `src+".json"` para `default` | `OriginClaudeJSON` (nuevo) |
| `EffDeny` | `permissions.deny` | la capa ganadora (arrays: reemplazo) |
| `EffAsk` | `permissions.ask` | ídem |
| `EffSettings` | `model`, `outputStyle`, `permissions.defaultMode`, `statusLine.command` | la capa ganadora; `statusLine` es `auto` si la capa de sensores lo envuelve |

**Files:**
- Modify: `internal/core/effective.go`
- Modify: `internal/core/effective_test.go`
- Modify: `internal/cli/serve_methods.go` (`effKindName`)
- Modify: `internal/tui/profile_view.go` (`effGroups`, `effGroupKey`), `internal/core/i18n/catalog_tui.go`
- Modify: `gui/src/lib/api.ts`, `gui/src/screens/Config.tsx`, `gui/src/lib/i18n_en.ts`

- [ ] **Step 1: Los tests (`effective_test.go`)**

Usa el fixture y el helper `sectionOf` que ya tiene el archivo.

```go
func TestEffectiveMCPAndSettings(t *testing.T) {
	home, src := t.TempDir(), t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{
	  "model": "opus",
	  "permissions": {"deny": ["Bash(rm -rf *)"], "ask": ["Bash(git push*)"], "defaultMode": "acceptEdits"}
	}`), 0o644)
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	CfgInitOverlay(home, "work")
	os.WriteFile(cfgSettingsFile(home, "work"), []byte(`{"outputStyle":"Explanatory","permissions":{"deny":["WebFetch"]}}`), 0o644)
	os.WriteFile(filepath.Join(ccHomePath(home, "work"), ".claude.json"),
		[]byte(`{"machineID":"x","mcpServers":{"github":{"command":"npx","args":["-y","gh"]},"jira":{"type":"http","url":"https://j/mcp"}}}`), 0o600)

	e, err := ProfileEffective(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	mcp := sectionOf(t, e, EffMCP).Rows
	if len(mcp) != 2 || mcp[0].Key != "github" || !strings.Contains(mcp[0].Value, "npx") ||
		mcp[1].Key != "jira" || !strings.Contains(mcp[1].Value, "https://j/mcp") || mcp[0].Origin != OriginClaudeJSON {
		t.Fatalf("MCP = %+v", mcp)
	}
	deny := sectionOf(t, e, EffDeny).Rows
	if len(deny) != 1 || deny[0].Key != "WebFetch" || deny[0].Origin != OriginOverlay || !deny[0].Shadowed {
		t.Fatalf("deny = %+v (el overlay reemplaza el array global entero)", deny)
	}
	if ask := sectionOf(t, e, EffAsk).Rows; len(ask) != 1 || ask[0].Origin != OriginGlobal {
		t.Fatalf("ask = %+v", ask)
	}
	settings := map[string]EffRow{}
	for _, r := range sectionOf(t, e, EffSettings).Rows {
		settings[r.Key] = r
	}
	if settings["model"].Value != "opus" || settings["outputStyle"].Origin != OriginOverlay ||
		settings["permissions.defaultMode"].Value != "acceptEdits" {
		t.Fatalf("settings = %+v", settings)
	}
	// Las secciones nuevas van al final: el orden de las de antes no cambia.
	if e.Sections[0].Kind != EffInstructions || e.Sections[len(e.Sections)-1].Kind != EffSettings {
		t.Fatalf("orden de secciones = %v", e.Sections)
	}
}
```

Run: `go test ./internal/core/ -run TestEffectiveMCPAndSettings`
Expected: FAIL (`undefined: EffMCP`).

- [ ] **Step 2: El motor — `internal/core/effective.go`**

1. Añade el origen y los tipos, **al final** de sus bloques:
   ```go
   	OriginAuto
   	// OriginClaudeJSON: sale del .claude.json que lee el perfil (los MCP de
   	// scope user), no de settings.json.
   	OriginClaudeJSON
   ```
   con `case OriginClaudeJSON: return "claude-json"` en `String()`, y
   ```go
   	EffSensors
   	EffMCP
   	EffDeny
   	EffAsk
   	EffSettings
   ```
2. En `ProfileEffective`, tras `effSensorsSection(cfg, name),`:
   ```go
   		effMCPSection(home, name, src),
   		EffSection{Kind: EffDeny, File: settingsFile, Err: L.overlayErr,
   			Rows: effArrayRows(L.docs, "permissions", "deny")},
   		EffSection{Kind: EffAsk, File: settingsFile, Err: L.overlayErr,
   			Rows: effArrayRows(L.docs, "permissions", "ask")},
   		EffSection{Kind: EffSettings, File: settingsFile, Err: L.overlayErr,
   			Rows: effSettingsRows(home, name, cfg, L)},
   ```
3. Las dos funciones nuevas:
   ```go
   // effMCPSection lista los MCP de scope user que carga el perfil: los de SU
   // .claude.json (cc-home/.claude.json; para default, el de junto a ~/.claude).
   // Los de proyecto (.mcp.json) dependen de la carpeta y no salen aquí.
   func effMCPSection(home, name, src string) EffSection {
   	path := src + ".json"
   	if name != "default" {
   		path = filepath.Join(ccHomePath(home, name), ".claude.json")
   	}
   	sec := EffSection{Kind: EffMCP, Rows: []EffRow{}}
   	data, err := os.ReadFile(path)
   	if err != nil {
   		return sec // sin archivo: sin MCP
   	}
   	var doc struct {
   		MCPServers map[string]struct {
   			Type    string   `json:"type"`
   			URL     string   `json:"url"`
   			Command string   `json:"command"`
   			Args    []string `json:"args"`
   		} `json:"mcpServers"`
   	}
   	if err := json.Unmarshal(data, &doc); err != nil {
   		sec.Err = fmt.Errorf("%s no es JSON válido: %w", path, err)
   		return sec
   	}
   	names := make([]string, 0, len(doc.MCPServers))
   	for n := range doc.MCPServers {
   		names = append(names, n)
   	}
   	sort.Strings(names)
   	for _, n := range names {
   		s := doc.MCPServers[n]
   		v := strings.TrimSpace(s.Command + " " + strings.Join(s.Args, " "))
   		if s.URL != "" {
   			t := s.Type
   			if t == "" {
   				t = "http"
   			}
   			v = t + " " + s.URL
   		}
   		sec.Rows = append(sec.Rows, EffRow{Key: n, Value: v, Origin: OriginClaudeJSON})
   	}
   	return sec
   }

   // effSettingKeys son los ajustes sueltos que la vista enseña, uno por fila.
   var effSettingKeys = [][]string{{"model"}, {"outputStyle"}, {"permissions", "defaultMode"}, {"statusLine", "command"}}

   // effSettingsRows: para cada ajuste, gana la última capa que lo define. La
   // barra de estado es la excepción: con la capa de sensores instalada, lo que
   // corre es el envoltorio de ccp (applyAutoLayer), igual que en effHookRows.
   func effSettingsRows(home, name string, cfg *Config, L effLayers) []EffRow {
   	out := []EffRow{}
   	for _, path := range effSettingKeys {
   		var row *EffRow
   		for _, l := range L.docs {
   			v, present := effAtPresent(l.doc, path...)
   			if !present {
   				continue
   			}
   			row = &EffRow{Key: strings.Join(path, "."), Value: effScalar(v), Origin: l.origin, Shadowed: row != nil}
   		}
   		if row != nil {
   			out = append(out, *row)
   		}
   	}
   	if name == "default" || !AutoHooksEnabled(cfg, name) {
   		return out
   	}
   	pre, err := MergeJSON(L.globalBytes, L.overlayBytes)
   	if err != nil {
   		return out
   	}
   	doc, err := effDecode(applyAutoLayer(home, name, pre))
   	if err != nil {
   		return out
   	}
   	cmd, ok := effAt(doc, "statusLine", "command").(string)
   	if !ok {
   		return out
   	}
   	for i, r := range out {
   		if r.Key == "statusLine.command" {
   			out[i] = EffRow{Key: r.Key, Value: cmd, Origin: OriginAuto, Shadowed: true}
   			return out
   		}
   	}
   	return append(out, EffRow{Key: "statusLine.command", Value: cmd, Origin: OriginAuto})
   }
   ```
   Añade `"strings"` a los imports si no está.

- [ ] **Step 3: `serve`, TUI y GUI**

- `internal/cli/serve_methods.go`, en `effKindName`:
  ```go
  	case core.EffMCP:
  		return "mcp"
  	case core.EffDeny:
  		return "deny"
  	case core.EffAsk:
  		return "ask"
  	case core.EffSettings:
  		return "settings"
  ```
- `internal/tui/profile_view.go`:
  - `effGroups` devuelve
    `{EffPermissions, EffDeny, EffAsk, EffSettings, EffMCP, EffHooks, EffPlugins, EffSensors}`;
  - `effGroupKey` gana los casos `tui.profview.deny`, `tui.profview.ask`,
    `tui.profview.settings` y `tui.profview.mcp`;
  - en `catalog_tui.go` van las cuatro claves:
    - `deny`: En «Denied» / Es «Denegados»;
    - `ask`: En «Ask first» / Es «Preguntar antes»;
    - `settings`: En «Settings» / Es «Ajustes»;
    - `mcp`: En «MCP servers» / Es «Servidores MCP».

  Si algún test de la TUI cuenta exactamente cuatro grupos, actualízalo a ocho.
- `gui/src/lib/api.ts`:
  ```ts
  export interface EffRow {
    key: string;
    value: string;
    origin: 'global' | 'overlay' | 'auto' | 'claude-json';
    shadowed: boolean;
  }

  export interface EffSection {
    kind: 'instructions' | 'env' | 'permissions' | 'deny' | 'ask' | 'settings' | 'mcp' | 'hooks' | 'plugins' | 'sensors' | 'other';
    file: string;
    error: string;
    rows: EffRow[];
  }
  ```
- `gui/src/screens/Config.tsx`:
  - en `SECTION_NAMES`: `deny: 'Permisos denegados'`,
    `ask: 'Permisos que preguntan'`, `settings: 'Ajustes'`,
    `mcp: 'Servidores MCP (del .claude.json del perfil)'`;
  - en `originLabel`, antes del `return` final:
    `if (r.origin === 'claude-json') return { label: t('.claude.json'), color: 'var(--ink-3)' };`
- `gui/src/lib/i18n_en.ts`: las traducciones inglesas de esas cinco cadenas
  nuevas.

- [ ] **Step 4: Comprobar**

Run: `go test ./internal/... && go vet ./... && (cd gui && npm run typecheck)`
Expected: PASS.

- [ ] **Step 5: Commit**

Mensaje propuesto: `feat(effective): MCP, deny/ask y ajustes sueltos en la vista efectiva (B5)`

---

### Task 7: cierre

- [ ] **Step 1:** `CHANGELOG.md`, *Unreleased*, una línea por arreglo (B1-B5)
  y, si ya está, el ADR 0016.
- [ ] **Step 2:** en `CLAUDE.md`:
  - la lista que siembra `seedCCHome` (B4);
  - que `HasLogin` mira la cuenta registrada (B3);
  - que `BackupRestore` regenera (B2).
- [ ] **Step 3:** en el spec, §1, marca B1-B5 como arreglados, con el commit de
  cada uno.
- [ ] **Step 4:** todos los gates. Mensaje propuesto: `docs: Fase 0 cerrada (B1-B5)`.

## Fuera de este plan

- **El arreglo de fondo de B1**: MCP por capas, proyectado a CLI y a Desktop. Es
  el subproyecto B del spec y se planifica con el resultado de M1-M6 en la mano.
- **Aviso en `ccp doctor` cuando el último snapshot es viejo.** Depende del plan
  de snapshots.
