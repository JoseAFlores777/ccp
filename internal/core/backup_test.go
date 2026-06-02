package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	yaml "github.com/goccy/go-yaml"
)

// fixedTime es un timestamp determinista para los tests (no time.Now).
var fixedTime = time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

// setupHome crea un CCP_HOME temporal con un perfil deepseek (con api_key +
// overlay) y uno official (con overlay + credenciales), más una regla de path.
// Devuelve el home. CCP_CLAUDE_SRC se apunta a un dir vacío para que seedCCHome
// no cree symlinks reales en los tests.
func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := ProfileAddDeepseek(home, "deep", Defaults{
		BaseURL: "https://api.deepseek.com", ModelPro: "deepseek-chat",
		ModelFlash: "deepseek-chat", Effort: "high",
	}); err != nil {
		t.Fatalf("add deepseek: %v", err)
	}
	if err := ProfileSetKey(home, "deep", "sk-secret-123"); err != nil {
		t.Fatalf("set key: %v", err)
	}
	if err := CfgInitOverlay(home, "deep"); err != nil {
		t.Fatalf("overlay deep: %v", err)
	}
	if err := os.WriteFile(cfgInstrFile(home, "deep"), []byte("# deep instr\n"), 0o644); err != nil {
		t.Fatalf("write deep instr: %v", err)
	}

	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatalf("add official: %v", err)
	}
	if err := CfgInitOverlay(home, "work"); err != nil {
		t.Fatalf("overlay work: %v", err)
	}
	if err := os.WriteFile(cfgInstrFile(home, "work"), []byte("# work instr\n"), 0o644); err != nil {
		t.Fatalf("write work instr: %v", err)
	}
	// Credenciales OAuth de la cuenta official.
	credPath := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	if err := os.WriteFile(credPath, []byte(`{"oauth":"tok"}`), 0o600); err != nil {
		t.Fatalf("write cred: %v", err)
	}

	// Una regla de path.
	c, err := Load(home)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	c.Rules = []Rule{{Path: "/repo/a", Profile: "work"}}
	if err := Save(home, c); err != nil {
		t.Fatalf("save rules: %v", err)
	}
	return home
}

// readTarMembers abre un .tar.gz y devuelve un map name->data (incluye manifest).
func readTarMembers(t *testing.T, archive string) map[string][]byte {
	t.Helper()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, tr); err != nil {
			t.Fatalf("tar copy: %v", err)
		}
		out[hdr.Name] = buf.Bytes()
	}
	return out
}

// TestBackupExportConfigOnlyExcludesSecrets verifica que sin --with-secrets el
// tar incluye manifest + ccp.yaml + overlays, pero NO api_key ni credenciales.
func TestBackupExportConfigOnlyExcludesSecrets(t *testing.T) {
	home := setupHome(t)
	dest := filepath.Join(t.TempDir(), "backup.tar.gz")

	if err := BackupExport(home, dest, false, fixedTime); err != nil {
		t.Fatalf("export: %v", err)
	}
	members := readTarMembers(t, dest)

	mustHave := []string{
		"manifest.yaml", "ccp.yaml",
		"profiles/deep/overlay/CLAUDE.md", "profiles/deep/overlay/settings.overlay.json",
		"profiles/work/overlay/CLAUDE.md", "profiles/work/overlay/settings.overlay.json",
	}
	for _, m := range mustHave {
		if _, ok := members[m]; !ok {
			t.Errorf("falta miembro %q en backup config-only", m)
		}
	}
	mustNotHave := []string{
		"profiles/deep/api_key", "profiles/work/cc-home/.claude.json",
	}
	for _, m := range mustNotHave {
		if _, ok := members[m]; ok {
			t.Errorf("backup config-only NO debe incluir %q", m)
		}
	}

	// Archivo config-only: modo 0644.
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("backup config-only debe ser 0644, es %o", fi.Mode().Perm())
	}
}

// TestBackupExportWithSecrets verifica que --with-secrets incluye api_key +
// credenciales y que el archivo queda 0600.
func TestBackupExportWithSecrets(t *testing.T) {
	home := setupHome(t)
	dest := filepath.Join(t.TempDir(), "full.tar.gz")

	if err := BackupExport(home, dest, true, fixedTime); err != nil {
		t.Fatalf("export: %v", err)
	}
	members := readTarMembers(t, dest)

	if got := string(members["profiles/deep/api_key"]); got != "sk-secret-123" {
		t.Errorf("api_key en backup = %q, quiero sk-secret-123", got)
	}
	if _, ok := members["profiles/work/cc-home/.claude.json"]; !ok {
		t.Errorf("falta credencial official en backup full")
	}

	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("backup full debe ser 0600, es %o", fi.Mode().Perm())
	}

	// Manifest debe declarar with_secrets: true y sha256 por miembro.
	ba, err := readBackup(dest)
	if err != nil {
		t.Fatalf("readBackup: %v", err)
	}
	if !ba.manifest.WithSecrets {
		t.Error("manifest.with_secrets debe ser true")
	}
	for name := range ba.members {
		if _, ok := ba.manifest.Checksums[name]; !ok {
			t.Errorf("falta checksum para miembro %q", name)
		}
	}
	if _, ok := ba.manifest.Checksums["manifest.yaml"]; ok {
		t.Error("el manifest NO debe auto-incluirse en checksums")
	}
}

