package core

import (
	"fmt"
	"strings"
)

// statusJSONEsc replica _json_esc de bin/ccp: escapa \ → \\ y " → \"
// en ese orden exacto. No usa encoding/json para coincidir byte a byte
// con el oráculo bash.
func statusJSONEsc(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// statusHR es la línea divisoria que emite hr() con NO_COLOR / non-TTY.
const statusHR = "──────────────────────────────────────────────"

// StatusJSON devuelve exactamente:
//
//	{"active":"<active>","profile":"<profile>","profile_type":"<profileType>","cwd":"<cwd>","repo":"<repo>"}
//
// con cada valor pasado por statusJSONEsc, replicando el printf de cmd_status
// con --json del oráculo bash. No lleva newline final (el caller decide).
func StatusJSON(active, profile, profileType, cwd, repo string) string {
	return fmt.Sprintf(
		`{"active":"%s","profile":"%s","profile_type":"%s","cwd":"%s","repo":"%s"}`,
		statusJSONEsc(active),
		statusJSONEsc(profile),
		statusJSONEsc(profileType),
		statusJSONEsc(cwd),
		statusJSONEsc(repo),
	)
}

// StatusHuman devuelve la representación en texto plano (NO_COLOR / non-TTY)
// del bloque hr/printf de cmd_status, replicando el oráculo bash.
// El string resultante termina con \n (la última hr incluye su propia newline).
func StatusHuman(active, profile, profileType, cwd, repo string) string {
	repoDisplay := repo
	if repoDisplay == "" {
		repoDisplay = "no es git"
	}

	var b strings.Builder
	b.WriteString(statusHR)
	b.WriteByte('\n')
	b.WriteString(" Estado de ccp en esta terminal")
	b.WriteByte('\n')
	b.WriteString(statusHR)
	b.WriteByte('\n')
	fmt.Fprintf(&b, " Perfil activo (terminal): %s\n", active)
	fmt.Fprintf(&b, " Perfil del cwd (regla):   %s  (%s)\n", profile, profileType)
	fmt.Fprintf(&b, " Cwd:                      %s\n", cwd)
	fmt.Fprintf(&b, " Repo:                     %s\n", repoDisplay)
	b.WriteString(statusHR)
	b.WriteByte('\n')
	return b.String()
}
