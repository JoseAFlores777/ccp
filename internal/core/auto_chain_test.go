package core

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// auto_chain_test.go — las mutaciones de `ccp auto chain`.
//
// Lo que se fija aquí no es «la lista queda como pide el usuario» (eso es fácil)
// sino los tres límites del ensanche de allow_from: solo la entrada del primario,
// nunca inventar un gate donde no lo había, y --no-allow como interruptor.

// chainYAML es autoBaseYAML (perfiles personal-1/work-2/work-1/personal-deepseek
// y reglas /work/personal→personal-1, /work/beta→work-1) más un bloque
// auto_handoff con gate DECLARADO.
const chainYAML = autoBaseYAML + `auto_handoff:
  enabled: true
  policies:
    default:
      fallback:
        - work-2
        - work-1
  allow_from:
    personal-1:
      - work-2
      - work-1
    work-1:
      - work-1
`

// chainYAMLSinGate es el mismo bloque SIN allow_from: no hay gate.
const chainYAMLSinGate = autoBaseYAML + `auto_handoff:
  enabled: true
  policies:
    default:
      fallback:
        - work-2
        - work-1
`

// chainYAMLDosPoliticas añade una segunda política, para los tests de la
// política LIGADA a un perfil. Es un const aparte y no una concatenación sobre
// chainYAML porque ese acaba dentro de `allow_from`: pegarle claves de política
// las metía en el gate y el yaml ni siquiera cargaba.
const chainYAMLDosPoliticas = autoBaseYAML + `auto_handoff:
  enabled: true
  policies:
    default:
      fallback:
        - work-2
        - work-1
    relajada:
      threshold: 80
      fallback:
        - work-1
  allow_from:
    personal-1:
      - work-2
      - work-1
    work-1:
      - work-1
`

// chainHome siembra un home temporal con el yaml dado.
func chainHome(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	seedAutoHome(t, home, body)
	return home
}

// chainFallbackOf relee del disco la cadena que USA un perfil: la suya propia si
// la declara, y si no la de la política default, que es de donde la hereda.
//
// El parámetro `owner` apareció cuando las cadenas pasaron a ser por perfil. Las
// mutaciones escriben por defecto en `chains[<primario>]`, así que un helper que
// siguiera leyendo solo `policies.default.fallback` daría por buena una cadena
// que ya no es la que el supervisor lee — o sea, pasaría en verde justo el fallo
// que tiene que cazar.
func chainFallbackOf(t *testing.T, home, owner string) []string {
	t.Helper()
	if owner != "" {
		cfg, err := Load(home)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if pc := AutoChainFor(cfg, owner); pc.Declared {
			return pc.Fallback
		}
	}
	return chainSharedFallbackOf(t, home)
}

// chainSharedFallbackOf lee la lista COMPARTIDA de la política default.
func chainSharedFallbackOf(t *testing.T, home string) []string {
	t.Helper()
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AutoHandoff == nil {
		t.Fatal("auto_handoff desapareció del yaml")
	}
	return cfg.AutoHandoff.Policies["default"].Fallback
}

// chainAllowOf relee del disco una entrada de allow_from.
func chainAllowOf(t *testing.T, home, primary string) ([]string, bool) {
	t.Helper()
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	entry, ok := cfg.AutoHandoff.AllowFrom[primary]
	return entry, ok
}

func chainEq(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// chainErrKind extrae el Kind de un error tipado del core.
func chainErrKind(t *testing.T, err error) ChainErrKind {
	t.Helper()
	var ce *ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("se esperaba *ChainError, llegó %T (%v)", err, err)
	}
	return ce.Kind
}

// ---------------------------------------------------------------------------
// El límite 1 de D5: add toca SOLO la entrada del primario actual.
// ---------------------------------------------------------------------------

func TestChainAddNoTocaOtrosPrimarios(t *testing.T) {
	home := chainHome(t, chainYAML)

	res, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"personal-deepseek"})
	if err != nil {
		t.Fatalf("ChainAdd: %v", err)
	}
	if res.Primary != "personal-1" {
		t.Fatalf("primario = %q, want personal-1", res.Primary)
	}
	if !chainEq(res.AllowAdded, []string{"personal-deepseek"}) {
		t.Errorf("AllowAdded = %v, want [personal-deepseek]", res.AllowAdded)
	}

	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"work-2", "work-1", "personal-deepseek"}) {
		t.Errorf("fallback = %v", got)
	}
	if got, _ := chainAllowOf(t, home, "personal-1"); !chainEq(got, []string{"work-2", "work-1", "personal-deepseek"}) {
		t.Errorf("allow_from[personal-1] = %v", got)
	}
	// El corazón del test: la entrada de OTRO primario no se movió ni un byte.
	// Un rewrite en bloque ensancharía permisos de repos donde el usuario ni está.
	if got, _ := chainAllowOf(t, home, "work-1"); !chainEq(got, []string{"work-1"}) {
		t.Errorf("allow_from[work-1] = %v, want [work-1] intacto", got)
	}
}

// ---------------------------------------------------------------------------
// Estado 1 del gate: mapa ausente. Crear ahí la entrada del primario declararía
// un gate donde no lo había y dejaría a TODOS los demás primarios en deny total.
// ---------------------------------------------------------------------------

