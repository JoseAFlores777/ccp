package core

// cfg.go — profile config (overlay + merge) para ccp, port de lib/cfg.sh.
//
// Cada perfil official|deepseek tiene un cc-home (= CLAUDE_CONFIG_DIR). Su
// "profile config" es una capa baseline:
//   - instrucciones: overlay/CLAUDE.md            -> @import en cc-home/CLAUDE.md
//   - settings:      overlay/settings.overlay.json -> deep-merge sobre el global
//                                                     => cc-home/settings.json
//
// El merge JSON es Go puro (sin jq): replica `. * $x` de jq — objetos se
// fusionan recursivamente, arrays y escalares se REEMPLAZAN por el overlay.
// MergeJSON es la superficie exportada que el issue #9 reutiliza.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// --- rutas del overlay (espejan los helpers de lib/cfg.sh) ---

func cfgOverlayDir(home, name string) string {
	return filepath.Join(home, "profiles", name, "overlay")
}

func cfgInstrFile(home, name string) string {
	return filepath.Join(cfgOverlayDir(home, name), "CLAUDE.md")
}

func cfgSettingsFile(home, name string) string {
	return filepath.Join(cfgOverlayDir(home, name), "settings.overlay.json")
}

// MergeJSON fusiona base ⊕ overlay y devuelve el JSON resultante (indentado a 2
// espacios, con newline final). Replica la semántica de jq `. * $x`:
//   - dos objetos se fusionan recursivamente clave a clave;
//   - cuando ambos lados tienen la misma clave y ambos valores son objetos,
//     se recursa; en cualquier otro caso (arrays, escalares, tipos mixtos) el
//     valor del overlay REEMPLAZA al de base;
//   - claves presentes solo en un lado se conservan.
//
// base vacío (nil o "") se trata como objeto vacío. Es la superficie compartida
// que reutiliza el motor de instrucciones (#9). No tiene dependencias externas.
func MergeJSON(base, overlay []byte) ([]byte, error) {
	bv, err := unmarshalJSONValue(base)
	if err != nil {
		return nil, fmt.Errorf("base JSON inválido: %w", err)
	}
	// Documento overlay completamente vacío (archivo en blanco) => conserva
	// base tal cual. Esto es distinto de un `null` explícito DENTRO de un
	// objeto, que sí reemplaza (lo maneja mergeValues).
	if len(bytes.TrimSpace(overlay)) == 0 {
		out, err := marshalIndent(bv)
		if err != nil {
			return nil, fmt.Errorf("no se pudo serializar el merge: %w", err)
		}
		return out, nil
	}
	ov, err := unmarshalJSONValue(overlay)
	if err != nil {
		return nil, fmt.Errorf("overlay JSON inválido: %w", err)
	}
	merged := mergeValues(bv, ov)
	out, err := marshalIndent(merged)
	if err != nil {
		return nil, fmt.Errorf("no se pudo serializar el merge: %w", err)
	}
	return out, nil
}

