package core

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// handoff_hardening_test.go — regresiones de la auditoría adversarial del motor
// (locking del ciclo leer-modificar-escribir, resolución por cwd exacto,
// degradación de versiones futuras y quoting del emit).

// --- Concurrencia: el ciclo leer-modificar-escribir debe ser atómico ---------

// Dos forwards en paralelo sobre repos/sesiones distintas no pueden pisarse:
// cada uno lee, muta y escribe la lista; sin el lock sostenido el segundo Save
// reescribe el snapshot viejo y pierde el marcador del primero (irrecuperable:
// sin from/to ya no se puede hacer `end` de esa sesión).
func TestHandoffForwardConcurrenteNoPierdeMarcadores(t *testing.T) {
	const n = 8
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cc := home + "/profiles/personal-1/cc-home"

	cwds := make([]string, n)
	uuids := make([]string, n)
	for i := 0; i < n; i++ {
		cwds[i] = fmt.Sprintf("/repo/p%02d", i)
		uuids[i] = fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)
		writeJSONL(t, ProjectDir(cc, SlugForCwd(cwds[i])), uuids[i], "T", time.Now())
	}

	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = HandoffForward(home, "personal-1", "work-1", cwds[i], uuids[i], true, false, false, time.Now())
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("forward %d: %v", i, err)
		}
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != n {
		t.Fatalf("esperaba %d marcadores activos tras %d forwards concurrentes, got %d: %+v",
			n, n, len(h.Active), h.Active)
	}
}

// Dos `end` en paralelo sobre marcadores distintos: ambos deben quedar
// archivados y ninguno resucitar como activo.
func TestHandoffEndConcurrenteNoResucitaMarcadores(t *testing.T) {
	const n = 6
	home := t.TempDir()
	seedHandoffEnv(t, home)
	dst := home + "/profiles/work-1/cc-home"

	var pre []Marker
	cwds := make([]string, n)
	for i := 0; i < n; i++ {
		cwds[i] = fmt.Sprintf("/repo/e%02d", i)
		uuid := fmt.Sprintf("%08d-2222-4222-8222-222222222222", i)
		writeJSONL(t, ProjectDir(dst, SlugForCwd(cwds[i])), uuid, "T", time.Now())
		pre = append(pre, Marker{
			Session: uuid, Slug: SlugForCwd(cwds[i]), Cwd: cwds[i],
			From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z",
		})
	}
	if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: pre}); err != nil {
		t.Fatal(err)
	}

	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = HandoffEnd(home, cwds[i], "", false, time.Now())
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("end %d: %v", i, err)
		}
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 {
		t.Fatalf("ningún marcador debía quedar activo, got %+v", h.Active)
	}
	if len(h.Archived) != n {
		t.Fatalf("esperaba %d archivados, got %d: %+v", n, len(h.Archived), h.Archived)
	}
}

// --- Carga: gate de versión y marcador v1 fantasma --------------------------

// El gate de versión futura debe evaluarse ANTES de elegir la rama de esquema:
// si no, un `active` que es mapping (v3 hipotético indexado por uuid, o el
// archivo editado a mano) se cuela por la ruta v1.
func TestLoadHandoffsVersionFuturaConActiveMapping(t *testing.T) {
	home := t.TempDir()
	future := "version: 99\nactive:\n  bbc1ed61-aaaa:\n    to: work-1\n    slug: -repo\n"
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 {
		t.Fatalf("una versión futura debe degradar a vacío, got %+v", h.Active)
	}
}

// Un `active` mapping que no es un Marker v1 no puede producir un marcador
// fantasma con todos los campos vacíos (aparecería como fila en blanco en
// `handoff list` y contaría para el aviso de acumulación).
func TestLoadHandoffsIgnoraMarcadorV1SinSesion(t *testing.T) {
	home := t.TempDir()
	raw := "version: 1\nactive:\n  perfil: work-1\n  ruta: /repo\n"
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 {
		t.Fatalf("un marcador v1 sin session es basura, no un activo: %+v", h.Active)
	}
}