func TestChainAddSinGateNoLoInventa(t *testing.T) {
	home := chainHome(t, chainYAMLSinGate)

	res, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"personal-deepseek"})
	if err != nil {
		t.Fatalf("ChainAdd: %v", err)
	}
	if !res.GateAbsent {
		t.Error("GateAbsent debería ser true: el yaml no declara allow_from")
	}
	if len(res.AllowAdded) != 0 || res.AllowCreated {
		t.Errorf("no se debió tocar allow_from: added=%v created=%v", res.AllowAdded, res.AllowCreated)
	}

	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AutoHandoff.AllowFrom) != 0 {
		t.Errorf("allow_from se inventó: %v", cfg.AutoHandoff.AllowFrom)
	}
	// El préstamo, que era lo que se pedía, sí quedó abierto.
	rc, err := ResolveAutoChain(home, nil, "", "/work/personal")
	if err != nil {
		t.Fatal(err)
	}
	if !chainEq(rc.Fallback, []string{"work-2", "work-1", "personal-deepseek"}) {
		t.Errorf("cadena efectiva = %v", rc.Fallback)
	}
}

// Estado 3 del gate: declarado SIN entrada para el primario (deny total). La
// entrada se CREA, y con exactamente lo autorizado: ni un candidato más.
func TestChainAddCreaLaEntradaEnDenyTotal(t *testing.T) {
	// El primario de /work/beta es work-1... pero usamos work-2 como primario
	// declarando una regla nueva: /work/labs no tiene entrada en allow_from.
	home := chainHome(t, chainYAML)
	if _, err := RuleSet(home, "/work/labs", "work-2"); err != nil {
		t.Fatal(err)
	}

	res, err := ChainAdd(home, ChainOpts{Cwd: "/work/labs"}, []string{"personal-deepseek"})
	if err != nil {
		t.Fatalf("ChainAdd: %v", err)
	}
	if res.Primary != "work-2" {
		t.Fatalf("primario = %q, want work-2", res.Primary)
	}
	if !res.AllowCreated {
		t.Error("AllowCreated debería ser true: no había entrada para work-2")
	}
	got, ok := chainAllowOf(t, home, "work-2")
	if !ok {
		t.Fatal("no se creó allow_from[work-2]")
	}
	// Solo lo pedido: los otros candidatos siguen tan bloqueados como estaban.
	if !chainEq(got, []string{"personal-deepseek"}) {
		t.Errorf("allow_from[work-2] = %v, want [personal-deepseek]", got)
	}
	rc, err := ResolveAutoChain(home, nil, "", "/work/labs")
	if err != nil {
		t.Fatal(err)
	}
	if !chainEq(rc.Fallback, []string{"personal-deepseek"}) {
		t.Errorf("cadena efectiva = %v, want solo el recién autorizado", rc.Fallback)
	}
	if !chainEq(rc.Denied, []string{"work-1"}) {
		t.Errorf("denegados = %v, want [work-1]", rc.Denied)
	}
}

// ---------------------------------------------------------------------------
// --no-allow: el interruptor del límite 3.
// ---------------------------------------------------------------------------

func TestChainAddNoAllow(t *testing.T) {
	home := chainHome(t, chainYAML)

	res, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal", NoAllow: true}, []string{"personal-deepseek"})
	if err != nil {
		t.Fatalf("ChainAdd: %v", err)
	}
	if !res.AllowSkipped || len(res.AllowAdded) != 0 {
		t.Errorf("con --no-allow: skipped=%v added=%v", res.AllowSkipped, res.AllowAdded)
	}
	if got, _ := chainAllowOf(t, home, "personal-1"); !chainEq(got, []string{"work-2", "work-1"}) {
		t.Errorf("allow_from[personal-1] = %v, debía quedar intacto", got)
	}
	// Y la consecuencia que el usuario tiene que ver: el perfil está en la cadena
	// pero el gate lo sigue denegando.
	rc, err := ResolveAutoChain(home, nil, "", "/work/personal")
	if err != nil {
		t.Fatal(err)
	}
	if !chainEq(rc.Denied, []string{"personal-deepseek"}) {
		t.Errorf("denegados = %v, want [personal-deepseek]", rc.Denied)
	}
}

// ---------------------------------------------------------------------------
// Validaciones
// ---------------------------------------------------------------------------

func TestChainAddRechazaPerfilInexistente(t *testing.T) {
	home := chainHome(t, chainYAML)

	_, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"personal-deepseek", "fantasma"})
	if err == nil {
		t.Fatal("se esperaba error por perfil inexistente")
	}
	if k := chainErrKind(t, err); k != ChainErrNoProfile {
		t.Errorf("kind = %v, want ChainErrNoProfile", k)
	}
	if !strings.Contains(err.Error(), "fantasma") {
		t.Errorf("el error debe nombrar el perfil: %q", err.Error())
	}
	// Validación completa ANTES de mutar: el perfil bueno tampoco entró.
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"work-2", "work-1"}) {
		t.Errorf("la cadena se mutó a medias: %v", got)
	}

	// Duplicado: tampoco entra dos veces.
	_, err = ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"work-2"})
	if k := chainErrKind(t, err); k != ChainErrDuplicate {
		t.Errorf("kind = %v, want ChainErrDuplicate", k)
	}
}

