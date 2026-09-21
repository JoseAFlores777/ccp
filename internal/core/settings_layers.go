package core

// settings_layers.go — editar una capa de settings por clave (spec §6.3) y la
// unión opcional de los permisos.
//
// El merge de ccp REEMPLAZA arrays, igual que `jq . * $x` (ADR 0002): un
// `permissions.allow` en el overlay tapa la lista global entera. Eso sigue así
// por defecto, porque cambiarlo en silencio rompería perfiles que cuentan con
// ello. Quien quiera sumar en vez de tapar lo dice en su overlay:
//
//	"permissions": {"$merge": "union", "allow": ["Bash(make)"]}
//
// y entonces allow/deny/ask salen como global ⊎ overlay, sin duplicados y con el
// global primero. La marca no llega al settings.json generado: es instrucción
// para ccp, no configuración de Claude Code.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// mergeMarkKey es la marca que pide unión en vez de reemplazo.
const mergeMarkKey = "$merge"

// permissionLists son las tres listas que la unión afecta. defaultMode no es una
// lista: se reemplaza siempre.
var permissionLists = []string{"allow", "deny", "ask"}

// applyPermissionUnion ajusta el resultado de MergeJSON cuando el overlay pidió
// unión. Devuelve el documento ya sin la marca.
func applyPermissionUnion(global, overlay, merged map[string]any) map[string]any {
	ov, _ := overlay["permissions"].(map[string]any)
	if ov == nil {
		return merged
	}
	mark, _ := ov[mergeMarkKey].(string)
	mp, _ := merged["permissions"].(map[string]any)
	if mp == nil {
		return merged
	}
	delete(mp, mergeMarkKey) // nunca viaja al archivo generado
	if mark != "union" {
		return merged
	}
	gp, _ := global["permissions"].(map[string]any)
	for _, k := range permissionLists {
		gl, _ := gp[k].([]any)
		ol, _ := ov[k].([]any)
		if len(gl) == 0 && len(ol) == 0 {
			continue
		}
		seen := map[string]bool{}
		out := make([]any, 0, len(gl)+len(ol))
		for _, v := range append(append([]any{}, gl...), ol...) {
			key := fmt.Sprint(v)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, v)
		}
		mp[k] = out
	}
	return merged
}

// SettingsLayer identifica una capa editable: "global" o "profile:<nombre>".
type SettingsLayer string

// settingsLayerFile devuelve el archivo de esa capa.
func settingsLayerFile(home, src string, layer SettingsLayer) (string, error) {
	if layer == "global" {
		return filepath.Join(src, "settings.json"), nil
	}
	name, ok := strings.CutPrefix(string(layer), "profile:")
	if !ok || name == "" || name == "default" {
		return "", fmt.Errorf("capa de settings desconocida: %q (vale «global» o «profile:<nombre>»)", layer)
	}
	return cfgSettingsFile(home, name), nil
}

// SettingsLayerGet lee una clave de una capa. El segundo valor dice si estaba.
func SettingsLayerGet(home, src string, layer SettingsLayer, path []string) (any, bool, error) {
	file, err := settingsLayerFile(home, src, layer)
	if err != nil {
		return nil, false, err
	}
	b, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	doc, err := decodeOverlayObject(b)
	if err != nil {
		return nil, false, fmt.Errorf("%s no es JSON válido: %w", file, err)
	}
	v, ok := jsonLookup(doc, path)
	return v, ok, nil
}

// SettingsLayerSet fija (o borra, con value nil y del=true) una clave de una
// capa y devuelve el archivo tocado. Escribe de forma atómica conservando el
// modo y siguiendo un symlink, como el overlay de la deriva: un repo de dotfiles
// enlazado sigue enlazado. No regenera: eso lo decide quien llama.
//
// Los hooks se editan como el array entero de su evento
// (path = ["hooks", "PreToolUse"]), que es lo que resuelve «se añaden pero no se
// borran»: para quitar uno se escribe la lista sin él.
func SettingsLayerSet(home, src string, layer SettingsLayer, path []string, value any, del bool) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("hace falta una clave que fijar")
	}
	file, err := settingsLayerFile(home, src, layer)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	doc, err := decodeOverlayObject(b)
	if err != nil {
		return "", fmt.Errorf("%s no es JSON válido; no se toca: %w", file, err)
	}
	if del {
		jsonDeletePath(doc, path)
	} else {
		jsonSetPath(doc, path, value)
	}
	out, err := marshalIndent(doc)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	if err := writeOverlayPreservingLink(file, out); err != nil {
		return "", err
	}
	return file, nil
}

// jsonDeletePath borra la clave del final del camino, si existe. Los objetos
// intermedios que queden vacíos se conservan: un `permissions: {}` explícito es
// distinto de no tener la clave, y borrarlo cambiaría el merge.
func jsonDeletePath(doc map[string]any, path []string) {
	m := doc
	for _, k := range path[:len(path)-1] {
		next, ok := m[k].(map[string]any)
		if !ok {
			return
		}
		m = next
	}
	delete(m, path[len(path)-1])
}
