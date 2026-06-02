package core

// backup.go — export/restore portable del estado de ccp (port del plan §6).
//
// Un backup es un .tar.gz que SIEMPRE incluye:
//   - manifest.yaml  (metadatos + sha256 por miembro del payload)
//   - ccp.yaml       (perfiles, reglas, defaults, authored)
//   - profiles/<n>/overlay/{CLAUDE.md, settings.overlay.json} de cada perfil
//
// Con --with-secrets añade además:
//   - profiles/<n>/api_key            (cada perfil deepseek)
//   - profiles/<n>/cc-home/.claude.json (credenciales OAuth de cada official)
//
// EXCLUYE los symlinks re-seedables (plugins/commands/agents/skills): se
// reconstruyen al restaurar via seedCCHome.
//
// El manifest NO se auto-incluye en checksums; restore valida que el tar abra
// y el manifest parsee, luego verifica sha256 de cada miembro antes de aplicar.
//
// `created` lo pasa el caller (no time.Now en lógica pura) para tests
// deterministas.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	yaml "github.com/goccy/go-yaml"
)

// ManifestProfile es la entrada por-perfil en el manifest (nombre + tipo).
type ManifestProfile struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// Manifest es el contenido de manifest.yaml dentro del .tar.gz.
type Manifest struct {
	CCPVersion    string            `yaml:"ccp_version"`
	SchemaVersion int               `yaml:"schema_version"`
	Created       string            `yaml:"created"`
	WithSecrets   bool              `yaml:"with_secrets"`
	Profiles      []ManifestProfile `yaml:"profiles"`
	// Checksums: sha256 por miembro del payload, en la forma "sha256:<hex>".
	// La clave es la ruta del miembro dentro del tar (p.ej. "ccp.yaml").
	Checksums map[string]string `yaml:"checksums"`
}

// manifestMemberName es el nombre reservado del manifest dentro del tar.
const manifestMemberName = "manifest.yaml"

// member es un archivo del payload listo para escribir al tar: su nombre
// (ruta relativa dentro del archivo), contenido y modo de permisos.
type member struct {
	name string
	data []byte
	mode int64
}

// BackupExport escribe un .tar.gz de backup en dest. Si withSecrets es true
// incluye api_key (deepseek) y cc-home/.claude.json (official) con modo 600.
// created se usa tal cual en el manifest (formato RFC3339 UTC recomendado por
// el caller). El archivo resultante se escribe con 0600 si withSecrets, 0644
// si no.
func BackupExport(home, dest string, withSecrets bool, created time.Time) error {
	if dest == "" {
		return fmt.Errorf("Uso: ccp backup export <archivo.tar.gz> [--with-secrets]")
	}
	c, err := Load(home)
	if err != nil {
		return err
	}

	members, mprofiles, err := collectMembers(home, c, withSecrets)
	if err != nil {
		return err
	}

	// Checksums sha256 por miembro (el manifest no se auto-incluye).
	checksums := make(map[string]string, len(members))
	for _, m := range members {
		checksums[m.name] = "sha256:" + sha256Hex(m.data)
	}

	man := Manifest{
		CCPVersion:    Version,
		SchemaVersion: c.Version,
		Created:       created.UTC().Format(time.RFC3339),
		WithSecrets:   withSecrets,
		Profiles:      mprofiles,
		Checksums:     checksums,
	}
	manData, err := yaml.Marshal(&man)
	if err != nil {
		return fmt.Errorf("no se pudo serializar manifest: %w", err)
	}

	// Escritura atómica: tmp + rename. El manifest va primero en el tar.
	mode := os.FileMode(0o644)
	if withSecrets {
		mode = 0o600
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear directorio destino: %w", err)
	}
	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", tmp, err)
	}

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	all := append([]member{{name: manifestMemberName, data: manData, mode: 0o600}}, members...)
	for _, m := range all {
		hdr := &tar.Header{
			Name:    m.name,
			Mode:    m.mode,
			Size:    int64(len(m.data)),
			ModTime: created.UTC(),
			Format:  tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = closeAll(tw, gz, f)
			_ = os.Remove(tmp)
			return fmt.Errorf("no se pudo escribir header de %s: %w", m.name, err)
		}
		if _, err := tw.Write(m.data); err != nil {
			_ = closeAll(tw, gz, f)
			_ = os.Remove(tmp)
			return fmt.Errorf("no se pudo escribir %s: %w", m.name, err)
		}
	}

	if err := closeAll(tw, gz, f); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("no se pudo renombrar %s -> %s: %w", tmp, dest, err)
	}
	// Reafirma el modo (rename conserva el del tmp, pero por si acaso).
	if err := os.Chmod(dest, mode); err != nil {
		return fmt.Errorf("no se pudo aplicar permisos a %s: %w", dest, err)
	}
	return nil
}