// La degradación suave es de LECTURA: si el archivo es de una versión futura,
// escribirlo encima lo destruiría (los handoffs v3 en vuelo dejarían de poder
// devolverse). SaveHandoffs debe negarse y dejar el archivo intacto.
func TestSaveHandoffsNoPisaVersionFutura(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "handoffs.yaml")
	future := "version: 99\nnuevo_campo: x\nactive:\n- session: futuro\n  to: work-1\n"
	if err := os.WriteFile(path, []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	h.Active = append(h.Active, Marker{Session: "nuevo", Slug: "-r", Cwd: "/r", From: "a", To: "b", Since: "2026-07-25T00:00:00Z"})
	if err := SaveHandoffs(home, h); err == nil {
		t.Fatal("SaveHandoffs debía negarse a pisar un handoffs.yaml de versión futura")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "futuro") || !strings.Contains(string(data), "nuevo_campo") {
		t.Fatalf("el archivo de versión futura fue destruido:\n%s", data)
	}
}

// --- Resolución: cwd exacto, archivados y prefijos de uuid ------------------

// SlugForCwd aplana '/' y '-' al mismo carácter, así que dos proyectos
// distintos pueden compartir slug. La resolución debe usar el cwd exacto que
// guarda el marcador: si no, `end` desde un repo cierra el handoff de otro.
func TestActiveForCwdNoConfundeSlugsColisionados(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		{Session: "aaa", Slug: SlugForCwd("/repo/foo-bar"), Cwd: "/repo/foo-bar", From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z"},
	}}
	if SlugForCwd("/repo/foo-bar") != SlugForCwd("/repo/foo/bar") {
		t.Skip("los slugs ya no colisionan; el caso perdió sentido")
	}
	if got := ActiveForCwd(h, "/repo/foo/bar"); len(got) != 0 {
		t.Fatalf("/repo/foo/bar es otro proyecto: no debe resolver al marcador de /repo/foo-bar, got %v", got)
	}
	if got := ActiveForCwd(h, "/repo/foo-bar"); len(got) != 1 {
		t.Fatalf("el proyecto propio sí debe resolver, got %v", got)
	}
}

func TestHandoffEndNoCierraElHandoffDeOtroProyecto(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo/foo-bar"
	uuid := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	writeJSONL(t, ProjectDir(home+"/profiles/work-1/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())
	if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: SlugForCwd(cwd), Cwd: cwd, From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z",
	}}}); err != nil {
		t.Fatal(err)
	}
	// Otro proyecto cuyo slug colisiona con el del marcador.
	if _, err := HandoffEnd(home, "/repo/foo/bar", "", false, time.Now()); err == nil {
		t.Fatal("esperaba error: en /repo/foo/bar no hay handoff activo")
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 || len(h.Archived) != 0 {
		t.Fatalf("no debía tocar el handoff del otro proyecto: %+v", h)
	}
}

// --session de un handoff ya terminado debe decir que terminó y con qué uuid
// volvió, no «no hay handoff activo con la sesión X».
func TestResolveActiveSesionArchivada(t *testing.T) {
	h := &Handoffs{
		Version:  HandoffsVersion,
		Active:   []Marker{mk("vivo", "/otro", "personal-1", "work-1")},
		Archived: []ArchivedMarker{{Session: "muerto", From: "personal-1", To: "work-1", Slug: "-repo", ReturnedAs: "nuevo-uuid", Ended: "2026-07-25T10:00:00Z"}},
	}
	_, _, err := ResolveActive(h, "/repo", "muerto")
	if err == nil {
		t.Fatal("esperaba error con una sesión archivada")
	}
	if !strings.Contains(err.Error(), "ya terminó") || !strings.Contains(err.Error(), "nuevo-uuid") {
		t.Fatalf("el error debe decir que terminó y con qué uuid volvió: %v", err)
	}
}

