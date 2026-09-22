package core

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	yaml "github.com/goccy/go-yaml"
)

// profile_rename.go — renombrar un perfil. El nombre no es solo una etiqueta:
// es la clave de `profiles` en ccp.yaml, el valor al que apunta cada regla, el
// nombre del directorio que guarda la api_key y el login (cc-home), el
// `from`/`to` de cada marcador de handoff y cada mención en `auto_handoff`
// (cadenas, allow_from y hooks). Cambiarlo a mano en uno solo deja el estado
// incoherente de una forma silenciosa: una regla huérfana no falla, resuelve a
// `default`, así que el usuario descubre el problema cuando Claude Code arranca
// con la cuenta equivocada. En `auto_handoff` es igual de mudo: la cuenta deja
// de usarse como préstamo, el gate allow_from ya no la reconoce ni como primario
// ni como destino, y sus sensores desaparecen en la siguiente regeneración del
// cc-home.

// profileNameRe es el conjunto de nombres aceptados al renombrar: el nombre se
// usa como componente de ruta (<home>/profiles/<name>), así que un `/` o un
// `..` escaparían del directorio de perfiles.
var profileNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// validProfileName rechaza los nombres que no pueden ser un directorio seguro.
// Solo lo aplica el rename: endurecer `profile add` cambiaría el contrato de un
// comando ya publicado y es una decisión aparte.
func validProfileName(name string) bool {
	return name != ".." && profileNameRe.MatchString(name)
}

// RenameResult es lo que el rename no puede arreglar solo y el usuario tiene que
// saber. Vale también junto a un error: si el directorio ya se movió y lo que
// falló fue la regeneración, el login se perdió igual.
type RenameResult struct {
	// Relogin: el perfil era official y tenía sesión. Claude Code guarda su
	// credencial en el Llavero con un nombre que sale de la ruta del cc-home
	// (`Claude Code-credentials-<sha256(dir)[:8]>`, ADR 0016, M4), y el rename
	// acaba de cambiar esa ruta: hay que volver a hacer /login. ccp no mueve la
	// credencial, porque tocaría secretos del Llavero. Tampoco distingue Linux,
	// donde la credencial vive en el cc-home y sí viajaría: no está medido, y un
	// aviso de más cuesta un /login mientras que uno de menos deja un perfil
	// que no entra sin decir por qué.
	Relogin bool
}