func TestChainAddPosicion(t *testing.T) {
	home := chainHome(t, chainYAML)

	if _, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal", At: 1}, []string{"personal-deepseek"}); err != nil {
		t.Fatalf("ChainAdd --at 1: %v", err)
	}
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"personal-deepseek", "work-2", "work-1"}) {
		t.Errorf("--at 1 dejó %v", got)
	}

	// Fuera de rango: error nombrando el rango válido (len+1 porque insertar al
	// final es una posición legítima), y sin tocar el disco.
	_, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal", At: 99}, []string{"default"})
	if k := chainErrKind(t, err); k != ChainErrRange {
		t.Fatalf("kind = %v, want ChainErrRange", k)
	}
	if msg := err.Error(); !strings.Contains(msg, "1..4") {
		t.Errorf("el error debe nombrar el rango 1..4: %q", msg)
	}
	if got := chainFallbackOf(t, home, "personal-1"); len(got) != 3 {
		t.Errorf("una posición inválida no debe mutar nada: %v", got)
	}

	// El límite superior sí es len+1.
	if _, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal", At: 4}, []string{"default"}); err != nil {
		t.Fatalf("ChainAdd --at 4 (== len+1): %v", err)
	}
	if got := chainFallbackOf(t, home, "personal-1"); got[3] != "default" {
		t.Errorf("--at len+1 debe insertar al final: %v", got)
	}
}

// TestChainAddPrimarioAvisaQueEsImplicito: meter un perfil en SU PROPIA cadena
// se acepta y se avisa.
//
// El aviso es ChainNoteSelf y no ChainNotePrimaryImplicit porque desde que la
// cadena es por perfil los dos casos dejaron de ser el mismo: en la cadena propia
// de `personal-1`, el nombre `personal-1` no vale NUNCA (nadie se presta a sí
// mismo), mientras que en la lista compartida solo no vale mientras este cwd
// resuelva ahí. Decirle «es implícito» a algo que no va a valer jamás invita a
// dejarlo puesto.
func TestChainAddPrimarioAvisaQueEsImplicito(t *testing.T) {
	home := chainHome(t, chainYAML)

	res, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"personal-1"})
	if err != nil {
		t.Fatalf("añadir el primario debe ACEPTARSE: %v", err)
	}
	if len(res.Notes) != 1 || res.Notes[0].Kind != ChainNoteSelf {
		t.Fatalf("faltó el aviso de que el perfil se metió en su propia cadena: %+v", res.Notes)
	}
	// Se guarda tal cual (mañana la regla cambia y la entrada empieza a valer)...
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"work-2", "work-1", "personal-1"}) {
		t.Errorf("fallback = %v", got)
	}
	// ...pero hoy ResolveAutoChain lo filtra, que es de lo que avisa la nota.
	rc, err := ResolveAutoChain(home, nil, "", "/work/personal")
	if err != nil {
		t.Fatal(err)
	}
	if chainIndex(rc.Fallback, "personal-1") >= 0 {
		t.Errorf("el primario no debe aparecer en la cadena efectiva: %v", rc.Fallback)
	}
	// Y no se autoriza a sí mismo en el gate: sería ruido en el yaml.
	if got, _ := chainAllowOf(t, home, "personal-1"); chainIndex(got, "personal-1") >= 0 {
		t.Errorf("el primario se metió en su propia entrada: %v", got)
	}
}

func TestChainMvReordena(t *testing.T) {
	home := chainHome(t, chainYAML)
	if _, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"personal-deepseek"}); err != nil {
		t.Fatal(err)
	}

	res, err := ChainMv(home, ChainOpts{Cwd: "/work/personal"}, "personal-deepseek", 1)
	if err != nil {
		t.Fatalf("ChainMv: %v", err)
	}
	if res.Moved != "personal-deepseek" || res.MovedTo != 1 {
		t.Errorf("res = %+v", res)
	}
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"personal-deepseek", "work-2", "work-1"}) {
		t.Errorf("fallback = %v", got)
	}

	// Al medio: pos es 1-based sobre la lista RESULTANTE.
	if _, err := ChainMv(home, ChainOpts{Cwd: "/work/personal"}, "personal-deepseek", 2); err != nil {
		t.Fatal(err)
	}
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"work-2", "personal-deepseek", "work-1"}) {
		t.Errorf("fallback = %v", got)
	}

	// Fuera de rango: aquí el máximo es len (no len+1: no se inserta, se mueve).
	_, err = ChainMv(home, ChainOpts{Cwd: "/work/personal"}, "personal-deepseek", 9)
	if k := chainErrKind(t, err); k != ChainErrRange {
		t.Fatalf("kind = %v, want ChainErrRange", k)
	}
	if msg := err.Error(); !strings.Contains(msg, "1..3") {
		t.Errorf("el error debe nombrar el rango 1..3: %q", msg)
	}

	// Un perfil que no está en la cadena.
	_, err = ChainMv(home, ChainOpts{Cwd: "/work/personal"}, "default", 1)
	if k := chainErrKind(t, err); k != ChainErrNotInChain {
		t.Errorf("kind = %v, want ChainErrNotInChain", k)
	}
}