// TestBackupRoundTrip exporta, borra los perfiles, restaura y verifica que el
// estado se reproduce (perfiles, secretos, overlays, reglas).
func TestBackupRoundTrip(t *testing.T) {
	home := setupHome(t)
	dest := filepath.Join(t.TempDir(), "rt.tar.gz")
	if err := BackupExport(home, dest, true, fixedTime); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Estado destino limpio.
	target := t.TempDir()

	rep, err := BackupRestore(target, dest, RestoreOpts{Now: fixedTime})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(rep.Created) != 2 {
		t.Errorf("esperaba 2 perfiles creados, got %v", rep.Created)
	}

	c, err := Load(target)
	if err != nil {
		t.Fatalf("load target: %v", err)
	}
	if _, ok := c.Profiles["deep"]; !ok {
		t.Error("perfil deep no restaurado")
	}
	if _, ok := c.Profiles["work"]; !ok {
		t.Error("perfil work no restaurado")
	}
	// Secreto restaurado con 0600.
	key, ok := GetKey(target, "deep")
	if !ok || key != "sk-secret-123" {
		t.Errorf("api_key no restaurada: %q ok=%v", key, ok)
	}
	keyFi, err := os.Stat(apiKeyPath(target, "deep"))
	if err != nil {
		t.Fatal(err)
	}
	if keyFi.Mode().Perm() != 0o600 {
		t.Errorf("api_key restaurada debe ser 0600, es %o", keyFi.Mode().Perm())
	}
	// Overlay restaurado.
	instr, err := os.ReadFile(cfgInstrFile(target, "deep"))
	if err != nil || string(instr) != "# deep instr\n" {
		t.Errorf("overlay deep no restaurado: %q err=%v", instr, err)
	}
	// Regla restaurada.
	if len(c.Rules) != 1 || c.Rules[0].Path != "/repo/a" {
		t.Errorf("regla no restaurada: %v", c.Rules)
	}
	// Snapshot pre-restore creado.
	if rep.SnapshotDir == "" || !pathExists(rep.SnapshotDir) {
		t.Errorf("snapshot pre-restore ausente: %q", rep.SnapshotDir)
	}
}

// TestBackupRestoreCollisionSkip: sin --overwrite, los perfiles que ya existen
// se saltan y se reportan.
func TestBackupRestoreCollisionSkip(t *testing.T) {
	home := setupHome(t)
	dest := filepath.Join(t.TempDir(), "c.tar.gz")
	if err := BackupExport(home, dest, true, fixedTime); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Restaurar sobre el MISMO home (colisión total).
	rep, err := BackupRestore(home, dest, RestoreOpts{Now: fixedTime})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(rep.Created) != 0 {
		t.Errorf("no debería crear nada, got %v", rep.Created)
	}
	if len(rep.Skipped) != 2 {
		t.Errorf("esperaba 2 saltados, got %v", rep.Skipped)
	}
}