// closeAll cierra el tar, gzip y archivo en orden, devolviendo el primer error.
func closeAll(tw *tar.Writer, gz *gzip.Writer, f *os.File) error {
	var first error
	if err := tw.Close(); err != nil && first == nil {
		first = fmt.Errorf("no se pudo cerrar tar: %w", err)
	}
	if err := gz.Close(); err != nil && first == nil {
		first = fmt.Errorf("no se pudo cerrar gzip: %w", err)
	}
	if err := f.Close(); err != nil && first == nil {
		first = fmt.Errorf("no se pudo cerrar archivo: %w", err)
	}
	return first
}

// collectMembers junta el payload (sin el manifest) y la lista de perfiles.
// Recorre los perfiles en orden alfabético para salida determinista.
func collectMembers(home string, c *Config, withSecrets bool) ([]member, []ManifestProfile, error) {
	var members []member

	// ccp.yaml: re-serializa desde el Config cargado (normaliza, sin secretos).
	ccpData, err := os.ReadFile(yamlPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("no hay ccp.yaml en %s; nada que respaldar", home)
		}
		return nil, nil, fmt.Errorf("no se pudo leer ccp.yaml: %w", err)
	}
	members = append(members, member{name: "ccp.yaml", data: ccpData, mode: 0o600})

	names := make([]string, 0, len(c.Profiles))
	for n := range c.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)

	mprofiles := make([]ManifestProfile, 0, len(names))
	for _, name := range names {
		p := c.Profiles[name]
		mprofiles = append(mprofiles, ManifestProfile{Name: name, Type: p.Type})

		// overlay/ siempre (CLAUDE.md + settings.overlay.json) si existen.
		for _, rel := range []string{"overlay/CLAUDE.md", "overlay/settings.overlay.json"} {
			abs := filepath.Join(profileDirPath(home, name), filepath.FromSlash(rel))
			data, err := os.ReadFile(abs)
			if err != nil {
				if os.IsNotExist(err) {
					continue // overlay opcional: no todos los perfiles lo tienen aún
				}
				return nil, nil, fmt.Errorf("no se pudo leer %s: %w", abs, err)
			}
			members = append(members, member{
				name: tarPath("profiles", name, rel),
				data: data,
				mode: 0o644,
			})
		}

		if !withSecrets {
			continue
		}
		// api_key de deepseek.
		if p.Type == "deepseek" {
			if data, ok := GetKey(home, name); ok {
				members = append(members, member{
					name: tarPath("profiles", name, "api_key"),
					data: []byte(data),
					mode: 0o600,
				})
			}
		}
		// credenciales OAuth de official: cc-home/.claude.json.
		if p.Type == "official" {
			cj := filepath.Join(ccHomePath(home, name), ".claude.json")
			if data, err := os.ReadFile(cj); err == nil {
				members = append(members, member{
					name: tarPath("profiles", name, "cc-home/.claude.json"),
					data: data,
					mode: 0o600,
				})
			}
		}
	}
	return members, mprofiles, nil
}

// path une segmentos con "/" (los nombres del tar usan siempre slash, no el
// separador de la plataforma — clave para portabilidad macOS/Linux).
func tarPath(parts ...string) string {
	return strings.Join(parts, "/")
}

// sha256Hex devuelve el hex del sha256 de data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

// --- restore ---

// RestoreOpts controla la política de colisiones del restore.
//   - Overwrite: reemplaza perfiles que ya existan (los que colisionan).
//   - Force: wipe global + restore total (descarta el estado actual de ccp.yaml
//     y perfiles, luego aplica el backup). Implica Overwrite.
type RestoreOpts struct {
	Overwrite bool
	Force     bool
	// SnapshotDir, si != "", fija el directorio del auto-snapshot pre-restore
	// (tests deterministas). Si "", se deriva de Now con sufijo de fecha.
	SnapshotDir string
	// Now lo pasa el caller para nombrar el snapshot (no time.Now en lógica
	// pura). Si SnapshotDir != "" se ignora.
	Now time.Time
}

// RestoreReport resume el resultado del restore para presentación.
type RestoreReport struct {
	Created     []string // perfiles creados (no existían)
	Skipped     []string // perfiles saltados por colisión (sin --overwrite/--force)
	Overwritten []string // perfiles reemplazados
	RulesAdded  int      // reglas de path añadidas por merge
	SnapshotDir string   // ruta del auto-snapshot tomado antes de aplicar
}

