package i18n

// catalog_cloud_agent.go — prosa de `ccp cloud agent|review|policy`
// (internal/cli/cloud_agent.go). Va aparte del resto de `cli.cloud.` porque el
// archivo de al lado ya es largo, no porque el prefijo cambie.

func init() { register(catalogCloudAgent) }

var catalogCloudAgent = map[string]map[Lang]string{
	"cli.cloud.agent_nothing": {
		En: "Nothing to apply: no revision is waiting for this machine.",
		Es: "Nada que aplicar: no hay ninguna revisión esperando a este equipo.",
	},
	"cli.cloud.agent_watching": {
		En: "Watching for revisions every %s. Ctrl-C to stop.",
		Es: "Atento a las revisiones cada %s. Ctrl-C para parar.",
	},
	"cli.cloud.agent_bad_interval": {
		En: "cloud agent: --interval wants a duration like 30s, 5m or 1h, not '%s'",
		Es: "cloud agent: --interval quiere una duración como 30s, 5m o 1h, no '%s'",
	},
	"cli.cloud.agent_applied": {
		En: "%d paths applied from revision %s.",
		Es: "%d rutas aplicadas de la revisión %s.",
	},
	"cli.cloud.agent_pre": {
		En: "The previous state is in snapshot %s.",
		Es: "El estado anterior quedó en el snapshot %s.",
	},
	"cli.cloud.agent_skipped": {
		En: "Not applied: %s (%s)",
		Es: "Sin aplicar: %s (%s)",
	},
	// Motivos de un «sin aplicar» que pone el propio agente. Los del motor de
	// restauración (missing_blob, project_missing…) viven en catalog_snapshot.go
	// y se comparten con `ccp snapshot restore`.
	"cli.cloud.reason_no_delete_on_restore": {
		En: "ccp does not delete files when restoring",
		Es: "ccp no borra archivos al restaurar",
	},
	"cli.cloud.reason_no_cloud_data": {
		En: "its data is not in the cloud",
		Es: "sus datos no están en la nube",
	},
	"cli.cloud.reason_not_confirmed": {
		En: "it was not confirmed on this machine",
		Es: "no se confirmó en la máquina",
	},
	"cli.cloud.agent_conflicts": {
		En: "%d paths changed here and in the revision, so they were left alone: %s",
		Es: "%d rutas cambiaron aquí y en la revisión, así que se quedaron como estaban: %s",
	},
	"cli.cloud.agent_waiting": {
		En: "%d changes run code on this machine: confirm them with ccp cloud review.",
		Es: "%d cambios ejecutan código en este equipo: confírmalos con ccp cloud review.",
	},
	"cli.cloud.agent_state": {
		En: "Reported to the portal: %s",
		Es: "Informado al portal: %s",
	},
	"cli.cloud.rev_applied":    {En: "applied", Es: "aplicada"},
	"cli.cloud.rev_partial":    {En: "partial", Es: "parcial"},
	"cli.cloud.rev_conflict":   {En: "conflict", Es: "en conflicto"},
	"cli.cloud.rev_failed":     {En: "failed", Es: "fallida"},
	"cli.cloud.rev_superseded": {En: "superseded", Es: "sustituida"},
	"cli.cloud.rev_pending":    {En: "pending", Es: "pendiente"},
	"cli.cloud.rev_revoked":    {En: "device revoked", Es: "equipo revocado"},

	"cli.cloud.review_nothing": {
		En: "Nothing to confirm.",
		Es: "No hay nada que confirmar.",
	},
	"cli.cloud.review_header": {
		En: "Revision %s: %d changes need your confirmation before they run here.",
		Es: "Revisión %s: %d cambios necesitan tu confirmación antes de ejecutarse aquí.",
	},
	"cli.cloud.review_item": {En: "%s — %s", Es: "%s — %s"},
	"cli.cloud.review_conflicts": {
		En: "%d paths clash with local changes. They are resolved from the portal or the app, not here:",
		Es: "%d rutas chocan con cambios locales. Se resuelven desde el portal o la app, no aquí:",
	},
	"cli.cloud.review_prompt": {
		En: "Apply %s (%s)? [y/N] ",
		Es: "¿Aplicar %s (%s)? [s/N] ",
	},
	"cli.cloud.review_need_answer": {
		En: "cloud review: no terminal to ask in. Use --yes to accept everything or --reject to refuse it.",
		Es: "cloud review: no hay terminal donde preguntar. Usa --yes para aceptarlo todo o --reject para rechazarlo.",
	},
	"cli.cloud.review_both": {
		En: "cloud review: --yes and --reject say opposite things; pick one.",
		Es: "cloud review: --yes y --reject dicen lo contrario; elige uno.",
	},
	"cli.cloud.review_superseded": {
		En: "That revision is no longer the current one: the portal replaced it. Nothing was applied; the new one will arrive on the next pass.",
		Es: "Esa revisión ya no es la vigente: el portal la sustituyó. No se aplicó nada; la nueva llegará en la próxima pasada.",
	},

	"cli.cloud.why_hooks":       {En: "it installs a hook", Es: "instala un hook"},
	"cli.cloud.why_mcp":         {En: "it changes what an MCP server runs", Es: "cambia lo que ejecuta un servidor MCP"},
	"cli.cloud.why_status_line": {En: "it changes the statusLine command", Es: "cambia el comando de statusLine"},
	"cli.cloud.why_permissions": {En: "it widens permissions", Es: "amplía permisos"},
	"cli.cloud.why_plugins":     {En: "it installs plugins", Es: "instala plugins"},
	"cli.cloud.why_script":      {En: "it is a script", Es: "es un script"},
	"cli.cloud.why_policy":      {En: "this machine confirms everything", Es: "este equipo lo confirma todo"},

	"cli.cloud.policy_auto":   {En: "auto (anything that does not run code is applied on its own)", Es: "auto (lo que no ejecuta código se aplica solo)"},
	"cli.cloud.policy_manual": {En: "manual (nothing is applied without confirming it)", Es: "manual (nada se aplica sin confirmarlo)"},
	"cli.cloud.policy_now":    {En: "This machine's policy: %s", Es: "Política de este equipo: %s"},
	"cli.cloud.policy_set":    {En: "Policy changed: %s", Es: "Política cambiada: %s"},
	"cli.cloud.policy_bad":    {En: "cloud policy: it is auto or manual, not '%s'", Es: "cloud policy: es auto o manual, no '%s'"},
	"cli.cloud.status_policy": {En: "Policy:   %s", Es: "Política: %s"},
}