// El hint de los errores muestra los uuid recortados; ese valor recortado tiene
// que ser aceptable por --session, o el camino de recuperación documentado
// (sin TTY, --session es la única salida) no existe.
func TestResolveActiveAceptaPrefijoDeSesion(t *testing.T) {
	full := "bbc1ed61-1111-4111-8111-111111111111"
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{mk(full, "/repo/uno", "personal-1", "work-1")}}
	_, _, err := ResolveActive(h, "/otro", "")
	if err == nil {
		t.Fatal("esperaba error sin activo para este cwd")
	}
	hint := err.Error()
	if !strings.Contains(hint, ShortUUID(full)) {
		t.Fatalf("el hint debe nombrar la sesión activa: %s", hint)
	}
	idx, _, err := ResolveActive(h, "/otro", ShortUUID(full))
	if err != nil {
		t.Fatalf("el uuid que el hint sugiere copiar debe resolver: %v", err)
	}
	if idx != 0 {
		t.Fatalf("esperaba idx 0, got %d", idx)
	}
}

func TestResolveActivePrefijoAmbiguoFalla(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		mk("abc11111-1111-4111-8111-111111111111", "/repo/uno", "p", "e"),
		mk("abc22222-2222-4222-8222-222222222222", "/repo/dos", "p", "k"),
	}}
	if _, _, err := ResolveActive(h, "/repo/uno", "abc"); err == nil {
		t.Fatal("un prefijo que empata con 2 activos no puede elegir uno")
	}
}

// --- Marcador huérfano: debe poder soltarse ---------------------------------