func TestChainRmAvisaSiNoEstaba(t *testing.T) {
	home := chainHome(t, chainYAML)

	// Lo que NO puede pasar: salir 0 y no haber hecho nada.
	_, err := ChainRm(home, ChainOpts{Cwd: "/work/personal"}, []string{"personal-deepseek"})
	if err == nil {
		t.Fatal("quitar algo que no está debe fallar, no ser un no-op silencioso")
	}
	if k := chainErrKind(t, err); k != ChainErrNotInChain {
		t.Fatalf("kind = %v, want ChainErrNotInChain", k)
	}
	msg := err.Error()
	if !strings.Contains(msg, "personal-deepseek") || !strings.Contains(msg, "work-2") {
		t.Errorf("el error debe nombrar el perfil y lo que SÍ hay: %q", msg)
	}

	// El caso bueno, y con validación previa: `rm work-2 fantasma` no deja work-2
	// fuera antes de descubrir que fantasma no estaba.
	if _, err := ChainRm(home, ChainOpts{Cwd: "/work/personal"}, []string{"work-2", "fantasma"}); err == nil {
		t.Fatal("se esperaba error")
	}
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"work-2", "work-1"}) {
		t.Errorf("la cadena se mutó a medias: %v", got)
	}

	res, err := ChainRm(home, ChainOpts{Cwd: "/work/personal"}, []string{"work-2"})
	if err != nil {
		t.Fatalf("ChainRm: %v", err)
	}
	if !chainEq(res.Removed, []string{"work-2"}) {
		t.Errorf("Removed = %v", res.Removed)
	}
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"work-1"}) {
		t.Errorf("fallback = %v", got)
	}
	// rm SÍ retira la autorización, y solo la del primario actual: es la única
	// forma que hay por CLI de estrechar el gate. Sin esto `add` sería un embudo
	// —el gate solo podría CRECER— y deshacer un `add` equivocado obligaría a
	// editar el yaml a mano, que es justo lo que estos comandos vienen a evitar.
	if !chainEq(res.AllowRemoved, []string{"work-2"}) {
		t.Errorf("AllowRemoved = %v, want [work-2]", res.AllowRemoved)
	}
	if got, _ := chainAllowOf(t, home, "personal-1"); !chainEq(got, []string{"work-1"}) {
		t.Errorf("allow_from[personal-1] = %v, want [work-1]", got)
	}
	// Límite 1, también aquí: la entrada de OTRO primario no se toca.
	if got, _ := chainAllowOf(t, home, "work-1"); !chainEq(got, []string{"work-1"}) {
		t.Errorf("allow_from[work-1] = %v, want [work-1] intacto", got)
	}
}

// TestChainRmCierraElCicloDeAdd es el rollback que el plan promete para el riesgo
// R3 («`ccp auto chain rm` deshace ambas claves»): tras add + rm el yaml tiene que
// quedar en el MISMO estado que antes del add, en las dos claves.
func TestChainRmCierraElCicloDeAdd(t *testing.T) {
	home := chainHome(t, chainYAML)
	opts := ChainOpts{Cwd: "/work/personal"}

	fbAntes := chainFallbackOf(t, home, "personal-1")
	allowAntes, _ := chainAllowOf(t, home, "personal-1")

	if _, err := ChainAdd(home, opts, []string{"personal-deepseek"}); err != nil {
		t.Fatalf("ChainAdd: %v", err)
	}
	if got, _ := chainAllowOf(t, home, "personal-1"); chainIndex(got, "personal-deepseek") < 0 {
		t.Fatalf("el add no autorizó nada: %v", got)
	}

	if _, err := ChainRm(home, opts, []string{"personal-deepseek"}); err != nil {
		t.Fatalf("ChainRm: %v", err)
	}
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, fbAntes) {
		t.Errorf("fallback = %v, want %v", got, fbAntes)
	}
	if got, _ := chainAllowOf(t, home, "personal-1"); !chainEq(got, allowAntes) {
		t.Errorf("allow_from[personal-1] = %v, want %v", got, allowAntes)
	}
}

// TestChainRmNoAllow: el interruptor del límite 3 vale igual para estrechar. Hay
// quien quiere sacar el perfil de la cadena pero dejar la autorización escrita
// (p.ej. porque la puso el equipo y no es suya para quitarla).
func TestChainRmNoAllow(t *testing.T) {
	home := chainHome(t, chainYAML)

	res, err := ChainRm(home, ChainOpts{Cwd: "/work/personal", NoAllow: true}, []string{"work-2"})
	if err != nil {
		t.Fatalf("ChainRm: %v", err)
	}
	if !res.AllowSkipped || len(res.AllowRemoved) != 0 {
		t.Errorf("con --no-allow: skipped=%v removed=%v", res.AllowSkipped, res.AllowRemoved)
	}
	if got, _ := chainAllowOf(t, home, "personal-1"); !chainEq(got, []string{"work-2", "work-1"}) {
		t.Errorf("allow_from[personal-1] = %v, debía quedar intacto", got)
	}
}

