package core

// instruct_manifest.go — el manifiesto de artefactos creados por ccp, split por
// localidad (asimetría a propósito, ver docs/adr/0005 y plan §4):
//
//   global/profile -> ccp.yaml .authored (machine-local, vía Load/Save).
//                     Profile solo se rellena cuando Scope == "profile".
//   project        -> <repo>/.claude/ccp-authored.tsv, versionado con el repo.
//                     ccp NO reescribe los archivos del repo del usuario: el
//                     TSV es append-only para add; rm reescribe SOLO ese TSV
//                     (es archivo de ccp dentro del repo, no del usuario).
//
// Formato TSV por fila: scope<TAB>profile<TAB>type<TAB>ref<TAB>desc
//   profile = "-" salvo scope=profile.

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// manifestEntry es una entrada del manifiesto, agnóstica del backend (ccp.yaml
// o TSV). Espeja el struct Authored de store.go más el orden estable.
type manifestEntry struct {
	Scope   string
	Profile string // "-" salvo scope=profile
	Type    string
	Ref     string
	Desc    string
}

// projectManifestFile devuelve <repoRoot>/.claude/ccp-authored.tsv.
func projectManifestFile(repoRoot string) string {
	return filepath.Join(repoRoot, ".claude", "ccp-authored.tsv")
}

// manifestAdd registra una entrada. Para global/profile escribe en ccp.yaml
// (Load/Save, atómico+flock). Para project hace append al TSV versionado.
func manifestAdd(home, repoRoot string, e manifestEntry) error {
	switch e.Scope {
	case "global", "profile":
		c, err := Load(home)
		if err != nil {
			return err
		}
		prof := ""
		if e.Scope == "profile" {
			prof = e.Profile
		}
		c.Authored = append(c.Authored, Authored{
			Scope:   e.Scope,
			Profile: prof,
			Type:    e.Type,
			Ref:     e.Ref,
			Desc:    e.Desc,
		})
		return Save(home, c)
	case "project":
		m := projectManifestFile(repoRoot)
		if err := os.MkdirAll(filepath.Dir(m), 0o755); err != nil {
			return fmt.Errorf("no se pudo crear directorio de %s: %w", m, err)
		}
		f, err := os.OpenFile(m, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("no se pudo abrir %s: %w", m, err)
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n", e.Scope, "-", e.Type, e.Ref, e.Desc)
		return err
	default:
		return fmt.Errorf("scope inválido: %q", e.Scope)
	}
}

// manifestList devuelve las entradas que matchean (scope[,profile]) en orden de
// inserción. Backend según scope.
func manifestList(home, repoRoot, scope, profile string) ([]manifestEntry, error) {
	switch scope {
	case "global", "profile":
		c, err := Load(home)
		if err != nil {
			return nil, err
		}
		var out []manifestEntry
		for _, a := range c.Authored {
			if a.Scope != scope {
				continue
			}
			if scope == "profile" && a.Profile != profile {
				continue
			}
			p := a.Profile
			if p == "" {
				p = "-"
			}
			out = append(out, manifestEntry{
				Scope: a.Scope, Profile: p, Type: a.Type, Ref: a.Ref, Desc: a.Desc,
			})
		}
		return out, nil
	case "project":
		return readProjectManifest(projectManifestFile(repoRoot))
	default:
		return nil, fmt.Errorf("scope inválido: %q", scope)
	}
}

// readProjectManifest parsea el TSV versionado. Archivo inexistente => vacío.
func readProjectManifest(file string) ([]manifestEntry, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("no se pudo leer %s: %w", file, err)
	}
	var out []manifestEntry
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 5)
		for len(f) < 5 {
			f = append(f, "")
		}
		out = append(out, manifestEntry{
			Scope: f[0], Profile: f[1], Type: f[2], Ref: f[3], Desc: f[4],
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("no se pudo escanear %s: %w", file, err)
	}
	return out, nil
}

// manifestRm borra la idx-ésima (1-based) entrada que matchea (scope[,profile])
// y devuelve la entrada borrada. Reescribe el backend correspondiente.
func manifestRm(home, repoRoot, scope, profile string, idx int) (manifestEntry, error) {
	switch scope {
	case "global", "profile":
		c, err := Load(home)
		if err != nil {
			return manifestEntry{}, err
		}
		matchN := 0
		removeAt := -1
		for i, a := range c.Authored {
			if a.Scope != scope {
				continue
			}
			if scope == "profile" && a.Profile != profile {
				continue
			}
			matchN++
			if matchN == idx {
				removeAt = i
				break
			}
		}
		if removeAt < 0 {
			return manifestEntry{}, fmt.Errorf("índice fuera de rango: %d", idx)
		}
		a := c.Authored[removeAt]
		c.Authored = append(c.Authored[:removeAt], c.Authored[removeAt+1:]...)
		if err := Save(home, c); err != nil {
			return manifestEntry{}, err
		}
		p := a.Profile
		if p == "" {
			p = "-"
		}
		return manifestEntry{Scope: a.Scope, Profile: p, Type: a.Type, Ref: a.Ref, Desc: a.Desc}, nil
	case "project":
		m := projectManifestFile(repoRoot)
		entries, err := readProjectManifest(m)
		if err != nil {
			return manifestEntry{}, err
		}
		if idx < 1 || idx > len(entries) {
			return manifestEntry{}, fmt.Errorf("índice fuera de rango: %d", idx)
		}
		removed := entries[idx-1]
		entries = append(entries[:idx-1], entries[idx:]...)
		var buf bytes.Buffer
		for _, e := range entries {
			fmt.Fprintf(&buf, "%s\t%s\t%s\t%s\t%s\n", e.Scope, "-", e.Type, e.Ref, e.Desc)
		}
		if err := os.WriteFile(m, buf.Bytes(), 0o644); err != nil {
			return manifestEntry{}, fmt.Errorf("no se pudo escribir %s: %w", m, err)
		}
		return removed, nil
	default:
		return manifestEntry{}, fmt.Errorf("scope inválido: %q", scope)
	}
}
