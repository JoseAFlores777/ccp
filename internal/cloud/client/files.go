// Package client es el lado de la máquina de la nube de ccp: login por
// dispositivo contra Keycloak, el API /v1 y la sincronización de snapshots. Lo
// usa la CLI y no importa nada del servidor.
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"

	"github.com/JoseAFlores777/ccp/internal/vault"
)

var (
	// ErrNotLoggedIn: este equipo no ha iniciado sesión en la nube.
	ErrNotLoggedIn = errors.New("este equipo no ha iniciado sesión en la nube")
	// ErrLocked: la bóveda no está desbloqueada en este equipo.
	ErrLocked = errors.New("la bóveda está bloqueada en este equipo")
)

// Files son los archivos de la nube de este equipo, en <home>/cloud (0700).
// Todos 0600:
//
//	config.json  servidor, emisor, cuenta y dispositivo
//	token.json   el refresh token: la credencial del dispositivo
//	vault.key    la clave de cuenta desbloqueada
//	state.json   qué snapshots locales están ya en la nube
type Files struct{ Dir string }

// NewFiles devuelve los archivos de la nube de un CCP_HOME.
func NewFiles(home string) Files { return Files{Dir: filepath.Join(home, "cloud")} }

// Config es la sesión de este equipo.
type Config struct {
	Server     string `json:"server"`
	Issuer     string `json:"issuer"`
	ClientID   string `json:"client_id"`
	TokenURL   string `json:"token_url"`
	DeviceURL  string `json:"device_url"`
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	// Policy es la política de este dispositivo frente a las revisiones que
	// llegan del portal (spec §10.3): "auto" aplica solo lo no ejecutable,
	// "manual" no aplica nada sin confirmación. Vacío = auto.
	Policy string `json:"policy,omitempty"`
}

// State recuerda qué hay arriba: id local -> id en la nube.
type State struct {
	Pushed map[string]string `json:"pushed"`
	// Pulled: los snapshots que este equipo solo ha BAJADO. Están en Pushed
	// (para no volver a subirlos), pero su `pinned` es el que llevaba el
	// manifiesto sellado el día del push, no una opinión de esta máquina:
	// propagarlo desfijaría lo que fijó otra. Campo añadido, omitempty: un
	// state.json viejo se lee igual.
	Pulled map[string]bool `json:"pulled,omitempty"`
}

func (f Files) path(name string) string { return filepath.Join(f.Dir, name) }

func (f Files) writeRaw(name string, data []byte) error {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(f.Dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path(name))
}

func (f Files) write(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return f.writeRaw(name, append(data, '\n'))
}

func (f Files) read(name string, v any) error {
	data, err := os.ReadFile(f.path(name))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s está dañado: %w", f.path(name), err)
	}
	return nil
}

// LoadConfig lee la sesión, o ErrNotLoggedIn.
func (f Files) LoadConfig() (Config, error) {
	var c Config
	if err := f.read("config.json", &c); errors.Is(err, fs.ErrNotExist) {
		return c, ErrNotLoggedIn
	} else if err != nil {
		return c, err
	}
	return c, nil
}

// SaveConfig guarda la sesión.
func (f Files) SaveConfig(c Config) error { return f.write("config.json", c) }

// LoadToken lee el token, o ErrNotLoggedIn.
func (f Files) LoadToken() (*oauth2.Token, error) {
	var t oauth2.Token
	if err := f.read("token.json", &t); errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotLoggedIn
	} else if err != nil {
		return nil, err
	}
	return &t, nil
}

// SaveToken guarda el token.
func (f Files) SaveToken(t *oauth2.Token) error { return f.write("token.json", t) }

// LoadAK lee la clave de cuenta, o ErrLocked.
func (f Files) LoadAK() ([]byte, error) {
	data, err := os.ReadFile(f.path("vault.key"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrLocked
	}
	if err != nil {
		return nil, err
	}
	if len(data) != vault.KeySize {
		return nil, fmt.Errorf("%s está dañado", f.path("vault.key"))
	}
	return data, nil
}

// SaveAK guarda la clave de cuenta desbloqueada.
func (f Files) SaveAK(ak []byte) error { return f.writeRaw("vault.key", ak) }

// LoadState lee qué está arriba (vacío si nunca se subió nada).
func (f Files) LoadState() (State, error) {
	s := State{Pushed: map[string]string{}, Pulled: map[string]bool{}}
	if err := f.read("state.json", &s); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return s, err
	}
	if s.Pushed == nil {
		s.Pushed = map[string]string{}
	}
	if s.Pulled == nil {
		s.Pulled = map[string]bool{}
	}
	return s, nil
}

// SaveState guarda qué está arriba.
func (f Files) SaveState(s State) error { return f.write("state.json", s) }

// ForgetVault borra la clave de cuenta y lo que este equipo creía subido. Es
// lo que hay que hacer al cambiar de cuenta o de servidor: la AK abre UNA
// bóveda, y usarla contra otra cuenta sellaría y firmaría blobs que nadie —ni
// quien los subió— podrá volver a leer, marcados además como ya subidos.
func (f Files) ForgetVault() error {
	for _, name := range []string{"vault.key", "state.json", "review.json"} {
		if err := os.Remove(f.path(name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// SaveJSON y LoadJSON guardan un archivo cualquiera del directorio de nube con
// los permisos del resto (0700 el directorio, 0600 el archivo) y por
// tmp+rename. Existen para el agente, que vive en su propio paquete porque
// importa el motor de snapshots: sin esto tendría que reimplementar la
// escritura o dar la vuelta a las dependencias.
func (f Files) SaveJSON(name string, v any) error { return f.write(name, v) }

// LoadJSON devuelve false y ningún error cuando el archivo no existe: para el
// agente, «todavía no hay nada» es un estado normal, no un fallo.
func (f Files) LoadJSON(name string, v any) (bool, error) {
	err := f.read(name, v)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// RemoveJSON borra uno de esos archivos; que no esté no es un error.
func (f Files) RemoveJSON(name string) error {
	if err := os.Remove(f.path(name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Forget borra la credencial y la bóveda de este equipo, olvida su dispositivo
// y también lo que creía subido: el mapa de subidos solo vale contra la cuenta
// y la bóveda con que se llenó, y tras un logout nadie garantiza que la próxima
// sesión sea esa —volver a subir no cuesta nada, saltarse un snapshot que
// arriba no está sí—.
func (f Files) Forget() error {
	for _, name := range []string{"token.json", "vault.key", "state.json", "review.json"} {
		if err := os.Remove(f.path(name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	c, err := f.LoadConfig()
	if err != nil {
		return nil
	}
	c.DeviceID, c.DeviceName = "", ""
	return f.SaveConfig(c)
}