// TestChainAddDesatascaElDenyTotal es el callejón sin salida que D5 existe para
// cerrar: en un repo cuyo primario no tiene entrada en allow_from, `auto init`
// deja TODOS los perfiles ya dentro de `fallback`, así que un chequeo de
// duplicado sobre la lista cruda hacía que `add <cualquiera>` respondiera «ya
// está en la cadena» — con la cadena efectiva VACÍA en la misma sesión. No había
// ninguna invocación de `ccp auto chain` capaz de des-denegar el repo.
func TestChainAddDesatascaElDenyTotal(t *testing.T) {
	home := chainHome(t, chainYAML)
	// /work/labs → work-2, que NO tiene entrada en allow_from: deny total.
	if _, err := RuleSet(home, "/work/labs", "work-2"); err != nil {
		t.Fatal(err)
	}
	opts := ChainOpts{Cwd: "/work/labs"}

	rc, err := ResolveAutoChain(home, nil, "", "/work/labs")
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Fallback) != 0 {
		t.Fatalf("el fixture debía partir en deny total: %v", rc.Fallback)
	}

	// work-1 YA está en `fallback`. Antes esto era ChainErrDuplicate y el repo
	// se quedaba sin salida.
	res, err := ChainAdd(home, opts, []string{"work-1"})
	if err != nil {
		t.Fatalf("add de un perfil en la cadena pero denegado debe ABRIR el gate: %v", err)
	}
	if !res.AllowCreated || !chainEq(res.AllowAdded, []string{"work-1"}) {
		t.Errorf("res = created:%v added:%v", res.AllowCreated, res.AllowAdded)
	}
	// La cadena no cambió, y hay que decirlo: si no, la línea de fallback sale
	// idéntica y parece un no-op.
	if !chainEq(res.Fallback, []string{"work-2", "work-1"}) {
		t.Errorf("la cadena no debía cambiar: %v", res.Fallback)
	}
	var vistaNota bool
	for _, n := range res.Notes {
		if n.Kind == ChainNoteAlreadyInChain && n.Profile == "work-1" {
			vistaNota = true
		}
	}
	if !vistaNota {
		t.Errorf("falta la nota de «ya estaba en la cadena»: %+v", res.Notes)
	}

	// Y el repo quedó desatascado de verdad.
	rc, err = ResolveAutoChain(home, nil, "", "/work/labs")
	if err != nil {
		t.Fatal(err)
	}
	if !chainEq(rc.Fallback, []string{"work-1"}) {
		t.Errorf("cadena efectiva = %v, want [work-1]", rc.Fallback)
	}

	// Repetirlo ya SÍ es un duplicado: está en la cadena y además autorizado.
	if _, err := ChainAdd(home, opts, []string{"work-1"}); chainErrKind(t, err) != ChainErrDuplicate {
		t.Errorf("el segundo add debe ser duplicado: %v", err)
	}
	// Y con --no-allow tampoco hay gate que abrir, así que vuelve a ser duplicado.
	if _, err := ChainAdd(home, ChainOpts{Cwd: "/work/labs", NoAllow: true}, []string{"work-2"}); err == nil {
		t.Error("add --no-allow de algo ya en la cadena debe seguir siendo duplicado")
	}
}