// ProfileRename renombra un perfil moviendo TODO su estado: el directorio
// (api_key + cc-home + overlay + datos de Desktop), la entrada de ccp.yaml, las
// reglas que lo apuntan, los artefactos authored de scope profile, sus
// menciones en `auto_handoff` y los marcadores de handoffs.yaml (activos e
// históricos). Termina regenerando el overlay, porque el CLAUDE.md del cc-home
// contiene la ruta absoluta —con el nombre viejo— del overlay del perfil.
//
// Orden y rollback: primero mueve el directorio (la operación que más razones
// tiene para fallar: permisos, disco, un resto de un rename anterior) con el
// config todavía íntegro; si después no puede persistir ccp.yaml, devuelve el
// directorio a su sitio. Un ccp.yaml que nombra un perfil sin directorio es el
// peor estado posible: el login y la key siguen en disco pero ccp ya no los
// encuentra. Todo ccp.yaml cambia en UN solo Save, así que no hay estado
// intermedio con las reglas renombradas y la cadena de rotación sin renombrar.
//
// Lo que NO puede arreglar: una terminal viva que exportó CCP_PROFILE=<viejo>.
// Eso lo resuelve el usuario con `ccp use <nuevo>`; el CLI lo recuerda. Tampoco
// toca el lanzador de Desktop (~/Applications/Claude (<viejo>).app): su
// manifiesto nombra el perfil viejo, pero rehacerlo es cosa de `ccp desktop
// app`, que sabe comprobar antes que su ventana no esté abierta; el CLI avisa.
// Tampoco puede mover el login de un perfil official: Claude Code guarda la
// credencial en el Llavero con un nombre que sale de la ruta del cc-home, y esa
// ruta es justo lo que el rename cambia. ccp no toca el Llavero; lo devuelve en
// RenameResult para que cada front-end pida el /login (B7).
func ProfileRename(home, oldName, newName string) (RenameResult, error) {
	if oldName == "" || oldName == "default" {
		return RenameResult{}, fmt.Errorf("no se puede renombrar el perfil reservado %q", oldName)
	}
	if newName == "default" {
		return RenameResult{}, fmt.Errorf("%q es un nombre reservado", newName)
	}
	if !validProfileName(newName) {
		return RenameResult{}, fmt.Errorf("nombre de perfil inválido: %q (usa letras, números, '.', '_' o '-')", newName)
	}
	c, err := Load(home)
	if err != nil {
		return RenameResult{}, err
	}
	if _, exists := c.Profiles[oldName]; !exists {
		return RenameResult{}, fmt.Errorf("no existe el perfil %q", oldName)
	}
	if _, taken := c.Profiles[newName]; taken {
		return RenameResult{}, fmt.Errorf("el perfil %q ya existe", newName)
	}
	// Un nombre destino que auto_handoff ya menciona es un resto, casi siempre
	// de un perfil borrado (`profile rm` no limpia el bloque). Renombrar encima
	// se lo regalaría al perfil renombrado: entraría en cadenas en las que no
	// estaba y heredaría un allow_from ajeno, así que un gate que hoy le deniega
	// préstamos se abriría sin que nadie lo decidiera. Mismo criterio que con el
	// directorio suelto de abajo: se respeta y se pide limpiarlo antes.
	if where := autoHandoffMentions(c.AutoHandoff, newName); len(where) > 0 {
		return RenameResult{}, fmt.Errorf("auto_handoff ya menciona %q (%s), seguramente restos de un perfil borrado: quita esas menciones de ccp.yaml antes de renombrar o el perfil renombrado las heredaría",
			newName, strings.Join(where, ", "))
	}
	oldDir, newDir := profileDirPath(home, oldName), profileDirPath(home, newName)
	// Un directorio suelto con el nombre destino (p. ej. un perfil borrado del
	// yaml pero no del disco) se respeta: pisarlo perdería su login y su key.
	if _, err := os.Stat(newDir); err == nil {
		return RenameResult{}, fmt.Errorf("ya existe el directorio %s; muévelo o elimínalo antes de renombrar", newDir)
	}

	// Se mira ANTES de mover: después, el .claude.json ya está en la ruta nueva.
	res := RenameResult{Relogin: c.Profiles[oldName].Type == "official" && HasLogin(home, oldName)}

	movido := false
	if _, err := os.Stat(oldDir); err == nil {
		if err := os.Rename(oldDir, newDir); err != nil {
			return RenameResult{}, fmt.Errorf("no se pudo mover %s -> %s: %w", oldDir, newDir, err)
		}
		movido = true
	}
	// revertirDir deshace el movimiento; se llama en cada salida por error
	// posterior para no dejar el directorio a medio camino.
	revertirDir := func() {
		if movido {
			_ = os.Rename(newDir, oldDir)
		}
	}

	if err := Save(home, renamedConfig(c, oldName, newName)); err != nil {
		revertirDir()
		return RenameResult{}, err
	}

	if err := renameInHandoffs(home, oldName, newName); err != nil {
		// Deshacer el config también: dejarlo renombrado con los marcadores
		// apuntando al nombre viejo rompería `handoff end` (buscaría un perfil
		// que ya no existe) sin decir por qué. renamedConfig no tocó c, así que
		// deshacer es guardarlo tal como se leyó.
		if serr := Save(home, c); serr != nil {
			revertirDir()
			return RenameResult{}, fmt.Errorf("%w (además falló revertir ccp.yaml: %v)", err, serr)
		}
		revertirDir()
		return RenameResult{}, err
	}

	// El cc-home guarda rutas absolutas al overlay del perfil, con el nombre
	// viejo dentro. Regenerar es lo que las pone al día; si falla, el rename ya
	// es válido, así que se reporta sin deshacerlo, y con res: el login se
	// perdió igual.
	if movido {
		if err := ProfileSync(home, newName); err != nil {
			return res, fmt.Errorf("perfil renombrado, pero no se pudo regenerar su config: %w", err)
		}
	}
	return res, nil
}

