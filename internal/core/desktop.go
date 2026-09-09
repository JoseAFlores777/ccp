package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// desktop.go — instancias aisladas de Claude Desktop, una por perfil.
//
// El aislamiento son DOS cosas, y la segunda es la que nadie ve:
//
//  1. `--user-data-dir` aísla la IDENTIDAD DE LA APP: sesión, tokens OAuth,
//     `claude_desktop_config.json` (MCP) y el estado de Cowork viven todos
//     dentro de ese directorio.
//
//  2. `CLAUDE_CONFIG_DIR` en el ENTORNO DEL PROCESO Desktop aísla el Code tab.
//     Sin esto, (1) no basta: el Code tab NO lee el user-data-dir, lee
//     `process.env.CLAUDE_CONFIG_DIR` (y cae a `~/.claude` si está vacío), y de
//     ahí saca `projects/` y `sessions/`. Dos ventanas con cuentas distintas
//     compartirían credenciales de CLI, historial y agentes. El hijo hereda el
//     entorno del proceso Desktop, así que inyectarlo en el lanzamiento es lo
//     que separa las dos mitades de la ventana.
//
// (2) obliga a (3), que es la parte incómoda: Desktop valida lo que llama el
// «co-writable boundary» y RECHAZA symlinks en cualquier componente NO-HOJA
// bajo el config root. seedCCHome deja exactamente esa forma
// (`cc-home/commands -> ~/.claude/commands`), así que apuntar Desktop a un
// cc-home recién sembrado falla. MirrorForDesktop lo arregla, y no cambiando
// seedCCHome: el oráculo bash afirma `[[ -L "$cch/plugins" ]]`, o sea que la
// forma con symlinks de directorio ES el contrato de `profile add`. La
// conversión es opt-in y por perfil.

// desktopMirrorItems son las entradas de cc-home que seedCCHome symlinkea y que
// por tanto hay que convertir antes de que Desktop escriba en el config root.
// Mismo orden y mismo conjunto que seedCCHome a propósito: si mañana alguien
// añade un item allí, este es el otro sitio que tiene que tocar.
var desktopMirrorItems = []string{"plugins", "commands", "agents", "skills"}

// DesktopDataDir devuelve el `--user-data-dir` del perfil.
//
// Para `default` devuelve "": el perfil default ES el login normal del usuario,
// y su instancia de Desktop es la de siempre (`~/Library/Application
// Support/Claude`). Reubicarla sería mover la sesión que el usuario ya tiene,
// no crear una nueva. Espeja que `default` tampoco tiene cc-home.
func DesktopDataDir(home, name string) string {
	if name == "default" {
		return ""
	}
	return filepath.Join(home, "profiles", name, "desktop")
}

// DesktopEligible decide si un perfil puede alojar una instancia de Desktop.
//
// Solo `default` y los `official`. Un perfil de proveedor (deepseek/kimi/glm)
// no tiene equivalente: Claude Desktop habla siempre con Anthropic y no lee
// ANTHROPIC_BASE_URL, así que inyectarle el entorno del perfil dejaría el Code
// tab apuntando a DeepSeek mientras la mitad de chat de la MISMA ventana sigue
// siendo Anthropic sin autenticar. Ese híbrido es peor que no soportarlo: el
// usuario no tiene forma de saber cuál de las dos mitades está mirando.
func DesktopEligible(cfg *Config, name string) error {
	if name == "default" {
		return nil
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("no existe el perfil %q", name)
	}
	if p.Type != "official" {
		return fmt.Errorf(
			"el perfil %q es de tipo %q: Claude Desktop solo funciona con cuentas de Anthropic (perfiles official o default)",
			name, p.Type)
	}
	return nil
}

// MirrorForDesktop deja el cc-home del perfil en una forma que Desktop acepta y
// devuelve las entradas que tuvo que convertir (vacío = ya estaba listo).
//
// La regla de Desktop exime la HOJA del path que valida (recorre
// `c[0..len-2]`), así que un symlink a nivel de ARCHIVO sí pasa. De ahí el
// espejo: directorios reales replicando el árbol de `~/.claude`, y symlinks
// solo en los archivos finales. Se conserva el compartido en vivo con el global
// —editar un comando en `~/.claude/commands` se ve desde el perfil— sin dejar
// un solo symlink en posición no-hoja.
//
// Es idempotente y se puede volver a llamar para re-espejar cuando el usuario
// añade archivos al global.
// DesktopMirrorPending informa qué entradas convertiría MirrorForDesktop, sin
// tocar disco. Existe para que `--dry-run` sea de verdad seco: sin esto, la
// única forma de saber qué se iba a convertir era convertirlo, que es
// exactamente lo que un dry-run promete no hacer.
func DesktopMirrorPending(home, name string) ([]string, error) {
	if name == "default" {
		return nil, nil
	}
	cch := ccHomePath(home, name)
	if _, err := os.Stat(cch); err != nil {
		return nil, fmt.Errorf("el perfil %q no tiene cc-home (%s): %w", name, cch, err)
	}

	var pending []string
	for _, item := range desktopMirrorItems {
		fi, err := os.Lstat(filepath.Join(cch, item))
		switch {
		case err == nil && fi.Mode()&os.ModeSymlink != 0:
			pending = append(pending, item) // symlink de directorio: hay que convertirlo
		case err != nil:
			pending = append(pending, item) // no existe: hay que crearlo
		}
	}
	return pending, nil
}