// backupArchive es el contenido parseado de un .tar.gz de backup en memoria.
type backupArchive struct {
	manifest Manifest
	members  map[string]member // por nombre de miembro (sin el manifest)
}

// BackupRestore aplica un backup desde archive sobre home. Verifica sha256 por
// miembro antes de aplicar, toma un auto-snapshot reversible del estado actual,
// y respeta la política de colisiones de opts. Aborta si schema_version del
// manifest es mayor a la conocida por el binario.
func BackupRestore(home, archive string, opts RestoreOpts) (RestoreReport, error) {
	var rep RestoreReport

	ba, err := readBackup(archive)
	if err != nil {
		return rep, err
	}

	// Cross-version: espeja la política de Load (aborta si es más nuevo).
	if ba.manifest.SchemaVersion > SchemaVersion {
		return rep, fmt.Errorf(
			"el backup usa schema version %d pero este ccp solo conoce hasta %d; "+
				"actualiza ccp antes de restaurar", ba.manifest.SchemaVersion, SchemaVersion)
	}

	// Verifica sha256 de cada miembro contra el manifest ANTES de tocar disco.
	if err := verifyChecksums(ba); err != nil {
		return rep, err
	}

	// Auto-snapshot reversible del estado actual.
	snapDir := opts.SnapshotDir
	if snapDir == "" {
		stamp := opts.Now.UTC().Format("20060102-150405")
		snapDir = filepath.Join(home, ".backup-pre-restore-"+stamp)
	}
	if err := snapshotCurrent(home, snapDir); err != nil {
		return rep, err
	}
	rep.SnapshotDir = snapDir

	// --force: wipe global. Borra ccp.yaml y todos los perfiles antes de
	// aplicar. El snapshot ya quedó tomado, así que es reversible.
	if opts.Force {
		if err := os.RemoveAll(filepath.Join(home, "profiles")); err != nil {
			return rep, fmt.Errorf("no se pudo limpiar profiles para --force: %w", err)
		}
		if err := os.Remove(yamlPath(home)); err != nil && !os.IsNotExist(err) {
			return rep, fmt.Errorf("no se pudo limpiar ccp.yaml para --force: %w", err)
		}
	}

	// Carga el Config destino actual (tras posible wipe) y el del backup.
	curp, err := Load(home)
	if err != nil {
		return rep, err
	}
	cur := curp
	bc, err := configFromBytes(ba.members["ccp.yaml"].data)
	if err != nil {
		return rep, fmt.Errorf("ccp.yaml del backup inválido: %w", err)
	}

	overwrite := opts.Overwrite || opts.Force

	// Aplica perfil por perfil (orden determinista por manifest).
	for _, mp := range ba.manifest.Profiles {
		name := mp.Name
		_, exists := cur.Profiles[name]
		switch {
		case !exists:
			if err := applyProfile(home, ba, bc, name, cur); err != nil {
				return rep, err
			}
			rep.Created = append(rep.Created, name)
		case name == "default":
			// 'default' es implícito; nunca se materializa.
			rep.Skipped = append(rep.Skipped, name)
		case !overwrite:
			rep.Skipped = append(rep.Skipped, name)
		default:
			// Reemplaza: borra el dir del perfil y reaplica.
			if err := os.RemoveAll(profileDirPath(home, name)); err != nil {
				return rep, fmt.Errorf("no se pudo limpiar perfil %q: %w", name, err)
			}
			if err := applyProfile(home, ba, bc, name, cur); err != nil {
				return rep, err
			}
			rep.Overwritten = append(rep.Overwritten, name)
		}
	}

	// Reglas de path: merge. Las del backup que no chocan (path nuevo) se
	// añaden; las que ya existen se conservan tal cual (no se pisan).
	existing := make(map[string]struct{}, len(cur.Rules))
	for _, r := range cur.Rules {
		existing[r.Path] = struct{}{}
	}
	for _, r := range bc.Rules {
		if _, ok := existing[r.Path]; ok {
			continue
		}
		cur.Rules = append(cur.Rules, r)
		existing[r.Path] = struct{}{}
		rep.RulesAdded++
	}

	// Defaults: si el destino no tenía ninguno (estado vacío/wipe), hereda los
	// del backup. Si ya tenía, se respetan (el backup no pisa config local).
	if (cur.Defaults == Defaults{}) {
		cur.Defaults = bc.Defaults
	}

	if err := Save(home, cur); err != nil {
		return rep, err
	}
	return rep, nil
}

