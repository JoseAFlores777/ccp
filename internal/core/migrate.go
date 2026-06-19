package core

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Migrate es el migrador universal y encadenado: dsctl -> ccp(TSV) -> ccp.yaml.
// Es idempotente: si ccp.yaml ya existe es no-op. El timestamp del backup lo
// pasa el caller (stamp) para mantener la lógica pura y determinista en tests.
//
// Estados detectados:
//   - ~/.config/dsctl existe Y el home ccp no tiene ccp.yaml/tsv -> dsctl->TSV
//     y luego continúa con TSV->ccp.yaml.
//   - el home tiene *.tsv/meta pero no ccp.yaml -> TSV/meta -> ccp.yaml.
//   - ccp.yaml ya existe -> no-op.
//
// Antes de tocar nada copia el estado legacy a
// <home>/.backup-pre-go-<stamp>/. NUNCA borra api_key ni cc-home.
//
// dsctlHome puede ser "" para usar ~/.config/dsctl; se expone para tests.
func Migrate(home, stamp string) error {
	return migrate(home, defaultDsctlHome(), stamp)
}

// MigrateFrom es como Migrate pero permite especificar el home de dsctl
// (útil en tests para no tocar el real).
func MigrateFrom(home, dsctlHome, stamp string) error {
	return migrate(home, dsctlHome, stamp)
}

func defaultDsctlHome() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config", "dsctl")
	}
	return ""
}

func migrate(home, dsctlHome, stamp string) error {
	// ccp.yaml ya existe -> no-op (idempotencia).
	if fileExists(yamlPath(home)) {
		return nil
	}

	hasCCPState := fileExists(filepath.Join(home, "profiles.tsv")) ||
		dirHasMeta(filepath.Join(home, "profiles"))
	dsctlPresent := dsctlHome != "" && isDir(dsctlHome)

	if !hasCCPState && !dsctlPresent {
		// Nada que migrar: arranque limpio. Escribimos un ccp.yaml mínimo.
		c := &Config{Version: SchemaVersion, Profiles: map[string]Profile{}}
		return Save(home, c)
	}

	if err := os.MkdirAll(home, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear home ccp %s: %w", home, err)
	}

	// Backup ANTES de tocar nada.
	backupDir := filepath.Join(home, ".backup-pre-go-"+stamp)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear backup %s: %w", backupDir, err)
	}

	// Si solo hay estado dsctl, primero lo convertimos a TSV/meta en el home.
	if !hasCCPState && dsctlPresent {
		if err := backupDsctl(dsctlHome, backupDir); err != nil {
			return err
		}
		if err := migrateDsctlToTSV(home, dsctlHome); err != nil {
			return err
		}
	} else {
		// Backup del estado TSV/meta ccp existente.
		if err := backupCCPState(home, backupDir); err != nil {
			return err
		}
	}

	// TSV/meta -> ccp.yaml.
	c, err := readLegacyState(home)
	if err != nil {
		return err
	}
	return Save(home, c)
}

// ----- dsctl -> TSV/meta -------------------------------------------------

// migrateDsctlToTSV reproduce cmd_migrate del bash: lee config (DS_*),
// crea un perfil deepseek con sus 4 campos, copia api_key, y convierte
// include->deepseek / exclude->default en rules.tsv.
func migrateDsctlToTSV(home, dsctlHome string) error {
	base := "https://api.deepseek.com/anthropic"
	pro := "deepseek-chat"
	flash := "deepseek-chat"
	effort := "high"

	if cfgPath := filepath.Join(dsctlHome, "config"); fileExists(cfgPath) {
		kv, err := readKVFile(cfgPath)
		if err != nil {
			return err
		}
		if v, ok := kv["DS_BASE_URL"]; ok {
			base = unquote(v)
		}
		if v, ok := kv["DS_MODEL_PRO"]; ok {
			pro = unquote(v)
		}
		if v, ok := kv["DS_MODEL_FLASH"]; ok {
			flash = unquote(v)
		}
		if v, ok := kv["DS_EFFORT"]; ok {
			effort = unquote(v)
		}
	}

	// Escribe profiles/deepseek/meta + índice profiles.tsv.
	metaDir := filepath.Join(home, "profiles", "deepseek")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}
	meta := fmt.Sprintf("type=deepseek\nbase_url=%s\nmodel_pro=%s\nmodel_flash=%s\neffort=%s\n",
		base, pro, flash, effort)
	if err := os.WriteFile(filepath.Join(metaDir, "meta"), []byte(meta), 0o644); err != nil {
		return err
	}
	if err := setIndexEntry(home, "deepseek", "deepseek"); err != nil {
		return err
	}

	// api_key (secreto): se copia a profiles/deepseek/api_key 600.
	if keyPath := filepath.Join(dsctlHome, "api_key"); fileExists(keyPath) {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return err
		}
		if err := SetKey(home, "deepseek", string(data)); err != nil {
			return err
		}
	}

	// rules.tsv: include -> deepseek, exclude -> default.
	rulesPath := filepath.Join(dsctlHome, "rules.tsv")
	if !fileExists(rulesPath) {
		return nil
	}
	f, err := os.Open(rulesPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		kind, path := parts[0], parts[1]
		if path == "" {
			continue
		}
		var profile string
		switch kind {
		case "include":
			profile = "deepseek"
		case "exclude":
			profile = "default"
		default:
			continue
		}
		lines = append(lines, NormalizePath(path)+"\t"+profile)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	out := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filepath.Join(home, "rules.tsv"), []byte(out), 0o644)
}