// Si el jsonl del destino desaparece, `end` y `resume` fallan siempre y la
// invariante de cadena bloquea el forward inverso: el marcador quedaría activo
// para siempre salvo editando handoffs.yaml a mano.
func TestHandoffDiscardSueltaMarcadorHuerfano(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	uuid := "ffffffff-ffff-4fff-8fff-ffffffffffff"
	if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: SlugForCwd(cwd), Cwd: cwd, From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z",
	}}}); err != nil {
		t.Fatal(err)
	}
	// Sin jsonl en el destino: end y resume no pueden cerrarlo.
	if _, err := HandoffEnd(home, cwd, "", false, time.Now()); err == nil {
		t.Fatal("end debía fallar sin transcript en el destino")
	}
	m, err := HandoffDiscard(home, cwd, "", time.Now())
	if err != nil {
		t.Fatalf("discard: %v", err)
	}
	if m.Session != uuid {
		t.Fatalf("descartó el marcador equivocado: %+v", m)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 0 {
		t.Fatalf("el marcador debía quedar suelto: %+v", h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].Session != uuid {
		t.Fatalf("el descarte debe dejar rastro archivado: %+v", h.Archived)
	}
}

func TestHandoffDiscardAmbiguoNoTocaNada(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	slug := SlugForCwd(cwd)
	if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{
		{Session: "aaa", Slug: slug, Cwd: cwd, From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z"},
		{Session: "bbb", Slug: slug, Cwd: cwd, From: "personal-1", To: "kimi", Since: "2026-07-25T01:00:00Z"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := HandoffDiscard(home, cwd, "", time.Now()); !errors.Is(err, ErrAmbiguousHandoff) {
		t.Fatalf("esperaba ErrAmbiguousHandoff, got %v", err)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatalf("un discard ambiguo no debe tocar el estado: %+v", h.Active)
	}
}

// --- `--session` es un uuid, no un path -------------------------------------

// El valor de --session se concatena para formar el path del transcript
// (<cc-home>/projects/<slug>/<id>.jsonl). Sin validarlo, un `..` copia cualquier
// .jsonl legible del disco al perfil destino y deja un marcador activo que
// apunta fuera del home: ese marcador fantasma secuestra la resolución por cwd
// del repo (end/resume ahí resuelven a él), cuenta para el aviso de acumulación
// y bloquea la invariante de cadena — todo con exit 0.
func TestHandoffForwardRechazaSessionConTraversal(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/work"
	slug := SlugForCwd(cwd)
	srcDir := ProjectDir(home+"/profiles/personal-1/cc-home", slug)
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Un .jsonl fuera del CCP_HOME, alcanzable solo saliendo del cc-home.
	outside := filepath.Join(t.TempDir(), "leak.jsonl")
	if err := os.WriteFile(outside, []byte("FUERA-DEL-HOME\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(srcDir, outside)
	if err != nil {
		t.Fatal(err)
	}
	traversal := strings.TrimSuffix(rel, ".jsonl")
	if !strings.Contains(traversal, "..") {
		t.Fatalf("el caso debe salir del cc-home para probar algo: %s", traversal)
	}

	if _, err := HandoffForward(home, "personal-1", "work-1", cwd, traversal, true, false, false, time.Now()); err == nil {
		t.Fatal("un --session con .. debe rechazarse, no copiar un jsonl ajeno")
	}
	dstDir := ProjectDir(home+"/profiles/work-1/cc-home", slug)
	if entries, _ := os.ReadDir(dstDir); len(entries) != 0 {
		t.Fatalf("no debía copiarse nada al perfil destino: %v", entries)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 0 {
		t.Fatalf("no debía quedar marcador activo: %+v", h.Active)
	}
}

func TestValidateSessionIDSoloAceptaUUIDs(t *testing.T) {
	ok := []string{
		"88888888-8888-4888-8888-888888888888",
		"BBC1ED61-ADA1-408F-0000-000000000000", // mayúsculas: mismo nombre de archivo
	}
	for _, s := range ok {
		if err := validateSessionID(s); err != nil {
			t.Fatalf("%q debía aceptarse: %v", s, err)
		}
	}
	bad := []string{
		"",
		".",
		"..",
		"../../otro/leak",
		"88888888-8888-4888-8888-888888888888/../otro",
		"/abs/oluto",
		"88888888-8888-4888-8888-88888888888",  // 11 hex al final
		"88888888_8888_4888_8888_888888888888", // guiones bajos
		"88888888-8888-4888-8888-888888888888\n",
	}
	for _, s := range bad {
		if err := validateSessionID(s); err == nil {
			t.Fatalf("%q debía rechazarse", s)
		}
	}
}

// --- Versión futura: las LECTURAS deben poder distinguirse de «no hay» ------

// LoadHandoffs degrada una versión futura a vacío; quien LEE tiene que poder
// saber que la lista está vacía porque el archivo es ilegible, no porque no haya
// handoffs. Sin ese dato, `handoff status` (el comando que uno corre justo para
// comprobarlo) afirmaría que no hay marcador.
func TestCheckUsableDistingueVacioDeIlegible(t *testing.T) {
	home := t.TempDir()
	vacio, err := LoadHandoffs(home) // sin archivo
	if err != nil {
		t.Fatal(err)
	}
	if err := vacio.CheckUsable(); err != nil {
		t.Fatalf("un handoffs.yaml ausente es legible y vacío: %v", err)
	}
	future := "version: 99\nactive:\n- session: 99999999-0000-4000-8000-000000000001\n  to: work-1\n"
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 {
		t.Fatalf("la degradación de lectura sigue siendo a vacío: %+v", h.Active)
	}
	cerr := h.CheckUsable()
	if cerr == nil {
		t.Fatal("una versión futura debe reportarse como ilegible, no como «sin handoffs»")
	}
	if !strings.Contains(cerr.Error(), "versión más nueva") {
		t.Fatalf("mensaje inesperado: %v", cerr)
	}
}

// --- Quoting del emit: nada de ejecución de comandos en el eval -------------

func TestWarnLineEscapaSusArgumentos(t *testing.T) {
	// Los 4 bytes que siguen activos dentro de "…" en bash y zsh: $ ` " \
	got := warnLine("cwd: %s", "/tmp/p$(id)`id`\"x\\y")
	want := "echo \"⚠️  ccp: cwd: /tmp/p\\$(id)\\`id\\`\\\"x\\\\y\" >&2\n"
	if got != want {
		t.Fatalf("warnLine no escapó su argumento\n got: %q\nwant: %q", got, want)
	}
	// El número sigue formateándose como número (los args no-string no se tocan).
	if n := warnLine("%d activos", 5); !strings.Contains(n, "5 activos") {
		t.Fatalf("warnLine rompió el formateo de un %%d: %s", n)
	}
}

// El caso real: $PWD llega crudo desde la shell function y se guarda crudo en
// el marcador; un directorio llamado `proj$(...)x` es perfectamente legal. Al
// evaluar el emit, ese comando NO puede ejecutarse.
func TestHandoffEmitNoEjecutaComandosDelCwd(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		sh := sh
		t.Run(sh, func(t *testing.T) {
			shPath, ok := lookShell(sh)
			if !ok {
				t.Skipf("%s no disponible en PATH", sh)
			}
			home := t.TempDir()
			seedHandoffEnv(t, home)
			canario := filepath.Join(t.TempDir(), "PWNED")

			// 4 marcadores previos; el más viejo con un cwd hostil, para cruzar
			// el umbral de acumulación y disparar el aviso que lo interpola.
			var pre []Marker
			for i := 0; i < ActiveWarnThreshold-1; i++ {
				cwd := fmt.Sprintf("/repo/viejo%d", i)
				if i == 0 {
					cwd = "/tmp/proj$(touch " + canario + ")x"
				}
				pre = append(pre, Marker{
					Session: fmt.Sprintf("old-%d-2222-4222-8222-222222222222", i),
					Slug:    SlugForCwd(cwd), Cwd: cwd,
					From: "personal-1", To: "work-1",
					Since: fmt.Sprintf("2026-07-0%dT00:00:00Z", i+1),
				})
			}
			if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: pre}); err != nil {
				t.Fatal(err)
			}

			cwd := "/repo/nuevo"
			uuid := "12341234-1234-4234-8234-123412341234"
			writeJSONL(t, ProjectDir(home+"/profiles/personal-1/cc-home", SlugForCwd(cwd)), uuid, "N", time.Now())
			emit, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, true, false, false, time.Now())
			if err != nil {
				t.Fatal(err)
			}

			script := emit + "\necho \"RID=$CCP_RESUME_ID\"\n"
			out, err := exec.Command(shPath, "-c", script).CombinedOutput()
			if err != nil {
				t.Fatalf("%s: el emit no evaluó limpio: %v\nsalida:\n%s\nscript:\n%s", sh, err, out, script)
			}
			if _, err := os.Stat(canario); err == nil {
				t.Fatalf("%s: el emit EJECUTÓ el comando escondido en el cwd del marcador", sh)
			}
			if !strings.Contains(string(out), "RID="+uuid) {
				t.Fatalf("%s: el emit no definió CCP_RESUME_ID:\n%s", sh, out)
			}
		})
	}
}

// La función shell lanza `claude` en un subshell tras evaluar el emit. Si ese
// eval falla (emit con error de sintaxis), NO puede seguir: el env no se aplicó,
// así que lanzaría claude en el perfil ORIGEN con --resume vacío mientras el
// marcador ya quedó persistido como activo. La guarda `|| exit` corta ahí.
// shellInitLaunchSnippet extrae del ShellInit el bloque que evalúa el emit y
// lanza claude: desde la línea con `eval "$out"` hasta el cierre del subshell
// (la primera línea que termina en `;;`), sin ese `;;` del case, para poder
// ejecutarlo tal cual en un shell real. El bloque es multilínea desde que la
// rama de --dangerously-skip-permissions vive en un if/else.
func shellInitLaunchSnippet() string {
	lines := strings.Split(ShellInit, "\n")
	start := -1
	for i, l := range lines {
		if strings.Contains(l, `eval "$out"`) {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	for i := start; i < len(lines); i++ {
		if strings.HasSuffix(strings.TrimSpace(lines[i]), ";;") {
			block := strings.Join(lines[start:i+1], "\n")
			return strings.TrimSuffix(strings.TrimSpace(block), ";;")
		}
	}
	return ""
}

func TestShellInitNoLanzaClaudeSiElEvalFalla(t *testing.T) {
	launch := shellInitLaunchSnippet()
	if launch == "" {
		t.Fatal("no encontré el bloque de lanzamiento del handoff en ShellInit")
	}
	if !strings.Contains(launch, "claude --resume") {
		t.Fatalf("el bloque de lanzamiento no llama a claude --resume: %s", launch)
	}
	if !strings.Contains(launch, `|| exit`) {
		t.Fatalf("el bloque de lanzamiento no aborta si el eval falla: %s", launch)
	}

	for _, sh := range []string{"bash", "zsh"} {
		sh := sh
		t.Run(sh, func(t *testing.T) {
			shPath, ok := lookShell(sh)
			if !ok {
				t.Skipf("%s no disponible en PATH", sh)
			}
			for _, tc := range []struct {
				name    string
				out     string
				lanzado bool
			}{
				{"emit roto", `echo "5 (viejo: /tmp/pro"j, x)" >&2`, false},
				{"emit válido", `CCP_RESUME_ID=abc`, true},
			} {
				canario := filepath.Join(t.TempDir(), "LANZADO")
				script := "claude() { : > " + canario + "; }\n" +
					"out='" + tc.out + "'\n" + launch + "\nexit 0\n"
				if out, err := exec.Command(shPath, "-c", script).CombinedOutput(); err != nil {
					t.Fatalf("%s/%s: %v\n%s", sh, tc.name, err, out)
				}
				_, err := os.Stat(canario)
				if lanzado := err == nil; lanzado != tc.lanzado {
					t.Errorf("%s/%s: lanzó claude=%v, esperaba %v", sh, tc.name, lanzado, tc.lanzado)
				}
			}
		})
	}
}

// Una comilla doble en un nombre de directorio (legal) no puede romper el
// parseo del emit: bash y zsh parsean TODO el input antes de ejecutar nada, así
// que un emit inválido deja el env sin cambiar y `claude` se lanzaría en el
// perfil equivocado.
func TestHandoffEmitParseaConComillasEnElCwd(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		sh := sh
		t.Run(sh, func(t *testing.T) {
			shPath, ok := lookShell(sh)
			if !ok {
				t.Skipf("%s no disponible en PATH", sh)
			}
			home := t.TempDir()
			seedHandoffEnv(t, home)

			var pre []Marker
			for i := 0; i < ActiveWarnThreshold-1; i++ {
				cwd := fmt.Sprintf("/repo/viejo%d", i)
				if i == 0 {
					cwd = `/tmp/pro"j`
				}
				pre = append(pre, Marker{
					Session: fmt.Sprintf("old-%d-3333-4333-8333-333333333333", i),
					Slug:    SlugForCwd(cwd), Cwd: cwd,
					From: "personal-1", To: "work-1",
					Since: fmt.Sprintf("2026-07-0%dT00:00:00Z", i+1),
				})
			}
			if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: pre}); err != nil {
				t.Fatal(err)
			}
			cwd := "/repo/nuevo"
			uuid := "56785678-5678-4678-8678-567856785678"
			writeJSONL(t, ProjectDir(home+"/profiles/personal-1/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())
			emit, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, true, false, false, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(shPath, "-c", emit+"\necho \"CCD=$CLAUDE_CONFIG_DIR\"\n").CombinedOutput()
			if err != nil {
				t.Fatalf("%s: el emit no evaluó limpio: %v\nsalida:\n%s\nscript:\n%s", sh, err, out, emit)
			}
			if !strings.Contains(string(out), "work-1/cc-home") {
				t.Fatalf("%s: el env del destino no se aplicó:\n%s", sh, out)
			}
		})
	}
}