// applyProfile escribe a disco un perfil del backup: lo registra en cur,
// materializa su overlay y secretos (si vienen), y re-siembra los symlinks del
// cc-home. No llama a Save (el caller lo hace una vez al final).
func applyProfile(home string, ba *backupArchive, bc *Config, name string, cur *Config) error {
	if name == "default" {
		return nil // implícito
	}
	p, ok := bc.Profiles[name]
	if !ok {
		return fmt.Errorf("perfil %q en el manifest pero ausente de ccp.yaml del backup", name)
	}
	cur.Profiles[name] = p

	// Materializa overlay y secretos desde los miembros del tar.
	prefix := tarPath("profiles", name) + "/"
	for memberName, m := range ba.members {
		if memberName == "ccp.yaml" {
			continue
		}
		if !strings.HasPrefix(memberName, prefix) {
			continue
		}
		rel := strings.TrimPrefix(memberName, "profiles/"+name+"/")
		dst := filepath.Join(profileDirPath(home, name), filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("no se pudo crear directorio para %s: %w", dst, err)
		}
		mode := os.FileMode(m.mode)
		if err := os.WriteFile(dst, m.data, mode); err != nil {
			return fmt.Errorf("no se pudo escribir %s: %w", dst, err)
		}
		// Reafirma el modo (secretos deben quedar 600).
		if err := os.Chmod(dst, mode); err != nil {
			return fmt.Errorf("no se pudo aplicar permisos a %s: %w", dst, err)
		}
	}

	// Re-siembra symlinks re-seedables (plugins/commands/agents/skills).
	if err := seedCCHome(home, name); err != nil {
		return err
	}
	return nil
}

// readBackup abre el .tar.gz, parsea el manifest y carga cada miembro en
// memoria. Valida que el tar abra y el manifest exista y parsee.
func readBackup(archive string) (*backupArchive, error) {
	f, err := os.Open(archive)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir %s: %w", archive, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir gzip de %s: %w", archive, err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	ba := &backupArchive{members: map[string]member{}}
	var manData []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar corrupto en %s: %w", archive, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue // ignora directorios u otros tipos
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, tr); err != nil { //nolint:gosec // tamaños de backup acotados
			return nil, fmt.Errorf("no se pudo leer %s del tar: %w", hdr.Name, err)
		}
		if hdr.Name == manifestMemberName {
			manData = buf.Bytes()
			continue
		}
		ba.members[hdr.Name] = member{
			name: hdr.Name,
			data: buf.Bytes(),
			mode: hdr.Mode,
		}
	}

	if manData == nil {
		return nil, fmt.Errorf("el backup no contiene %s; archivo inválido", manifestMemberName)
	}
	if err := yaml.Unmarshal(manData, &ba.manifest); err != nil {
		return nil, fmt.Errorf("manifest.yaml inválido: %w", err)
	}
	return ba, nil
}

// verifyChecksums compara el sha256 de cada miembro contra el manifest. Reporta
// el primer miembro que falle (corrupción/tampering parcial). También exige que
// cada miembro esperado por el manifest esté presente.
func verifyChecksums(ba *backupArchive) error {
	for name, want := range ba.manifest.Checksums {
		m, ok := ba.members[name]
		if !ok {
			return fmt.Errorf("miembro %q listado en el manifest pero ausente del tar", name)
		}
		got := "sha256:" + sha256Hex(m.data)
		if got != want {
			return fmt.Errorf("checksum no coincide para %q: esperado %s, obtenido %s", name, want, got)
		}
	}
	return nil
}

// configFromBytes parsea el ccp.yaml del backup a un Config (sin tocar disco).
func configFromBytes(data []byte) (*Config, error) {
	c := &Config{Profiles: map[string]Profile{}}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, err
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	delete(c.Profiles, "default")
	return c, nil
}

// snapshotCurrent copia el estado actual de ccp (ccp.yaml + profiles/) a dir.
// Reversible: permite deshacer un restore. Si no hay estado, deja dir vacío.
func snapshotCurrent(home, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear snapshot %s: %w", dir, err)
	}
	// ccp.yaml.
	if data, err := os.ReadFile(yamlPath(home)); err == nil {
		if err := os.WriteFile(filepath.Join(dir, "ccp.yaml"), data, 0o600); err != nil {
			return fmt.Errorf("no se pudo copiar ccp.yaml al snapshot: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("no se pudo leer ccp.yaml para snapshot: %w", err)
	}
	// profiles/ (copia recursiva preservando modos; los symlinks se copian
	// como symlinks para no romper el re-seed). Reusa copyTree de migrate.go.
	profiles := filepath.Join(home, "profiles")
	if pathExists(profiles) {
		if err := copyTree(profiles, filepath.Join(dir, "profiles")); err != nil {
			return fmt.Errorf("no se pudo copiar profiles al snapshot: %w", err)
		}
	}
	return nil
}
