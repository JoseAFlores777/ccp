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

// profileStateDir es el estado derivado de un perfil que no es de Claude Code.
// No vive en el cc-home, porque Claude Code lo lee entero, ni en overlay/, que
// es authored y lo capturan snapshots y backups: SnapshotSources y BackupExport
// enumeran rutas explícitas y esta no está. Cuelga de profiles/<n>/, así que se
// mueve con el rename y se borra con el rm.
func profileStateDir(home, name string) string {
	return filepath.Join(profileDirPath(home, name), "state")
}

// lastSettingsPath es la copia de lo último que ccp escribió en
// cc-home/settings.json: la línea base de la deriva de /config (cfg_drift.go).
// Sin ella no se distingue «el usuario cambió esto con /config» de «cambió el
// global», y adoptar lo segundo congelaría el global en el overlay.
func lastSettingsPath(home, name string) string {
	return filepath.Join(profileStateDir(home, name), "last-settings.json")
}

// invalidSettingsPath guarda un cc-home/settings.json que no era JSON antes de
// regenerarlo encima (adoptSettingsDrift): lo escribió alguien, y regenerar sin
// copia sería perderlo sin rastro. Una sola ruta: la copia más reciente es la
// que el aviso enseña.
func invalidSettingsPath(home, name string) string {
	return filepath.Join(profileStateDir(home, name), "settings.invalid.json")
}

// ProfileSettingsFile expone la ruta del settings.overlay.json de un perfil a
// los front-ends (la vista de perfil la usa para sembrar overlays en tests y
// para saber qué archivo abre 'e' sobre la caja Env/Efectivo).
func ProfileSettingsFile(home, name string) string { return cfgSettingsFile(home, name) }

// ProfileInstrFile expone la ruta del CLAUDE.md del overlay a los front-ends.
func ProfileInstrFile(home, name string) string { return cfgInstrFile(home, name) }

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
	return CfgValidateBytes(data)
}