// TestBackupRestoreOverwrite: --overwrite reemplaza el perfil colisionado.
func TestBackupRestoreOverwrite(t *testing.T) {
	home := setupHome(t)
	dest := filepath.Join(t.TempDir(), "o.tar.gz")
	if err := BackupExport(home, dest, true, fixedTime); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Muta el overlay local de deep para detectar el reemplazo.
	if err := os.WriteFile(cfgInstrFile(home, "deep"), []byte("LOCAL CHANGED\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := BackupRestore(home, dest, RestoreOpts{Overwrite: true, Now: fixedTime})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(rep.Overwritten) != 2 {
		t.Errorf("esperaba 2 reemplazados, got %v", rep.Overwritten)
	}
	instr, _ := os.ReadFile(cfgInstrFile(home, "deep"))
	if string(instr) != "# deep instr\n" {
		t.Errorf("overlay no reemplazado por el del backup: %q", instr)
	}
}

// TestBackupRestoreForceWipe: --force limpia el estado actual y restaura solo
// lo del backup.
func TestBackupRestoreForceWipe(t *testing.T) {
	home := setupHome(t)
	dest := filepath.Join(t.TempDir(), "f.tar.gz")
	if err := BackupExport(home, dest, true, fixedTime); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Añade un perfil EXTRA que no está en el backup; --force debe eliminarlo.
	if err := ProfileAddOfficial(home, "extra"); err != nil {
		t.Fatalf("add extra: %v", err)
	}

	rep, err := BackupRestore(home, dest, RestoreOpts{Force: true, Now: fixedTime})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	c, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Profiles["extra"]; ok {
		t.Error("--force debió eliminar el perfil 'extra' ausente del backup")
	}
	if len(c.Profiles) != 2 {
		t.Errorf("tras --force esperaba 2 perfiles, got %v", c.Profiles)
	}
	if len(rep.Created) != 2 {
		t.Errorf("--force debe recrear los 2 del backup, got %v", rep.Created)
	}
}

// TestBackupRestoreChecksumMismatch: si un miembro está corrupto, restore
// aborta nombrando el miembro y NO toca el estado.
func TestBackupRestoreChecksumMismatch(t *testing.T) {
	home := setupHome(t)
	dest := filepath.Join(t.TempDir(), "tamper.tar.gz")
	if err := BackupExport(home, dest, false, fixedTime); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Reescribe el tar mutando un miembro (ccp.yaml) sin tocar el manifest.
	tamperArchive(t, dest, "profiles/deep/overlay/CLAUDE.md", []byte("TAMPERED\n"))

	target := t.TempDir()
	_, err := BackupRestore(target, dest, RestoreOpts{Now: fixedTime})
	if err == nil {
		t.Fatal("esperaba error por checksum mismatch")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("checksum no coincide")) {
		t.Errorf("error no menciona checksum: %v", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("profiles/deep/overlay/CLAUDE.md")) {
		t.Errorf("error no nombra el miembro corrupto: %v", err)
	}
}

// TestBackupRestoreSchemaAbort: si el manifest declara schema_version mayor a
// la conocida, restore aborta.
func TestBackupRestoreSchemaAbort(t *testing.T) {
	home := setupHome(t)
	dest := filepath.Join(t.TempDir(), "newer.tar.gz")
	if err := BackupExport(home, dest, false, fixedTime); err != nil {
		t.Fatalf("export: %v", err)
	}
	// Reconstruye el tar con un manifest de schema_version futura.
	bumpManifestSchema(t, dest, SchemaVersion+1)

	target := t.TempDir()
	_, err := BackupRestore(target, dest, RestoreOpts{Now: fixedTime})
	if err == nil {
		t.Fatal("esperaba abort por schema_version mayor")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("schema version")) {
		t.Errorf("error no menciona schema version: %v", err)
	}
}

// TestBackupRestoreRulesMerge: las reglas del backup que no chocan se añaden;
// las existentes se conservan.
func TestBackupRestoreRulesMerge(t *testing.T) {
	home := setupHome(t) // tiene regla /repo/a -> work
	dest := filepath.Join(t.TempDir(), "rules.tar.gz")
	if err := BackupExport(home, dest, false, fixedTime); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Destino con una regla distinta (/repo/b) y misma colisión potencial.
	target := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	c := &Config{Version: SchemaVersion, Profiles: map[string]Profile{}}
	c.Rules = []Rule{{Path: "/repo/b", Profile: "deep"}}
	if err := Save(target, c); err != nil {
		t.Fatalf("save target: %v", err)
	}

	rep, err := BackupRestore(target, dest, RestoreOpts{Now: fixedTime})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if rep.RulesAdded != 1 {
		t.Errorf("esperaba 1 regla añadida (/repo/a), got %d", rep.RulesAdded)
	}
	out, _ := Load(target)
	if len(out.Rules) != 2 {
		t.Errorf("esperaba 2 reglas tras merge, got %v", out.Rules)
	}
}

// --- helpers de manipulación de tar para tests negativos ---

// tamperArchive reescribe el .tar.gz reemplazando el contenido de un miembro,
// dejando el resto (incluido el manifest) intacto.
func tamperArchive(t *testing.T, archive, member string, newData []byte) {
	t.Helper()
	members := readTarMembers(t, archive)
	members[member] = newData
	rewriteTar(t, archive, members)
}

// bumpManifestSchema reescribe el manifest del tar con un schema_version dado,
// recalculando nada más (los checksums siguen siendo válidos para el resto).
func bumpManifestSchema(t *testing.T, archive string, schema int) {
	t.Helper()
	ba, err := readBackup(archive)
	if err != nil {
		t.Fatalf("readBackup: %v", err)
	}
	ba.manifest.SchemaVersion = schema
	manData, err := yaml.Marshal(&ba.manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	members := map[string][]byte{manifestMemberName: manData}
	for name, m := range ba.members {
		members[name] = m.data
	}
	rewriteTar(t, archive, members)
}

// rewriteTar reescribe el .tar.gz con el conjunto de miembros dado (modo 0600).
func rewriteTar(t *testing.T, archive string, members map[string][]byte) {
	t.Helper()
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, data := range members {
		hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Format: tar.FormatPAX}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