// renamedConfig devuelve una COPIA de c con oldName cambiado por newName en
// todo ccp.yaml: la clave de `profiles`, las reglas, los authored de scope
// profile, el bloque `auto_handoff` y los comentarios que colgaban de las
// claves renombradas.
//
// Que c quede intacto es lo que hace exacto el rollback: deshacer es volver a
// guardar c. Invertir el cambio a mano (newName → oldName) no sabría distinguir
// lo que se renombró de lo que ya se llamaba newName, como una regla huérfana
// de un perfil borrado con ese nombre.
//
// Solo se copia lo que cambia. El resto (Extra, Defaults, los Extra del bloque
// auto) se comparte con c: lo único que Save les hace es quitar claves
// conocidas, y eso ya lo hizo Load.
func renamedConfig(c *Config, oldName, newName string) *Config {
	n := *c
	n.Profiles = make(map[string]Profile, len(c.Profiles))
	for name, p := range c.Profiles {
		if name == oldName {
			name = newName
		}
		n.Profiles[name] = p
	}
	n.Rules = slices.Clone(c.Rules)
	for i, r := range n.Rules {
		if r.Profile == oldName {
			n.Rules[i].Profile = newName
		}
	}
	n.Authored = slices.Clone(c.Authored)
	for i, a := range n.Authored {
		if a.Scope == "profile" && a.Profile == oldName {
			n.Authored[i].Profile = newName
		}
	}
	n.AutoHandoff = renamedAutoHandoff(c.AutoHandoff, oldName, newName)
	n.comments = renamedComments(c.comments, [][2]string{
		{commentPath("profiles", oldName), commentPath("profiles", newName)},
		{commentPath("auto_handoff", "allow_from", oldName), commentPath("auto_handoff", "allow_from", newName)},
		{commentPath("auto_handoff", "chains", oldName), commentPath("auto_handoff", "chains", newName)},
	})
	return &n
}

// renamedAutoHandoff es la parte de renamedConfig que toca `auto_handoff`: el
// fallback de cada política, las claves y las listas de `chains` y de
// `allow_from`, y hooks.
// Los nombres de política no se tocan: no son perfiles.
//
// No deduplica. ProfileRename ya rechazó un newName que el bloque mencionara,
// así que cambiar un nombre por otro no puede crear un duplicado que no
// estuviera ya, y uno que el usuario escribiera a mano se respeta tal cual.
func renamedAutoHandoff(a *AutoHandoff, oldName, newName string) *AutoHandoff {
	if a == nil {
		return nil
	}
	n := *a
	if a.Policies != nil {
		n.Policies = make(map[string]AutoPolicy, len(a.Policies))
		for name, pol := range a.Policies {
			pol.Fallback = renamedList(pol.Fallback, oldName, newName)
			n.Policies[name] = pol
		}
	}
	// Las cadenas propias se renombran en las DOS posiciones: la clave (el dueño)
	// y los nombres de dentro (los destinos). Renombrar solo una deja al perfil
	// renombrado sin su cadena —volvería a heredar sin que nadie lo dijera— o con
	// una cadena que apunta a un perfil que ya no existe, que es un error al
	// resolver justo cuando toca rotar.
	if a.Chains != nil {
		n.Chains = make(map[string]AutoChain, len(a.Chains))
		for owner, entry := range a.Chains {
			if owner == oldName {
				owner = newName
			}
			entry.Fallback = renamedList(entry.Fallback, oldName, newName)
			n.Chains[owner] = entry
		}
	}
	if a.AllowFrom != nil {
		n.AllowFrom = make(map[string][]string, len(a.AllowFrom))
		for primary, allowed := range a.AllowFrom {
			if primary == oldName {
				primary = newName
			}
			n.AllowFrom[primary] = renamedList(allowed, oldName, newName)
		}
	}
	n.Hooks = renamedList(a.Hooks, oldName, newName)
	return &n
}