// unmarshalJSONValue decodifica data a un valor genérico. Entrada vacía => nil
// (que mergeValues trata como "ausente"). Usa json.Number para no perder
// precisión de enteros grandes en el round-trip.
func unmarshalJSONValue(data []byte) (any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// mergeValues aplica la regla de merge a dos valores ya decodificados. Solo
// recursa cuando AMBOS son objetos JSON; de lo contrario gana el overlay,
// incluido un `null` explícito (espeja jq `. * $x`, donde una clave con valor
// null en $x reemplaza). El caso "overlay ausente por completo" lo filtra
// MergeJSON antes de llamar aquí.
func mergeValues(base, overlay any) any {
	bm, bok := base.(map[string]any)
	om, ook := overlay.(map[string]any)
	if !bok || !ook {
		// Al menos uno no es objeto: el overlay reemplaza (arrays/escalares).
		return overlay
	}
	out := make(map[string]any, len(bm)+len(om))
	for k, v := range bm {
		out[k] = v
	}
	for k, ov := range om {
		if bv, exists := out[k]; exists {
			out[k] = mergeValues(bv, ov)
		} else {
			out[k] = ov
		}
	}
	return out
}

// marshalIndent serializa v con indentación de 2 espacios y newline final.
// nil => "{}" (un overlay y base vacíos producen un objeto vacío, no "null").
func marshalIndent(v any) ([]byte, error) {
	if v == nil {
		return []byte("{}\n"), nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CfgInitOverlay crea overlay/ con archivos vacíos si faltan (idempotente).
// CLAUDE.md vacío; settings.overlay.json con "{}\n".
func CfgInitOverlay(home, name string) error {
	d := cfgOverlayDir(home, name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear overlay de %q: %w", name, err)
	}
	instr := filepath.Join(d, "CLAUDE.md")
	if !pathExists(instr) {
		if err := os.WriteFile(instr, []byte{}, 0o644); err != nil {
			return fmt.Errorf("no se pudo crear %s: %w", instr, err)
		}
	}
	settings := filepath.Join(d, "settings.overlay.json")
	if !pathExists(settings) {
		if err := os.WriteFile(settings, []byte("{}\n"), 0o644); err != nil {
			return fmt.Errorf("no se pudo crear %s: %w", settings, err)
		}
	}
	return nil
}

// CfgValidateJSON devuelve error si el archivo no existe o no contiene JSON
// válido. A diferencia del bash (que era no-op sin jq), aquí siempre validamos.
func CfgValidateJSON(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("no se pudo leer %s: %w", file, err)
	}
	if !json.Valid(data) {
		return fmt.Errorf("JSON inválido en %s", file)
	}
	return nil
}

// cfgWriteClaudeMD escribe cc-home/CLAUDE.md: header + @import del global (si
// existe <src>/CLAUDE.md) + @import del overlay del perfil. Si cc-home/CLAUDE.md
// es un symlink viejo se elimina antes de escribir (escribir sobre el symlink
// corrompería el archivo global apuntado).
func cfgWriteClaudeMD(home, name, src string) error {
	cch := ccHomePath(home, name)
	overlay := cfgInstrFile(home, name)
	if err := os.MkdirAll(cch, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear cc-home de %q: %w", name, err)
	}
	dst := filepath.Join(cch, "CLAUDE.md")
	if isSymlink(dst) {
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("no se pudo quitar symlink viejo %s: %w", dst, err)
		}
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# %s — generado por ccp (no editar a mano; usa: ccp profile config %s)\n\n", name, name)
	globalMD := filepath.Join(src, "CLAUDE.md")
	if fileExists(globalMD) {
		fmt.Fprintf(&buf, "@%s\n", globalMD)
	}
	fmt.Fprintf(&buf, "@%s\n", overlay)
	if err := os.WriteFile(dst, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", dst, err)
	}
	return nil
}

// cfgMergeSettings escribe cc-home/settings.json = global ⊕ overlay. El global
// (<src>/settings.json) se ignora si está ausente o es JSON inválido (solo
// overlay). El overlay debe existir (lo crea CfgInitOverlay).
func cfgMergeSettings(home, name, src string) error {
	cch := ccHomePath(home, name)
	out := filepath.Join(cch, "settings.json")
	overlayFile := cfgSettingsFile(home, name)

	overlayData, err := os.ReadFile(overlayFile)
	if err != nil {
		return fmt.Errorf("no se pudo leer overlay de settings de %q: %w", name, err)
	}

	var globalData []byte
	globalFile := filepath.Join(src, "settings.json")
	if g, err := os.ReadFile(globalFile); err == nil && json.Valid(g) {
		globalData = g
	}

	merged, err := MergeJSON(globalData, overlayData)
	if err != nil {
		return fmt.Errorf("merge de settings de %q falló: %w", name, err)
	}
	if err := os.MkdirAll(cch, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear cc-home de %q: %w", name, err)
	}
	if err := os.WriteFile(out, merged, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", out, err)
	}
	return nil
}

// CfgRegenerate regenera el cc-home efectivo de un perfil desde global ⊕
// overlay (idempotente). src es la fuente global (CCP_CLAUDE_SRC o ~/.claude).
// Se ejecuta en create/edit/sync — NUNCA en el hook.
func CfgRegenerate(home, name, src string) error {
	if err := CfgInitOverlay(home, name); err != nil {
		return err
	}
	if err := cfgWriteClaudeMD(home, name, src); err != nil {
		return err
	}
	return cfgMergeSettings(home, name, src)
}

// CfgMigrateLegacy convierte un cc-home viejo (pre-overlay) al modelo overlay.
// Idempotente: solo actúa si detecta el estado viejo (settings.json como copia
// real + CLAUDE.md como symlink). Espeja ccp_cfg_migrate_legacy del bash.
func CfgMigrateLegacy(home, name string) error {
	cch := ccHomePath(home, name)
	overlayFile := cfgSettingsFile(home, name)
	if err := os.MkdirAll(cfgOverlayDir(home, name), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear overlay de %q: %w", name, err)
	}

	// settings.json copia real (no symlink) y sin overlay aún => muévelo.
	legacySettings := filepath.Join(cch, "settings.json")
	if isRegularFile(legacySettings) && !pathExists(overlayFile) {
		if err := os.Rename(legacySettings, overlayFile); err != nil {
			return fmt.Errorf("no se pudo mover settings legacy de %q: %w", name, err)
		}
	}

	// CLAUDE.md symlink viejo => quítalo (se regenerará como @import).
	legacyClaude := filepath.Join(cch, "CLAUDE.md")
	if isSymlink(legacyClaude) {
		if err := os.Remove(legacyClaude); err != nil {
			return fmt.Errorf("no se pudo quitar CLAUDE.md symlink de %q: %w", name, err)
		}
	}
	return nil
}

// --- helpers de filesystem ---

// pathExists devuelve true si la ruta existe (siguiendo el equivalente de
// `[[ -e ]]`: incluye symlinks, dado que Lstat no sigue el enlace).
func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// isSymlink devuelve true si p existe y es un symlink (`[[ -L ]]`).
func isSymlink(p string) bool {
	fi, err := os.Lstat(p)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// isRegularFile devuelve true si p existe, es archivo regular y NO symlink
// (espeja `[[ -f X && ! -L X ]]`).
func isRegularFile(p string) bool {
	fi, err := os.Lstat(p)
	if err != nil {
		return false
	}
	return fi.Mode().IsRegular()
}