func TestChainSetReemplaza(t *testing.T) {
	home := chainHome(t, chainYAML)

	res, err := ChainSet(home, ChainOpts{Cwd: "/work/personal"}, []string{"work-1", "personal-deepseek"})
	if err != nil {
		t.Fatalf("ChainSet: %v", err)
	}
	if !chainEq(res.Fallback, []string{"work-1", "personal-deepseek"}) {
		t.Errorf("Fallback = %v", res.Fallback)
	}
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"work-1", "personal-deepseek"}) {
		t.Errorf("fallback en disco = %v", got)
	}
	// set ajusta el gate en las DOS direcciones, porque es el rm+add que de hecho
	// es: personal-deepseek entra y se autoriza, work-2 sale y pierde el permiso,
	// work-1 sigue en ambos lados y no se toca. Sin esto, `set` podía dejar la
	// cadena INERTE —lista nueva en el yaml, ningún destino autorizado— con un
	// `[ok]` delante y sin una línea que lo dijera.
	if !chainEq(res.AllowAdded, []string{"personal-deepseek"}) {
		t.Errorf("AllowAdded = %v", res.AllowAdded)
	}
	if !chainEq(res.AllowRemoved, []string{"work-2"}) {
		t.Errorf("AllowRemoved = %v", res.AllowRemoved)
	}
	if got, _ := chainAllowOf(t, home, "personal-1"); !chainEq(got, []string{"work-1", "personal-deepseek"}) {
		t.Errorf("allow_from[personal-1] = %v", got)
	}
	// Y la consecuencia: la cadena que se acaba de escribir se usa de verdad.
	rc, err := ResolveAutoChain(home, nil, "", "/work/personal")
	if err != nil {
		t.Fatal(err)
	}
	if !chainEq(rc.Fallback, []string{"work-1", "personal-deepseek"}) {
		t.Errorf("cadena efectiva = %v, la lista nueva quedó inerte", rc.Fallback)
	}

	// Perfil inexistente y duplicado: mismos errores tipados que add.
	if _, err := ChainSet(home, ChainOpts{Cwd: "/work/personal"}, []string{"fantasma"}); chainErrKind(t, err) != ChainErrNoProfile {
		t.Errorf("set con perfil inexistente: %v", err)
	}
	if _, err := ChainSet(home, ChainOpts{Cwd: "/work/personal"}, []string{"work-2", "work-2"}); chainErrKind(t, err) != ChainErrDuplicate {
		t.Errorf("set con duplicado: %v", err)
	}
	// Sin nombres: vaciar la cadena en silencio dejaría a `ccp session` sin
	// destinos, así que se exige al menos uno.
	if _, err := ChainSet(home, ChainOpts{Cwd: "/work/personal"}, nil); chainErrKind(t, err) != ChainErrEmpty {
		t.Errorf("set sin nombres: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Forward-compat: mutar la cadena no puede ser la puerta por la que se cuela un
// bump de esquema ni por la que se pierden las claves que este binario no conoce.
// ---------------------------------------------------------------------------

func TestChainPersisteVersionDelEsquema(t *testing.T) {
	home := chainHome(t, chainYAML+`future_key: keepme
future_block:
  nested: 1
`)

	if _, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"personal-deepseek"}); err != nil {
		t.Fatalf("ChainAdd: %v", err)
	}
	raw, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "version: 2") {
		t.Errorf("el esquema debe seguir en 2:\n%s", text)
	}
	if !strings.Contains(text, "future_key: keepme") || !strings.Contains(text, "nested: 1") {
		t.Errorf("se perdieron claves desconocidas:\n%s", text)
	}

	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != SchemaVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, SchemaVersion)
	}
	if _, ok := cfg.Extra["future_key"]; !ok {
		t.Errorf("Extra = %v", cfg.Extra)
	}
}

// Sin bloque auto_handoff no se escribe nada: el error manda a `ccp auto init`
// en vez de sembrar media política por su cuenta.
func TestChainSinBloqueAutoHandoff(t *testing.T) {
	home := chainHome(t, autoBaseYAML)

	_, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"work-2"})
	if k := chainErrKind(t, err); k != ChainErrNotConfigured {
		t.Fatalf("kind = %v, want ChainErrNotConfigured", k)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoHandoff != nil {
		t.Error("no se debe sembrar el bloque desde chain")
	}

	// Política inexistente: se nombran las que hay.
	home = chainHome(t, chainYAML)
	_, err = ChainAdd(home, ChainOpts{Cwd: "/work/personal", Policy: "noche"}, []string{"work-2"})
	if k := chainErrKind(t, err); k != ChainErrNoPolicy {
		t.Fatalf("kind = %v, want ChainErrNoPolicy", k)
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("el error debe listar las políticas existentes: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Cadenas por perfil: el destino por defecto y la bifurcación de la herencia.
// ---------------------------------------------------------------------------

// TestChainAddEscribeLaCadenaDelPrimario es el cambio de destino: `add` sin
// banderas escribe `chains[<primario>]`, no la lista compartida.
//
// Las dos mitades importan por igual. Que la cadena propia quede con lo pedido
// prueba que se escribió donde el supervisor va a leer; que la compartida NO se
// mueva prueba lo que el usuario vino a pedir — que tocar la cadena de un repo
// deje de cambiársela a todos los demás.
func TestChainAddEscribeLaCadenaDelPrimario(t *testing.T) {
	home := chainHome(t, chainYAML)

	res, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"personal-deepseek"})
	if err != nil {
		t.Fatalf("ChainAdd: %v", err)
	}
	if res.Owner != "personal-1" || res.Shared {
		t.Errorf("destino = owner:%q shared:%v; quería la cadena propia de personal-1", res.Owner, res.Shared)
	}
	if got := chainFallbackOf(t, home, "personal-1"); !chainEq(got, []string{"work-2", "work-1", "personal-deepseek"}) {
		t.Errorf("cadena propia = %v", got)
	}
	if got := chainSharedFallbackOf(t, home); !chainEq(got, []string{"work-2", "work-1"}) {
		t.Errorf("la lista COMPARTIDA no debía moverse: %v", got)
	}
	// Y el otro primario sigue heredando la compartida: la cadena de uno no es la
	// de todos, que es justo lo que antes no se podía expresar.
	rc, err := ResolveAutoChain(home, nil, "", "/work/beta")
	if err != nil {
		t.Fatal(err)
	}
	if rc.OwnChain {
		t.Errorf("work-1 no declaró cadena propia y aun así dice tenerla")
	}
	if chainIndex(rc.Fallback, "personal-deepseek") >= 0 {
		t.Errorf("el add de personal-1 se coló en la cadena de work-1: %v", rc.Fallback)
	}
}

// TestChainAddBifurcaYLoDice: la primera mutación sobre un perfil que HEREDA
// crea su cadena propia sembrada con la heredada, y lo reporta.
//
// Sembrar (y no arrancar de cero) es lo que evita que «añade uno» signifique
// «quita todos los demás». Reportarlo es lo que evita que el perfil deje de
// seguir la lista compartida sin que nadie se entere: es una consecuencia
// invisible en el yaml y solo se nota meses después, cuando un cambio en la
// compartida no llega.
func TestChainAddBifurcaYLoDice(t *testing.T) {
	home := chainHome(t, chainYAML)

	res, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"personal-deepseek"})
	if err != nil {
		t.Fatalf("ChainAdd: %v", err)
	}
	if !res.Forked {
		t.Fatal("la primera mutación sobre una cadena heredada tiene que reportar la bifurcación")
	}
	if !chainEq(res.Inherited, []string{"work-2", "work-1"}) {
		t.Errorf("Inherited = %v; tiene que decir qué lista deja de seguir", res.Inherited)
	}
	// La segunda ya no bifurca: la cadena propia ya existe.
	res2, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal"}, []string{"default"})
	if err != nil {
		t.Fatalf("ChainAdd 2: %v", err)
	}
	if res2.Forked {
		t.Error("bifurcar dos veces la misma cadena es imposible")
	}
}

