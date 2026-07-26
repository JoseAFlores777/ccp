package core

import (
	"fmt"
	"os"
	"regexp"
)

// profile_rename.go — renombrar un perfil. El nombre no es solo una etiqueta:
// es la clave de `profiles` en ccp.yaml, el valor al que apunta cada regla, el
// nombre del directorio que guarda la api_key y el login (cc-home), y el
// `from`/`to` de cada marcador de handoff. Cambiarlo a mano en uno solo deja el
// estado incoherente de una forma silenciosa: una regla huérfana no falla,
// resuelve a `default`, así que el usuario descubre el problema cuando Claude
// Code arranca con la cuenta equivocada.

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

// ProfileRename renombra un perfil moviendo TODO su estado: el directorio
// (api_key + cc-home + overlay), la entrada de ccp.yaml, las reglas que lo
// apuntan, los artefactos authored de scope profile y los marcadores de
// handoffs.yaml (activos e históricos). Termina regenerando el overlay, porque
// el CLAUDE.md del cc-home contiene la ruta absoluta —con el nombre viejo— del
// overlay del perfil.
//
// Orden y rollback: primero mueve el directorio (la operación que más razones
// tiene para fallar: permisos, disco, un resto de un rename anterior) con el
// config todavía íntegro; si después no puede persistir ccp.yaml, devuelve el
// directorio a su sitio. Un ccp.yaml que nombra un perfil sin directorio es el
// peor estado posible: el login y la key siguen en disco pero ccp ya no los
// encuentra.
//
// Lo que NO puede arreglar: una terminal viva que exportó CCP_PROFILE=<viejo>.
// Eso lo resuelve el usuario con `ccp use <nuevo>`; el CLI lo recuerda.
func ProfileRename(home, oldName, newName string) error {
	if oldName == "" || oldName == "default" {
		return fmt.Errorf("no se puede renombrar el perfil reservado %q", oldName)
	}
	if newName == "default" {
		return fmt.Errorf("%q es un nombre reservado", newName)
	}
	if !validProfileName(newName) {
		return fmt.Errorf("nombre de perfil inválido: %q (usa letras, números, '.', '_' o '-')", newName)
	}
	c, err := Load(home)
	if err != nil {
		return err
	}
	p, exists := c.Profiles[oldName]
	if !exists {
		return fmt.Errorf("no existe el perfil %q", oldName)
	}
	if _, taken := c.Profiles[newName]; taken {
		return fmt.Errorf("el perfil %q ya existe", newName)
	}
	oldDir, newDir := profileDirPath(home, oldName), profileDirPath(home, newName)
	// Un directorio suelto con el nombre destino (p. ej. un perfil borrado del
	// yaml pero no del disco) se respeta: pisarlo perdería su login y su key.
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("ya existe el directorio %s; muévelo o elimínalo antes de renombrar", newDir)
	}

	movido := false
	if _, err := os.Stat(oldDir); err == nil {
		if err := os.Rename(oldDir, newDir); err != nil {
			return fmt.Errorf("no se pudo mover %s -> %s: %w", oldDir, newDir, err)
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

	delete(c.Profiles, oldName)
	c.Profiles[newName] = p
	for i, r := range c.Rules {
		if r.Profile == oldName {
			c.Rules[i].Profile = newName
		}
	}
	for i, a := range c.Authored {
		if a.Scope == "profile" && a.Profile == oldName {
			c.Authored[i].Profile = newName
		}
	}
	if err := Save(home, c); err != nil {
		revertirDir()
		return err
	}

	if err := renameInHandoffs(home, oldName, newName); err != nil {
		// Deshacer el config también: dejarlo renombrado con los marcadores
		// apuntando al nombre viejo rompería `handoff end` (buscaría un perfil
		// que ya no existe) sin decir por qué.
		c.Profiles[oldName] = p
		delete(c.Profiles, newName)
		for i, r := range c.Rules {
			if r.Profile == newName {
				c.Rules[i].Profile = oldName
			}
		}
		for i, a := range c.Authored {
			if a.Scope == "profile" && a.Profile == newName {
				c.Authored[i].Profile = oldName
			}
		}
		if serr := Save(home, c); serr != nil {
			revertirDir()
			return fmt.Errorf("%w (además falló revertir ccp.yaml: %v)", err, serr)
		}
		revertirDir()
		return err
	}

	// El cc-home guarda rutas absolutas al overlay del perfil, con el nombre
	// viejo dentro. Regenerar es lo que las pone al día; si falla, el rename ya
	// es válido, así que se reporta sin deshacerlo.
	if movido {
		if err := ProfileSync(home, newName); err != nil {
			return fmt.Errorf("perfil renombrado, pero no se pudo regenerar su config: %w", err)
		}
	}
	return nil
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