// setIndexEntry añade/reemplaza una entrada en profiles.tsv (name<TAB>type).
func setIndexEntry(home, name, typ string) error {
	idx := filepath.Join(home, "profiles.tsv")
	var kept []string
	if data, err := os.ReadFile(idx); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if parts[0] == name {
				continue
			}
			kept = append(kept, line)
		}
	}
	kept = append(kept, name+"\t"+typ)
	return os.WriteFile(idx, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}

// ----- TSV/meta -> Config ------------------------------------------------

// readLegacyState lee profiles.tsv + cada meta + rules.tsv + config + authored.tsv
// del home y construye un Config.
func readLegacyState(home string) (*Config, error) {
	c := &Config{Version: SchemaVersion, Profiles: map[string]Profile{}}

	// Defaults desde `config` (key=value).
	if cfgPath := filepath.Join(home, "config"); fileExists(cfgPath) {
		kv, err := readKVFile(cfgPath)
		if err != nil {
			return nil, err
		}
		c.Defaults = Defaults{
			BaseURL:    kv["base_url"],
			ModelPro:   kv["model_pro"],
			ModelFlash: kv["model_flash"],
			Effort:     kv["effort"],
			Editor:     kv["editor"],
		}
	}

	// Perfiles: itera profiles.tsv y lee cada meta.
	idxPath := filepath.Join(home, "profiles.tsv")
	if fileExists(idxPath) {
		data, err := os.ReadFile(idxPath)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			name := parts[0]
			if name == "" || name == "default" {
				continue
			}
			metaPath := filepath.Join(home, "profiles", name, "meta")
			if !fileExists(metaPath) {
				continue
			}
			kv, err := readKVFile(metaPath)
			if err != nil {
				return nil, err
			}
			p := Profile{Type: kv["type"]}
			if IsProviderType(p.Type) {
				p.BaseURL = kv["base_url"]
				p.ModelPro = kv["model_pro"]
				p.ModelFlash = kv["model_flash"]
				p.Effort = kv["effort"]
			}
			c.Profiles[name] = p
		}
	}

	// Reglas: rules.tsv (/abs<TAB>profile).
	if rulesPath := filepath.Join(home, "rules.tsv"); fileExists(rulesPath) {
		f, err := os.Open(rulesPath)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			t := strings.TrimSpace(line)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				continue
			}
			c.Rules = append(c.Rules, Rule{Path: parts[0], Profile: parts[1]})
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}

	// Authored: authored.tsv (scope<TAB>profile<TAB>type<TAB>ref<TAB>desc).
	if aPath := filepath.Join(home, "authored.tsv"); fileExists(aPath) {
		f, err := os.Open(aPath)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 5)
			if len(parts) != 5 {
				continue
			}
			prof := parts[1]
			if prof == "-" {
				prof = ""
			}
			c.Authored = append(c.Authored, Authored{
				Scope:   parts[0],
				Profile: prof,
				Type:    parts[2],
				Ref:     parts[3],
				Desc:    parts[4],
			})
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}

	return c, nil
}

// ----- backups -----------------------------------------------------------

func backupDsctl(dsctlHome, backupDir string) error {
	dst := filepath.Join(backupDir, "dsctl")
	return copyTree(dsctlHome, dst)
}

// backupCCPState copia los archivos de estado legacy del home ccp (no cc-home,
// no api_key se eliminan: solo se copian los archivos legacy de índice/reglas
// y los meta de perfiles).
func backupCCPState(home, backupDir string) error {
	for _, name := range []string{"profiles.tsv", "rules.tsv", "config", "authored.tsv"} {
		src := filepath.Join(home, name)
		if fileExists(src) {
			if err := copyFile(src, filepath.Join(backupDir, name)); err != nil {
				return err
			}
		}
	}
	// Copia el árbol profiles/ (incluye meta y api_key) para tener un backup
	// completo y recuperable. NO se borra el original.
	profSrc := filepath.Join(home, "profiles")
	if isDir(profSrc) {
		if err := copyTree(profSrc, filepath.Join(backupDir, "profiles")); err != nil {
			return err
		}
	}
	return nil
}

// ----- helpers -----------------------------------------------------------

// readKVFile parsea un archivo key=value (parse, NO source). Devuelve el
// primer valor por clave (tras el primer '='). Ignora líneas vacías y
// comentarios.
func readKVFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		key := line[:i]
		val := line[i+1:]
		if _, exists := out[key]; !exists {
			out[key] = val
		}
	}
	return out, sc.Err()
}

// unquote quita comillas dobles envolventes (formato DS_* del config dsctl).
func unquote(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
		return s[1 : len(s)-1]
	}
	return s
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// dirHasMeta reporta si profiles/ contiene al menos un <name>/meta.
func dirHasMeta(profilesDir string) bool {
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && fileExists(filepath.Join(profilesDir, e.Name(), "meta")) {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Preserva symlinks (cc-home enlaza a ~/.claude). Copia el link.
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		}
		return copyFile(path, target)
	})
}

// NormalizePath normaliza textualmente ~, paths relativos, . y .. (espeja
// ccp_norm_path del bash). No usa realpath: paths inexistentes funcionan.
func NormalizePath(p string) string {
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	if !strings.HasPrefix(p, "/") {
		if wd, err := os.Getwd(); err == nil {
			p = wd + "/" + p
		}
	}
	var out []string
	for _, part := range strings.Split(p, "/") {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return "/"
	}
	return "/" + strings.Join(out, "/")
}