// TestChainSharedSigueSiendoAlcanzable: --shared y --policy escriben la lista
// compartida, que es el comportamiento que había antes de que existieran las
// cadenas por perfil. No se retira porque sigue siendo lo que quiere quien tiene
// una sola manera de rotar para todos.
func TestChainSharedSigueSiendoAlcanzable(t *testing.T) {
	home := chainHome(t, chainYAML)

	res, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal", Shared: true}, []string{"personal-deepseek"})
	if err != nil {
		t.Fatalf("ChainAdd --shared: %v", err)
	}
	if !res.Shared || res.Owner != "" {
		t.Errorf("destino = owner:%q shared:%v; quería la lista compartida", res.Owner, res.Shared)
	}
	if got := chainSharedFallbackOf(t, home); !chainEq(got, []string{"work-2", "work-1", "personal-deepseek"}) {
		t.Errorf("lista compartida = %v", got)
	}
	cfg, _ := Load(home)
	if pc := AutoChainFor(cfg, "personal-1"); pc.Declared {
		t.Errorf("--shared no debe crear cadena propia a nadie: %+v", pc)
	}
}

// TestChainDestinoEnConflicto: --for y --shared nombran destinos distintos y no
// se resuelve eligiendo uno. Escribir en cualquiera de los dos sorprendería, y
// una cadena escrita donde no era no se nota hasta que la rotación salta mal.
func TestChainDestinoEnConflicto(t *testing.T) {
	home := chainHome(t, chainYAML)
	_, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal", For: "work-1", Shared: true}, []string{"work-2"})
	if k := chainErrKind(t, err); k != ChainErrTargetClash {
		t.Errorf("kind = %v, want ChainErrTargetClash", k)
	}
	_, err = ChainAdd(home, ChainOpts{Cwd: "/work/personal", For: "work-1", Policy: "default"}, []string{"work-2"})
	if k := chainErrKind(t, err); k != ChainErrTargetClash {
		t.Errorf("--for con --policy también son dos destinos: %v", k)
	}
}

// TestChainForOtroPerfil: --for edita la cadena de OTRO perfil sin cambiar de
// carpeta, y ajusta el gate de ESE perfil — no el del cwd. La entrada de
// allow_from que gobierna un préstamo es la del perfil que lo hace, así que
// tocar la del cwd dejaría la cadena nueva inerte.
func TestChainForOtroPerfil(t *testing.T) {
	home := chainHome(t, chainYAML)

	res, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal", For: "work-1"}, []string{"personal-deepseek"})
	if err != nil {
		t.Fatalf("ChainAdd --for: %v", err)
	}
	if res.Owner != "work-1" || res.Gate != "work-1" {
		t.Errorf("owner=%q gate=%q; quería work-1 en los dos", res.Owner, res.Gate)
	}
	if got, _ := chainAllowOf(t, home, "work-1"); chainIndex(got, "personal-deepseek") < 0 {
		t.Errorf("el gate de work-1 no se abrió: %v", got)
	}
	if got, _ := chainAllowOf(t, home, "personal-1"); chainIndex(got, "personal-deepseek") >= 0 {
		t.Errorf("se tocó el gate del cwd en vez del de --for: %v", got)
	}
	// Un --for a un perfil que no existe se rechaza antes de escribir nada.
	if _, err := ChainAdd(home, ChainOpts{Cwd: "/work/personal", For: "fantasma"}, []string{"work-2"}); err == nil {
		t.Error("--for a un perfil inexistente debe fallar")
	}
}

