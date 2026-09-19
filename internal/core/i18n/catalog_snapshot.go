package i18n

// catalog_snapshot.go — prosa de `ccp snapshot` y del snapshot automático
// (internal/cli/snapshot.go). Prefijo `cli.snapshot.` y nada más.

func init() { register(catalogSnapshot) }

var catalogSnapshot = map[string]map[Lang]string{
	"cli.snapshot.usage": {
		En: `Usage: ccp snapshot <subcommand>

  create [-m <label>] [--with-state] [--json]    take a snapshot of the whole configuration
  list [--json]                                  history, newest first
  show <id> [--json]                             what a snapshot contains
  diff <id> [<id>] [--json]                      changes from a snapshot to another (or to now)
  restore <id> [--only <path>]... [--dry-run | --yes] [--json]
                                                 bring the configuration back to a snapshot
  pin <id> [-m <label>] | unpin <id>             keep a snapshot forever (or stop keeping it)
  prune [--dry-run] [--json]                     drop old snapshots (keeps 7 days, 4 weeks, 6 months)
  export <id> <file.ccpsnap> [--with-secrets]    one portable file (secrets need a passphrase)
  import <file.ccpsnap>                          add an exported snapshot to this machine

<id> is "latest", a full id or at least its first 4 characters.
--with-state also captures conversations and loans (large and sensitive).
CCP_SNAPSHOT_PASSPHRASE gives the passphrase without asking.`,
		Es: `Uso: ccp snapshot <subcomando>

  create [-m <etiqueta>] [--with-state] [--json] guarda un snapshot de toda la configuración
  list [--json]                                  el histórico, del más nuevo al más viejo
  show <id> [--json]                             qué contiene un snapshot
  diff <id> [<id>] [--json]                      cambios de un snapshot a otro (o a ahora)
  restore <id> [--only <ruta>]... [--dry-run | --yes] [--json]
                                                 devuelve la configuración a un snapshot
  pin <id> [-m <etiqueta>] | unpin <id>          conserva un snapshot para siempre (o deja de hacerlo)
  prune [--dry-run] [--json]                     borra los viejos (conserva 7 días, 4 semanas, 6 meses)
  export <id> <archivo.ccpsnap> [--with-secrets] un solo archivo portable (los secretos piden una frase)
  import <archivo.ccpsnap>                       añade a esta máquina un snapshot exportado

<id> es «latest», un id completo o al menos sus 4 primeros caracteres.
--with-state captura también conversaciones y préstamos (pesan y son sensibles).
CCP_SNAPSHOT_PASSPHRASE da la frase sin preguntarla.`,
	},
	"cli.snapshot.unknown_sub": {En: "snapshot: unknown subcommand '%s'", Es: "snapshot: subcomando desconocido '%s'"},
	"cli.snapshot.unknown_opt": {En: "snapshot: unknown option or extra argument '%s'", Es: "snapshot: opción desconocida o argumento de más '%s'"},
	"cli.snapshot.need_id":     {En: "snapshot: this needs a snapshot id (or «latest»)", Es: "snapshot: hace falta el id de un snapshot (o «latest»)"},
	"cli.snapshot.need_file":   {En: "snapshot: this needs a file", Es: "snapshot: hace falta un archivo"},
	"cli.snapshot.created": {
		En: "Snapshot %s saved (%d items, %d secrets sealed).",
		Es: "Snapshot %s guardado (%d elementos, %d secretos sellados).",
	},
	"cli.snapshot.unchanged": {
		En: "No changes: the latest snapshot (%s) already matches your configuration.",
		Es: "Sin cambios: el último snapshot (%s) ya refleja tu configuración.",
	},
	"cli.snapshot.list_empty":     {En: "No snapshots yet. Take one with: ccp snapshot create", Es: "Aún no hay snapshots. Toma uno con: ccp snapshot create"},
	"cli.snapshot.list_header":    {En: "ID\tDATE\tTRIGGER\tITEMS\tLABEL", Es: "ID\tFECHA\tDISPARADOR\tELEMENTOS\tETIQUETA"},
	"cli.snapshot.pinned_mark":    {En: "[pinned]", Es: "[fijado]"},
	"cli.snapshot.show_header":    {En: "Snapshot %s · %s · %s · %s", Es: "Snapshot %s · %s · %s · %s"},
	"cli.snapshot.show_parent":    {En: "after %s", Es: "después de %s"},
	"cli.snapshot.show_label":     {En: "label: %s", Es: "etiqueta: %s"},
	"cli.snapshot.class_authored": {En: "config", Es: "config"},
	"cli.snapshot.class_secret":   {En: "secret", Es: "secreto"},
	"cli.snapshot.class_state":    {En: "state", Es: "estado"},
	"cli.snapshot.diff_none":      {En: "No differences.", Es: "Sin diferencias."},

	"cli.snapshot.restore_plan": {En: "Restore plan for %s:", Es: "Plan para restaurar %s:"},
	"cli.snapshot.act_write":    {En: "write", Es: "escribir"},
	"cli.snapshot.act_merge":    {En: "merge", Es: "fusionar"},
	"cli.snapshot.act_skip":     {En: "skip", Es: "omitir"},
	"cli.snapshot.reason_missing_blob": {
		En: "the snapshot has no data for it (exported without secrets?)",
		Es: "el snapshot no tiene sus datos (¿se exportó sin secretos?)",
	},
	"cli.snapshot.reason_project_missing": {
		En: "the project folder does not exist on this machine",
		Es: "la carpeta del proyecto no existe en esta máquina",
	},
	"cli.snapshot.reason_invalid":    {En: "not a path ccp knows how to restore", Es: "no es una ruta que ccp sepa restaurar"},
	"cli.snapshot.reason_unreadable": {En: "the current file could not be read", Es: "no se pudo leer el archivo actual"},
	"cli.snapshot.restore_same":      {En: "%d already identical.", Es: "%d ya idénticos."},
	"cli.snapshot.restore_nothing": {
		En: "Nothing to restore: everything already matches the snapshot.",
		Es: "Nada que restaurar: todo coincide ya con el snapshot.",
	},
	"cli.snapshot.restore_confirm": {
		En: "Nothing was changed. Run it again with --yes to apply it (or --dry-run to just see it).",
		Es: "No se cambió nada. Ejecútalo de nuevo con --yes para aplicarlo (o con --dry-run para solo verlo).",
	},
	"cli.snapshot.restored": {
		En: "Restored. Your previous configuration is in snapshot %s.",
		Es: "Restaurado. La configuración anterior quedó en el snapshot %s.",
	},
	"cli.snapshot.regenerated": {En: "Profiles regenerated: %s", Es: "Perfiles regenerados: %s"},

	"cli.snapshot.pruned": {
		En: "Deleted %d snapshots and %d unused blobs; %d kept.",
		Es: "Borrados %d snapshots y %d blobs sin uso; se conservan %d.",
	},
	"cli.snapshot.prune_dry": {
		En: "Would delete %d snapshots and %d unused blobs; %d would be kept.",
		Es: "Se borrarían %d snapshots y %d blobs sin uso; se conservarían %d.",
	},
	"cli.snapshot.pinned":   {En: "Snapshot %s pinned: prune will never delete it.", Es: "Snapshot %s fijado: prune nunca lo borrará."},
	"cli.snapshot.unpinned": {En: "Snapshot %s unpinned.", Es: "Snapshot %s ya no está fijado."},

	"cli.snapshot.exported": {En: "Snapshot %s exported to %s (without secrets).", Es: "Snapshot %s exportado a %s (sin secretos)."},
	"cli.snapshot.exported_secrets": {
		En: "Snapshot %s exported to %s; secrets sealed with your passphrase.",
		Es: "Snapshot %s exportado a %s; los secretos van sellados con tu frase.",
	},
	"cli.snapshot.imported": {En: "Snapshot %s imported (%d items).", Es: "Snapshot %s importado (%d elementos)."},
	"cli.snapshot.imported_missing": {
		En: "%d items came without data (the file was exported without secrets); they cannot be restored.",
		Es: "%d elementos llegaron sin datos (el archivo se exportó sin secretos); no se podrán restaurar.",
	},
	"cli.snapshot.pass_prompt":   {En: "Passphrase: ", Es: "Frase: "},
	"cli.snapshot.pass_confirm":  {En: "Repeat it: ", Es: "Repítela: "},
	"cli.snapshot.pass_mismatch": {En: "the passphrases do not match", Es: "las frases no coinciden"},
	"cli.snapshot.pass_short":    {En: "the passphrase needs at least %d characters", Es: "la frase necesita al menos %d caracteres"},
	"cli.snapshot.pass_needed": {
		En: "a passphrase is needed: set CCP_SNAPSHOT_PASSPHRASE or run it in a terminal",
		Es: "hace falta una frase: define CCP_SNAPSHOT_PASSPHRASE o ejecútalo en una terminal",
	},

	"cli.snapshot.auto_saved": {En: "Safety snapshot: %s", Es: "Snapshot de seguridad: %s"},
	"cli.snapshot.auto_failed": {
		En: "Could not save the safety snapshot, so nothing was done: %v (CCP_NO_AUTO_SNAPSHOT=1 skips it)",
		Es: "No se pudo guardar el snapshot de seguridad y no se hizo nada: %v (CCP_NO_AUTO_SNAPSHOT=1 lo salta)",
	},
}
