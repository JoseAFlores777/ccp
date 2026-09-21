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
		"doctor.provider_keyok": {
			En: "Profile '%s' (%s): key OK.",
			Es: "Perfil '%s' (%s): key OK.",
		},
		"doctor.provider_nokey": {
			En: "Profile '%s' (%s): NO key (ccp key %s).",
			Es: "Perfil '%s' (%s): SIN key (ccp key %s).",
		},

		// --- doctor_projection.go: hallazgos de la proyeccion (spec 6.4) ---
		"doctor.projection_stale": {
			En: "Profile '%s': what it declares is not projected where the apps read it (ccp profile sync %s).",
			Es: "Perfil '%s': lo que declara no está proyectado donde lo leen las apps (ccp profile sync %s).",
		},
		"doctor.projection_error": {
			En: "Profile '%s': the projection could not be checked (%s).",
			Es: "Perfil '%s': no se pudo comprobar la proyección (%s).",
		},
		"doctor.desktop_restart_pending": {
			En: "Profile '%s': its Desktop window has MCP waiting; only starting that window applies them.",
			Es: "Perfil '%s': su ventana de Desktop tiene MCP esperando; solo arrancar esa ventana los aplica.",
		},
		"doctor.mcp_command_missing": {
			En: "Profile '%s': the command of %s does not resolve, so that server will not start.",
			Es: "Perfil '%s': el command de %s no resuelve, así que ese servidor no arranca.",
		},
		"doctor.mcp_only_desktop": {
			En: "Profile '%s': %s live only in its Desktop chat config; declare them (ccp instruct add profile mcp … --profile %s) or they exist on no other machine.",
			Es: "Perfil '%s': %s solo viven en la config del chat de su Desktop; decláralos (ccp instruct add profile mcp … --profile %s) o no existen en ninguna otra máquina.",
		},
		"doctor.cc_home_symlink_nonleaf": {
			En: "Profile '%s': %s are directory symlinks under its cc-home and Desktop rejects them, so its Code tab will not open (ccp profile sync %s).",
			Es: "Perfil '%s': %s son symlinks de directorio bajo su cc-home y Desktop los rechaza, así que su pestaña Code no abre (ccp profile sync %s).",
		},
	})
}