// TestChainCadenaVaciaEsUnaDecision: `set` no vacía, pero `rm` del último sí, y
// la entrada declarada y vacía tiene que sobrevivir a guardar y releer como «no
// presta a nadie» — no volver a «hereda», que resucitaría la rotación sola.
func TestChainCadenaVaciaEsUnaDecision(t *testing.T) {
	home := chainHome(t, chainYAML)

	if _, err := ChainRm(home, ChainOpts{Cwd: "/work/personal"}, []string{"work-2", "work-1"}); err != nil {
		t.Fatalf("ChainRm: %v", err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	pc := AutoChainFor(cfg, "personal-1")
	if !pc.Declared || len(pc.Fallback) != 0 {
		t.Fatalf("la cadena vacía no sobrevivió al round-trip: %+v", pc)
	}
	rc, err := ResolveAutoChain(home, nil, "", "/work/personal")
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Fallback) != 0 {
		t.Errorf("un perfil con cadena declarada vacía no presta a nadie: %v", rc.Fallback)
	}
}

// TestChainReset devuelve un perfil a la herencia, y se niega si no había nada
// que deshacer en vez de decir «hecho» sobre un no-op.
func TestChainReset(t *testing.T) {
	home := chainHome(t, chainYAML)
	opts := ChainOpts{Cwd: "/work/personal"}

	if k := chainErrKind(t, mustErr(t, func() error {
		_, err := ChainReset(home, opts)
		return err
	})); k != ChainErrNoChain {
		t.Errorf("reset sin cadena propia: kind = %v, want ChainErrNoChain", k)
	}

	if _, err := ChainAdd(home, opts, []string{"personal-deepseek"}); err != nil {
		t.Fatal(err)
	}
	res, err := ChainReset(home, opts)
	if err != nil {
		t.Fatalf("ChainReset: %v", err)
	}
	if !res.Reset || !chainEq(res.Fallback, []string{"work-2", "work-1"}) {
		t.Errorf("reset debe devolver la cadena heredada: %+v", res)
	}
	cfg, _ := Load(home)
	if pc := AutoChainFor(cfg, "personal-1"); pc.Entry {
		t.Errorf("la entrada tenía que desaparecer del yaml: %+v", pc)
	}
	// El gate NO se toca: quitar la cadena propia no retira permisos que se
	// declararon aparte y pueden estar puestos a propósito.
	if got, _ := chainAllowOf(t, home, "personal-1"); chainIndex(got, "personal-deepseek") < 0 {
		t.Errorf("reset no debe estrechar allow_from: %v", got)
	}
}

// TestChainPolicyLiga: ligar una política a un perfil que HEREDA no le declara
// una cadena vacía de rebote. Escribir `{policy: x}` sin `fallback` tiene que
// dejarlo heredando —de la política ligada—, porque forzar la clave le apagaría
// la rotación con un `[ok]` delante.
func TestChainPolicyLiga(t *testing.T) {
	home := chainHome(t, chainYAMLDosPoliticas)
	opts := ChainOpts{Cwd: "/work/personal"}

	res, err := ChainPolicySet(home, opts, "relajada", false)
	if err != nil {
		t.Fatalf("ChainPolicySet: %v", err)
	}
	if res.PolicyBound != "relajada" {
		t.Errorf("PolicyBound = %q", res.PolicyBound)
	}
	cfg, _ := Load(home)
	pc := AutoChainFor(cfg, "personal-1")
	if pc.Declared {
		t.Errorf("ligar política no debe declarar cadena propia: %+v", pc)
	}
	if pc.Policy != "relajada" {
		t.Errorf("Policy = %q", pc.Policy)
	}
	rc, err := ResolveAutoChain(home, nil, "", "/work/personal")
	if err != nil {
		t.Fatal(err)
	}
	if rc.Policy.Name != "relajada" || rc.Policy.Threshold != 80 || !rc.PolicyPinned {
		t.Errorf("la política ligada no llegó al supervisor: %+v", rc.Policy)
	}
	if !chainEq(rc.Fallback, []string{"work-1"}) {
		t.Errorf("hereda la cadena de la política LIGADA: %v", rc.Fallback)
	}

	// Una política inexistente se rechaza; desligar borra la entrada entera si no
	// quedaba nada más en ella.
	if _, err := ChainPolicySet(home, opts, "fantasma", false); err == nil {
		t.Error("ligar una política inexistente debe fallar")
	}
	if _, err := ChainPolicySet(home, opts, "", true); err != nil {
		t.Fatalf("desligar: %v", err)
	}
	cfg, _ = Load(home)
	if pc := AutoChainFor(cfg, "personal-1"); pc.Entry {
		t.Errorf("la entrada vacía tenía que desaparecer: %+v", pc)
	}
}

// TestChainPolicyRotaYCadenaSobreviven: una política ligada convive con una
// cadena propia, y ninguna de las dos borra a la otra al guardar.
func TestChainPolicyRotaYCadenaSobreviven(t *testing.T) {
	home := chainHome(t, chainYAMLDosPoliticas)
	opts := ChainOpts{Cwd: "/work/personal"}
	if _, err := ChainPolicySet(home, opts, "relajada", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ChainSet(home, opts, []string{"personal-deepseek"}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := Load(home)
	pc := AutoChainFor(cfg, "personal-1")
	if !pc.Declared || !chainEq(pc.Fallback, []string{"personal-deepseek"}) || pc.Policy != "relajada" {
		t.Fatalf("cadena y política ligada tienen que convivir: %+v", pc)
	}
}

// mustErr ejecuta fn y exige que falle.
func mustErr(t *testing.T, fn func() error) error {
	t.Helper()
	err := fn()
	if err == nil {
		t.Fatal("se esperaba error")
	}
	return err
}