func MirrorForDesktop(home, name string) ([]string, error) {
	if name == "default" {
		// default usa ~/.claude directamente: no hay cc-home que arreglar.
		return nil, nil
	}
	cch := ccHomePath(home, name)
	if _, err := os.Stat(cch); err != nil {
		return nil, fmt.Errorf("el perfil %q no tiene cc-home (%s): %w", name, cch, err)
	}

	src, err := claudeSrcDir()
	if err != nil {
		return nil, err
	}

	var converted []string
	for _, item := range desktopMirrorItems {
		dst := filepath.Join(cch, item)
		srcItem := filepath.Join(src, item)

		fi, err := os.Lstat(dst)
		switch {
		case err == nil && fi.Mode()&os.ModeSymlink != 0:
			// El caso que rompe Desktop: symlink de DIRECTORIO sembrado por
			// seedCCHome. Se sustituye por el espejo. Quitar el symlink no
			// borra nada del global: apunta fuera del perfil.
			target, rerr := os.Readlink(dst)
			if rerr != nil {
				return converted, fmt.Errorf("no se pudo leer el symlink %s: %w", dst, rerr)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(dst), target)
			}
			if err := os.Remove(dst); err != nil {
				return converted, fmt.Errorf("no se pudo quitar el symlink %s: %w", dst, err)
			}
			if err := mirrorTree(target, dst); err != nil {
				return converted, err
			}
			converted = append(converted, item)

		case err == nil:
			// Ya es un directorio real (o un archivo del usuario): re-espejar
			// para recoger lo nuevo del global, sin pisar nada suyo.
			if fi.IsDir() {
				if _, serr := os.Stat(srcItem); serr == nil {
					if err := mirrorTree(srcItem, dst); err != nil {
						return converted, err
					}
				}
			}

		default:
			// No existe. Si el global lo tiene, se espeja; si no, se crea vacío
			// y REAL. Crear `agents/` vacío no es un detalle: es lo que hace
			// que cada perfil pueda tener sus propios agentes sin heredar los
			// del global (que en muchas máquinas ni existe).
			if _, serr := os.Stat(srcItem); serr == nil {
				if err := mirrorTree(srcItem, dst); err != nil {
					return converted, err
				}
				converted = append(converted, item)
				continue
			}
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return converted, fmt.Errorf("no se pudo crear %s: %w", dst, err)
			}
			converted = append(converted, item)
		}
	}
	return converted, nil
}

// mirrorTree replica `src` dentro de `dst`: directorios REALES, archivos como
// symlink absoluto al original.
//
// Dos reglas gobiernan todo lo demás:
//
//   - Si una entrada YA existe en dst, no se toca. Eso da idempotencia y, de
//     regalo, semántica de override: un archivo real que el usuario ponga en el
//     perfil gana sobre el del global y sobrevive a los re-espejados.
//   - Los symlinks colgados que apuntan DENTRO de src se podan. Son restos de
//     un espejo anterior cuyo original se borró; dejarlos haría que el perfil
//     arrastrara comandos fantasma para siempre. Solo se poda lo que es
//     nuestro: un symlink del usuario a cualquier otro sitio se respeta.
func mirrorTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("no se pudo leer %s: %w", src, err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", dst, err)
	}

	if err := pruneDangling(src, dst); err != nil {
		return err
	}

	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())

		// Un symlink en el origen se resuelve para decidir si es dir o archivo:
		// `~/.claude/plugins/x -> /otro/sitio` debe espejarse como directorio,
		// no como un symlink que volvería a ser componente no-hoja.
		info, serr := os.Stat(srcPath)
		if serr != nil {
			continue // colgado o inaccesible en el origen: no es cosa nuestra
		}

		if info.IsDir() {
			if err := mirrorTree(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if _, err := os.Lstat(dstPath); err == nil {
			continue // ya existe (override del perfil o espejo previo)
		}
		if err := os.Symlink(srcPath, dstPath); err != nil {
			return fmt.Errorf("no se pudo enlazar %s -> %s: %w", dstPath, srcPath, err)
		}
	}
	return nil
}

// pruneDangling borra de dst los symlinks colgados que apuntan dentro de src.
// Ver mirrorTree para el porqué de la restricción a src.
func pruneDangling(src, dst string) error {
	entries, err := os.ReadDir(dst)
	if err != nil {
		return nil // dst recién creado o ilegible: nada que podar
	}
	for _, e := range entries {
		p := filepath.Join(dst, e.Name())
		fi, lerr := os.Lstat(p)
		if lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if _, serr := os.Stat(p); serr == nil {
			continue // apunta a algo vivo
		}
		target, rerr := os.Readlink(p)
		if rerr != nil || !strings.HasPrefix(target, src+string(os.PathSeparator)) {
			continue // no lo hicimos nosotros
		}
		if err := os.Remove(p); err != nil {
			return fmt.Errorf("no se pudo podar el enlace colgado %s: %w", p, err)
		}
	}
	return nil
}

// claudeSrcDir devuelve el ~/.claude global (o CCP_CLAUDE_SRC), igual que
// seedCCHome. Existe aparte porque seedCCHome lo resuelve inline y aquí hace
// falta el mismo valor sin duplicar la regla de precedencia.
func claudeSrcDir() (string, error) {
	if s := os.Getenv("CCP_CLAUDE_SRC"); s != "" {
		return s, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo determinar HOME: %w", err)
	}
	return filepath.Join(userHome, ".claude"), nil
}
