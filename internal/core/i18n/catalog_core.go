package i18n

// catalog_core.go registra las pocas keys de prosa que viven en core (no en
// cli/tui): superficies de contrato con cfg en alcance. La key del aviso de
// _env está protegida por el golden (la parity corre con CCP_LANG=es y exige
// bytes idénticos al oráculo bash), así que su ES debe ser byte-a-byte exacto.
func init() {
	register(map[string]map[Lang]string{
		"core.env.profile_missing": {
			En: "⚠️  ccp: profile %s not found; using default",
			Es: "⚠️  ccp: perfil %s no existe; usando default",
		},

		// --- status.go: StatusHuman (non-TTY / NO_COLOR) ---
		"status.header": {
			En: " ccp status in this terminal",
			Es: " Estado de ccp en esta terminal",
		},
		"status.not_git": {
			En: "not git",
			Es: "no es git",
		},
		"status.active": {
			En: " Active profile (terminal): %s\n",
			Es: " Perfil activo (terminal): %s\n",
		},
		"status.rule": {
			En: " Cwd profile (rule):        %s  (%s)\n",
			Es: " Perfil del cwd (regla):   %s  (%s)\n",
		},
		"status.cwd": {
			En: " Cwd:                       %s\n",
			Es: " Cwd:                      %s\n",
		},
		"status.repo": {
			En: " Repo:                      %s\n",
			Es: " Repo:                     %s\n",
		},

		// --- doctor.go: Doctor check labels ---
		"doctor.path_found": {
			En: "%s: found in PATH.",
			Es: "%s: encontrado en PATH.",
		},
		"doctor.path_missing": {
			En: "%s: not found in PATH.",
			Es: "%s: no encontrado en PATH.",
		},
		"doctor.official_logged": {
			En: "Profile '%s' (official): logged in.",
			Es: "Perfil '%s' (oficial): logueado.",
		},
		"doctor.official_nologin": {
			En: "Profile '%s' (official): NO login (ccp profile login %s).",
			Es: "Perfil '%s' (oficial): SIN login (ccp profile login %s).",
		},
		"doctor.deepseek_keyok": {
			En: "Profile '%s' (deepseek): key OK.",
			Es: "Perfil '%s' (deepseek): key OK.",
		},
		"doctor.deepseek_nokey": {
			En: "Profile '%s' (deepseek): NO key (ccp key %s).",
			Es: "Perfil '%s' (deepseek): SIN key (ccp key %s).",
		},
	})
}