// autoHandoffMentions dice dónde nombra `auto_handoff` a name, como rutas del
// yaml y en orden estable (va a un mensaje de error).
//
// Compara igual que los lectores del bloque, que es lo que decide si una
// mención cuenta: los valores de las listas recortando espacios (Effective,
// ResolveAutoChain, AutoHooksEnabled) y las claves de allow_from tal cual
// (AutoGateFor). renamedAutoHandoff usa las mismas reglas, así que renombra
// exactamente lo que aquí se detecta.
func autoHandoffMentions(a *AutoHandoff, name string) []string {
	if a == nil {
		return nil
	}
	var where []string
	for _, pol := range slices.Sorted(maps.Keys(a.Policies)) {
		if listMentions(a.Policies[pol].Fallback, name) {
			where = append(where, "policies."+pol+".fallback")
		}
	}
	for _, owner := range slices.Sorted(maps.Keys(a.Chains)) {
		if owner == name || listMentions(a.Chains[owner].Fallback, name) {
			where = append(where, "chains."+owner)
		}
	}
	for _, primary := range slices.Sorted(maps.Keys(a.AllowFrom)) {
		if primary == name || listMentions(a.AllowFrom[primary], name) {
			where = append(where, "allow_from."+primary)
		}
	}
	if listMentions(a.Hooks, name) {
		where = append(where, "hooks")
	}
	return where
}

func listMentions(list []string, name string) bool {
	return slices.ContainsFunc(list, func(s string) bool { return strings.TrimSpace(s) == name })
}

// renamedList devuelve una copia de list con oldName cambiado por newName.
func renamedList(list []string, oldName, newName string) []string {
	out := slices.Clone(list)
	for i, s := range out {
		if strings.TrimSpace(s) == oldName {
			out[i] = newName
		}
	}
	return out
}

// commentPath es la ruta con la que goccy indexa un comentario de ccp.yaml: la
// misma que produce CommentToMap al cargar, comillas incluidas cuando la clave
// lleva un punto.
func commentPath(keys ...string) string {
	b := (&yaml.PathBuilder{}).Root()
	for _, k := range keys {
		b = b.Child(k)
	}
	return b.Build().String()
}

// renamedComments devuelve una copia de cm con los comentarios de cada clave
// renombrada, y de todo lo que cuelga de ella, movidos a su ruta nueva.
//
// Sin esto Save los descarta en silencio: goccy recoloca cada comentario por su
// ruta y la vieja ya no existe. Pesa sobre todo en allow_from, que es donde uno
// deja escrito por qué un cliente no presta a cierta cuenta. Los comentarios de
// los elementos de lista (reglas, fallback, hooks) no necesitan nada: el
// elemento se renombra en su sitio y su ruta, que es su índice, no cambia.
//
// Es un extra, así que nunca puede hacer fallar el rename: una ruta nueva que
// goccy no supiera leer rompería el Save entero (WithComment la parsea), y en
// ese caso el comentario se pierde como se perdía antes.
func renamedComments(cm yaml.CommentMap, moves [][2]string) yaml.CommentMap {
	if len(cm) == 0 {
		return cm
	}
	out := make(yaml.CommentMap, len(cm))
	for path, comments := range cm {
		for _, mv := range moves {
			from, to := mv[0], mv[1]
			if path != from && !strings.HasPrefix(path, from+".") && !strings.HasPrefix(path, from+"[") {
				continue
			}
			moved := to + strings.TrimPrefix(path, from)
			if _, err := yaml.PathString(moved); err == nil {
				path = moved
			}
			break
		}
		out[path] = comments
	}
	return out
}

// renameInHandoffs reescribe from/to en los marcadores activos y archivados.
// Va por UpdateHandoffs (lectura-modificación-escritura bajo un solo flock)
// para no pisar el marcador que otra terminal esté creando a la vez.
func renameInHandoffs(home, oldName, newName string) error {
	return UpdateHandoffs(home, func(h *Handoffs) error {
		tocado := false
		for i, m := range h.Active {
			if m.From == oldName {
				h.Active[i].From = newName
				tocado = true
			}
			if m.To == oldName {
				h.Active[i].To = newName
				tocado = true
			}
		}
		for i, a := range h.Archived {
			if a.From == oldName {
				h.Archived[i].From = newName
				tocado = true
			}
			if a.To == oldName {
				h.Archived[i].To = newName
				tocado = true
			}
		}
		if !tocado {
			// Sin marcadores que tocar no se reescribe el archivo: así un home
			// sin handoffs.yaml no lo estrena por un rename.
			return ErrHandoffsUnchanged
		}
		return nil
	})
}
