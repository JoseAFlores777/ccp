package core

import (
	"fmt"
	"os"
	"path/filepath"

	yaml "github.com/goccy/go-yaml"
	"golang.org/x/sys/unix"
)

// SchemaVersion es la versión de schema de ccp.yaml que este binario conoce.
// Si el archivo en disco declara una version mayor, Load aborta y Save no se
// permite: nunca truncamos data escrita por un binario más nuevo.
const SchemaVersion = 2

// Defaults reproduce el ex-archivo `config`: solo siembra perfiles nuevos.
// Editar estos valores NO muta perfiles existentes.
type Defaults struct {
	BaseURL    string `yaml:"base_url"`
	ModelPro   string `yaml:"model_pro"`
	ModelFlash string `yaml:"model_flash"`
	Effort     string `yaml:"effort"`
	Editor     string `yaml:"editor"`
}

// Profile es un perfil con nombre. 'default' es IMPLÍCITO y nunca se serializa.
// Los perfiles official solo llevan Type; los deepseek llevan los 4 campos
// explícitos (no heredan de Defaults). api_key NUNCA va aquí.
type Profile struct {
	Type       string `yaml:"type"`
	BaseURL    string `yaml:"base_url,omitempty"`
	ModelPro   string `yaml:"model_pro,omitempty"`
	ModelFlash string `yaml:"model_flash,omitempty"`
	Effort     string `yaml:"effort,omitempty"`
}

// Rule asocia un path absoluto normalizado (único) a un perfil.
type Rule struct {
	Path    string `yaml:"path"`
	Profile string `yaml:"profile"`
}

// Authored es una entrada del manifiesto de artefactos gestionados por ccp
// (solo scope global+profile; project vive versionado en el repo). Profile
// solo está presente cuando Scope == "profile".
type Authored struct {
	Scope   string `yaml:"scope"`
	Profile string `yaml:"profile,omitempty"`
	Type    string `yaml:"type"`
	Ref     string `yaml:"ref"`
	Desc    string `yaml:"desc"`
}

// Config es el modelo en memoria de ccp.yaml: la fuente de verdad única.
// Extra captura claves desconocidas de nivel superior para no perderlas en
// round-trip (forward-compat). comments preserva los comentarios por-path.
type Config struct {
	Version  int                `yaml:"version"`
	Defaults Defaults           `yaml:"defaults"`
	Profiles map[string]Profile `yaml:"profiles"`
	Rules    []Rule             `yaml:"rules"`
	Authored []Authored         `yaml:"authored"`

	// Extra: catch-all para claves de nivel superior que este binario no
	// conoce. Se preservan tal cual en el round-trip.
	Extra map[string]any `yaml:",inline"`

	// comments preserva los comentarios capturados al cargar para re-adjuntarlos
	// al guardar. No se serializa como campo YAML.
	comments yaml.CommentMap `yaml:"-"`
}

// knownTopKeys son las claves de nivel superior que el struct maneja
// explícitamente; se filtran del catch-all Extra para evitar redundancia.
var knownTopKeys = map[string]struct{}{
	"version":  {},
	"defaults": {},
	"profiles": {},
	"rules":    {},
	"authored": {},
}

func yamlPath(home string) string { return filepath.Join(home, "ccp.yaml") }

// Load lee <home>/ccp.yaml. Si no existe, devuelve un Config vacío con
// Version = SchemaVersion (sin error: arranque limpio). Aborta si la version
// del archivo es mayor a la conocida por este binario.
func Load(home string) (*Config, error) {
	path := yamlPath(home)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				Version:  SchemaVersion,
				Profiles: map[string]Profile{},
				comments: yaml.CommentMap{},
			}, nil
		}
		return nil, fmt.Errorf("no se pudo leer %s: %w", path, err)
	}

	c := &Config{Profiles: map[string]Profile{}}
	cm := yaml.CommentMap{}
	if err := yaml.UnmarshalWithOptions(data, c, yaml.CommentToMap(cm)); err != nil {
		return nil, fmt.Errorf("ccp.yaml inválido (%s): %w", path, err)
	}
	c.comments = cm

	if c.Version > SchemaVersion {
		return nil, fmt.Errorf(
			"ccp.yaml usa schema version %d pero este ccp solo conoce hasta %d; "+
				"actualiza ccp (no se tocará el archivo)", c.Version, SchemaVersion)
	}

	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	// El inline map de goccy captura también las claves conocidas; las
	// quitamos para que Extra solo contenga lo realmente desconocido.
	stripKnownKeys(c.Extra)
	// 'default' es implícito: nunca debe vivir en el mapa.
	delete(c.Profiles, "default")
	return c, nil
}

// Save escribe <home>/ccp.yaml de forma atómica (tmp + rename) bajo un flock
// advisory. Re-adjunta los comentarios preservados. Aborta si la version del
// Config es mayor a la conocida. No requiere time.Now: lógica pura.
func Save(home string, c *Config) error {
	if c.Version > SchemaVersion {
		return fmt.Errorf(
			"Config declara schema version %d > %d conocida; no se escribirá",
			c.Version, SchemaVersion)
	}
	if c.Version == 0 {
		c.Version = SchemaVersion
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", home, err)
	}

	// 'default' nunca se serializa; api_key nunca está en el modelo.
	if c.Profiles != nil {
		delete(c.Profiles, "default")
	}
	stripKnownKeys(c.Extra)

	var out []byte
	var err error
	if len(c.comments) > 0 {
		out, err = yaml.MarshalWithOptions(c, yaml.WithComment(c.comments))
	} else {
		out, err = yaml.Marshal(c)
	}
	if err != nil {
		return fmt.Errorf("no se pudo serializar ccp.yaml: %w", err)
	}

	// Lock advisory sobre un archivo dedicado para evitar lost-update entre
	// terminales concurrentes.
	unlock, err := acquireLock(home)
	if err != nil {
		return err
	}
	defer unlock()

	path := yamlPath(home)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("no se pudo renombrar %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// acquireLock toma un flock advisory exclusivo sobre <home>/.ccp.lock y
// devuelve la función para liberarlo.
func acquireLock(home string) (func(), error) {
	lockPath := filepath.Join(home, ".ccp.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir lock %s: %w", lockPath, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("no se pudo bloquear %s: %w", lockPath, err)
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}

// stripKnownKeys elimina del catch-all las claves que el struct ya maneja.
func stripKnownKeys(m map[string]any) {
	if m == nil {
		return
	}
	for k := range knownTopKeys {
		delete(m, k)
	}
}
