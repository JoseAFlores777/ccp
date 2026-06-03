package core

import (
	"fmt"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
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
func StatusHuman(l i18n.Lang, active, profile, profileType, cwd, repo string) string {
	repoDisplay := repo
	if repoDisplay == "" {
		repoDisplay = i18n.T(l, "status.not_git")
	}

	var b strings.Builder
	b.WriteString(statusHR)
	b.WriteByte('\n')
	b.WriteString(i18n.T(l, "status.header"))
	b.WriteByte('\n')
	b.WriteString(statusHR)
	b.WriteByte('\n')
	b.WriteString(i18n.T(l, "status.active", active))
	b.WriteString(i18n.T(l, "status.rule", profile, profileType))
	b.WriteString(i18n.T(l, "status.cwd", cwd))
	b.WriteString(i18n.T(l, "status.repo", repoDisplay))
	b.WriteString(statusHR)
	b.WriteByte('\n')
	return b.String()
}