// CfgValidateBytes devuelve error si data no es JSON válido. Helper compartido
// por el motor de instrucciones (#9) para no reimportar encoding/json fuera de
// este archivo.
func CfgValidateBytes(data []byte) error {
	if !json.Valid(data) {
		return fmt.Errorf("JSON inválido")
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

// cfgBuildUserSettings es global ⊕ overlay: lo que sale de las capas del
// usuario, sin la capa auto. El global (<src>/settings.json) se ignora si está
// ausente o es JSON inválido (solo overlay). Está separado de la escritura
// porque la adopción de la deriva necesita saber qué saldría de regenerar sin
// adoptar nada, para no copiar al overlay lo que ya sale de las capas; y
// separado de la capa auto porque la deriva se mide sin ella (stripAutoLayer).
func cfgBuildUserSettings(name, src string, overlayData []byte) ([]byte, error) {
	var globalData []byte
	globalFile := filepath.Join(src, "settings.json")
	if g, err := os.ReadFile(globalFile); err == nil && json.Valid(g) {
		globalData = g
	}
	merged, err := MergeJSON(globalData, overlayData)
	if err != nil {
		return nil, fmt.Errorf("merge de settings de %q falló: %w", name, err)
	}
	return merged, nil
}

// cfgBuildSettings construye el settings.json de un perfil (global ⊕ overlay ⊕
// auto) sin escribirlo.
func cfgBuildSettings(home, name, src string, overlayData []byte) ([]byte, error) {
	merged, err := cfgBuildUserSettings(name, src, overlayData)
	if err != nil {
		return nil, err
	}
	// Tercera capa: los sensores del auto-handoff (autohooks.go), solo si este
	// perfil los tiene instalados en ccp.yaml. Va DESPUÉS del overlay porque es
	// infraestructura de ccp, no preferencia del usuario: si alguien deja un
	// statusLine en su overlay, la capa lo envuelve en vez de perderlo.
	// applyAutoLayer no falla nunca — regenerar un cc-home no puede depender de
	// que ccp.yaml sea legible en ese instante.
	return applyAutoLayer(home, name, merged), nil
}

// cfgMergeSettings escribe cc-home/settings.json = global ⊕ overlay ⊕ auto
// (cfgBuildSettings) y guarda la copia de lo escrito, que es la línea base de
// la deriva de /config. Devuelve la deriva que adoptó antes de escribir. El
// overlay debe existir (lo crea CfgInitOverlay).
func cfgMergeSettings(home, name, src string) (SettingsDrift, error) {
	// Primero la deriva: lo que /config escribió en cc-home pasa al overlay ANTES
	// de leerlo para el merge, o la escritura de abajo lo pisaría (B6). Si falla
	// (un settings.json inválido del que no se pudo guardar copia), no se
	// escribe nada.
	drift, err := adoptSettingsDrift(home, name, src)
	if err != nil {
		return drift, err
	}
	cch := ccHomePath(home, name)
	out := filepath.Join(cch, "settings.json")
	overlayFile := cfgSettingsFile(home, name)

	overlayData, err := os.ReadFile(overlayFile)
	if err != nil {
		return drift, fmt.Errorf("no se pudo leer overlay de settings de %q: %w", name, err)
	}
	merged, err := cfgBuildSettings(home, name, src, overlayData)
	if err != nil {
		return drift, err
	}
	if err := os.MkdirAll(cch, 0o755); err != nil {
		return drift, fmt.Errorf("no se pudo crear cc-home de %q: %w", name, err)
	}
	if err := os.WriteFile(out, merged, 0o644); err != nil {
		return drift, fmt.Errorf("no se pudo escribir %s: %w", out, err)
	}
	if err := saveLastSettings(home, name, merged); err != nil {
		return drift, err
	}
	return drift, nil
}

// saveLastSettings guarda la copia de lo que se acaba de escribir en el
// cc-home, en 0600 y por tmp+rename (un corte a medias no deja media copia).
// Si no puede, borra la vieja y lo dice: una copia desfasada es peor que
// ninguna, porque la siguiente regeneración tomaría por deriva los cambios del
// global y los congelaría en el overlay. Sin copia, simplemente no se adopta.
func saveLastSettings(home, name string, data []byte) error {
	p := lastSettingsPath(home, name)
	if err := writeFileAtomic(p, data, 0o600); err != nil {
		_ = os.Remove(p)
		return fmt.Errorf("no se pudo guardar la copia de lo generado para %q: %w", name, err)
	}
	return nil
}

// CfgRegenerate regenera el cc-home efectivo de un perfil desde global ⊕
// overlay ⊕ auto (idempotente). src es la fuente global (CCP_CLAUDE_SRC o
// ~/.claude); la capa auto solo se aplica a los perfiles listados en
// auto_handoff.hooks (ver autohooks.go). Se ejecuta en create/edit/sync —
// NUNCA en el hook. Adopta la deriva de /config igual que
// CfgRegenerateReport; solo no la cuenta.
func CfgRegenerate(home, name, src string) error {
	_, err := CfgRegenerateReport(home, name, src)
	return err
}

// CfgRegenerateReport es CfgRegenerate devolviendo lo que encontró cambiado a
// mano en cc-home/settings.json (B6). La adopción ocurre en TODOS los caminos
// que regeneran (sync, config, instruct, la GUI, los restores, el rename): si
// solo la hiciera `profile sync`, cualquier otro camino perdería el /config
// igual que antes. Contarlo es lo opcional.
func CfgRegenerateReport(home, name, src string) (SettingsDrift, error) {
	if err := CfgInitOverlay(home, name); err != nil {
		return SettingsDrift{Profile: name}, err
	}
	if err := cfgWriteClaudeMD(home, name, src); err != nil {
		return SettingsDrift{Profile: name}, err
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
