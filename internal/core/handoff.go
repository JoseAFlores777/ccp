package core

import (
	"fmt"
	"os"
	"time"
)

// handoff.go — orquesta el round-trip. Devuelve el string EMIT (eval-able):
// el delta de env del perfil objetivo + una línea CCP_RESUME_ID=<uuid>. La
// shell function hace `( eval "$emit"; claude --resume "$CCP_RESUME_ID" )`.

// HandoffForward valida, copia la sesión origen→destino (mismo uuid), escribe el
// marcador activo y devuelve el emit con el env del DESTINO. Regla 1-nivel:
// falla si ya hay un handoff activo.
func HandoffForward(home, from, to, cwd, sessionUUID string, writeMarker bool, now time.Time) (string, error) {
	if from == to {
		return "", fmt.Errorf("el perfil destino es el mismo que el origen (%s)", to)
	}
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	if to != "default" {
		if _, ok := cfg.Profiles[to]; !ok {
			return "", fmt.Errorf("perfil destino desconocido: %s", to)
		}
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	if h.Active != nil {
		return "", fmt.Errorf("ya hay un handoff activo (%s → %s); termínalo con `ccp handoff end`", h.Active.From, h.Active.To)
	}

	slug := SlugForCwd(cwd)
	fromCC, err := CCHome(home, from)
	if err != nil {
		return "", err
	}
	toCC, err := CCHome(home, to)
	if err != nil {
		return "", err
	}
	srcPath := ProjectDir(fromCC, slug) + "/" + sessionUUID + ".jsonl"
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("no encuentro la sesión %s en %s", sessionUUID, from)
	}
	if _, err := CopyTranscript(srcPath, ProjectDir(toCC, slug), false); err != nil {
		return "", err
	}

	if writeMarker {
		h.Active = &Marker{
			Session: sessionUUID, Slug: slug, Cwd: cwd,
			From: from, To: to,
			Title: readAITitle(srcPath),
			Since: now.UTC().Format(time.RFC3339),
		}
		if err := SaveHandoffs(home, h); err != nil {
			return "", err
		}
	}
	return EnvDelta(home, to, cfg) + "CCP_RESUME_ID=" + sessionUUID + "\n", nil
}

// HandoffEnd toma el marcador activo, hace back-sync del transcript (que creció
// en el destino) hacia el origen como una sesión NUEVA (uuid nuevo, sessionId
// reescrito, aiTitle prefijado con el origen), archiva el marcador y devuelve el
// emit con el env del ORIGEN + el uuid nuevo. No destructivo: ni el original del
// origen ni el del destino se borran.
func HandoffEnd(home, cwd string, now time.Time) (string, error) {
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	if h.Active == nil {
		return "", fmt.Errorf("no hay handoff activo que terminar")
	}
	m := h.Active

	toCC, err := CCHome(home, m.To)
	if err != nil {
		return "", err
	}
	fromCC, err := CCHome(home, m.From)
	if err != nil {
		return "", err
	}
	srcPath := ProjectDir(toCC, m.Slug) + "/" + m.Session + ".jsonl"
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("no encuentro la sesión %s en %s; el marcador queda activo", m.Session, m.To)
	}
	newID, err := NewUUID()
	if err != nil {
		return "", err
	}
	dstPath := ProjectDir(fromCC, m.Slug) + "/" + newID + ".jsonl"
	if err := RewriteSession(srcPath, dstPath, m.Session, newID, m.To); err != nil {
		return "", err // RewriteSession ya validó; no se archiva el marcador
	}

	h.Archived = append(h.Archived, ArchivedMarker{
		Session: m.Session, From: m.From, To: m.To,
		ReturnedAs: newID, Since: m.Since, Ended: now.UTC().Format(time.RFC3339),
	})
	h.Active = nil
	if err := SaveHandoffs(home, h); err != nil {
		return "", err
	}
	return EnvDelta(home, m.From, cfg) + "CCP_RESUME_ID=" + newID + "\n", nil
}
