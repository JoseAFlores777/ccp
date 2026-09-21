package i18n

// catalog_sync.go — prosa de `ccp sync` (internal/cli/sync.go). Prefijo
// `cli.sync.` y nada más.

func init() { register(catalogSync) }

var catalogSync = map[string]map[Lang]string{
	"cli.sync.usage": {
		En: `Usage: ccp sync <subcommand>

  remote add <name> <url>         register a folder or a bucket and open its vault
  remote list [--json]            the destinations this machine knows
  remote rm <name>                forget one here (nothing is deleted in the destination)
  push [<snapshot>] [--json]      upload the local snapshots the destination does not have
  pull [<id>|latest]              download one into the local store
  apply [<id>|latest] [--plan]    bring this machine to that snapshot; --yes writes
       [--only <lpath>] [--yes]

  --remote <name>                 which destination, when there is more than one

A destination is a folder —iCloud Drive, Dropbox, Syncthing, a NAS— or an S3 bucket:
  ccp sync remote add icloud "file:///Users/me/Library/Mobile Documents/ccp"
  ccp sync remote add r2 s3://my-bucket/ccp

Everything is sealed on this machine before it leaves: whoever operates the folder or
the bucket moves packages they cannot open. CCP_SYNC_PASSPHRASE and CCP_SYNC_RECOVERY
give the secrets without asking; the bucket's credentials come from
CCP_SYNC_S3_ACCESS_KEY / CCP_SYNC_S3_SECRET_KEY (or the AWS ones), never from the URL.`,
		Es: `Uso: ccp sync <subcomando>

  remote add <nombre> <url>       registra una carpeta o un bucket y abre su bóveda
  remote list [--json]            los destinos que conoce esta máquina
  remote rm <nombre>              lo olvida aquí (en el destino no se borra nada)
  push [<snapshot>] [--json]      sube los snapshots locales que el destino no tiene
  pull [<id>|latest]              baja uno al almacén local
  apply [<id>|latest] [--plan]    deja esta máquina en ese snapshot; --yes escribe
       [--only <ruta>] [--yes]

  --remote <nombre>               a qué destino, cuando hay más de uno

Un destino es una carpeta —iCloud Drive, Dropbox, Syncthing, un NAS— o un bucket S3:
  ccp sync remote add icloud "file:///Users/yo/Library/Mobile Documents/ccp"
  ccp sync remote add r2 s3://mi-bucket/ccp

Todo se sella en esta máquina antes de salir: quien opera la carpeta o el bucket mueve
bultos que no sabe abrir. CCP_SYNC_PASSPHRASE y CCP_SYNC_RECOVERY dan los secretos sin
preguntar; las credenciales del bucket salen de CCP_SYNC_S3_ACCESS_KEY /
CCP_SYNC_S3_SECRET_KEY (o de las de AWS), nunca de la URL.`,
	},
	"cli.sync.unknown_sub": {En: "sync: unknown subcommand '%s'", Es: "sync: subcomando desconocido '%s'"},
	"cli.sync.unknown_opt": {En: "sync: unknown option or extra argument '%s'", Es: "sync: opción desconocida o argumento de más '%s'"},
	"cli.sync.remote_sub":  {En: "sync remote: unknown subcommand '%s'", Es: "sync remote: subcomando desconocido '%s'"},
	"cli.sync.need_name":   {En: "sync remote: the destination's name is missing", Es: "sync remote: falta el nombre del destino"},
	"cli.sync.need_url": {
		En: "sync remote add: the destination's URL is missing (file:///path or s3://bucket/prefix)",
		Es: "sync remote add: falta la URL del destino (file:///ruta o s3://bucket/prefijo)",
	},
	"cli.sync.no_remotes": {
		En: "There is no destination yet. Add one with `ccp sync remote add <name> <url>`.",
		Es: "Todavía no hay ningún destino. Añade uno con `ccp sync remote add <nombre> <url>`.",
	},
	"cli.sync.many_remotes": {
		En: "there is more than one destination (%s): say which with --remote <name>",
		Es: "hay más de un destino (%s): di cuál con --remote <nombre>",
	},
	"cli.sync.no_such_remote": {En: "there is no destination named '%s'", Es: "no hay ningún destino llamado '%s'"},
	"cli.sync.remote_other_url": {
		En: "there is already a destination named '%s' pointing at %s; remove it first with `ccp sync remote rm %s`",
		Es: "ya hay un destino llamado '%s' que apunta a %s; quítalo antes con `ccp sync remote rm %s`",
	},
	"cli.sync.remote_none":    {En: "This machine has no destinations.", Es: "Esta máquina no tiene destinos."},
	"cli.sync.remote_added":   {En: "Destination '%s' ready: %s", Es: "Destino '%s' listo: %s"},
	"cli.sync.remote_removed": {En: "Destination '%s' forgotten here. Nothing was deleted in %s.", Es: "Destino '%s' olvidado aquí. En %s no se borró nada."},
	"cli.sync.recovery_title": {En: "RECOVERY CODE — shown only this once:", Es: "CÓDIGO DE RECUPERACIÓN — se enseña solo esta vez:"},
	"cli.sync.recovery_hint": {
		En: "Keep it off this machine (a password manager, paper). It opens the destination if you forget the passphrase; without the passphrase or this code, what is in there cannot be read by anyone.",
		Es: "Guárdalo fuera de este equipo (un gestor de contraseñas, papel). Abre el destino si olvidas la frase; sin la frase ni este código, lo que hay ahí no lo puede leer nadie.",
	},
	"cli.sync.vault_created_locked": {
		En: "The vault was created in the destination, but this machine could not save its key: run `ccp sync remote add` again with the same passphrase.",
		Es: "La bóveda se creó en el destino, pero este equipo no pudo guardar su clave: vuelve a ejecutar `ccp sync remote add` con la misma frase.",
	},
	"cli.sync.locked": {
		En: "The destination '%s' is locked on this machine: run `ccp sync remote rm %s` and add it again with its passphrase.",
		Es: "El destino '%s' está bloqueado en este equipo: ejecuta `ccp sync remote rm %s` y vuelve a añadirlo con su frase.",
	},
	"cli.sync.push_nothing": {En: "Nothing to upload: %s already has every local snapshot.", Es: "Nada que subir: %s ya tiene todos los snapshots locales."},
	"cli.sync.pushed":       {En: "Uploaded to %s: %d snapshots, %d contents (%s).", Es: "Subido a %s: %d snapshots, %d contenidos (%s)."},
	"cli.sync.push_missing": {
		En: "%d paths have no content on this machine (imported without secrets): they did not travel.",
		Es: "%d rutas no tienen contenido en este equipo (importadas sin secretos): no viajaron.",
	},
	"cli.sync.push_too_large": {En: "these paths are over the per-file limit and did not travel: %s", Es: "estas rutas pasan del tope por archivo y no viajaron: %s"},
	"cli.sync.pull_none":      {En: "%s has no snapshots yet.", Es: "%s todavía no tiene snapshots."},
	"cli.sync.pulled":         {En: "Downloaded from %s: it is now the local snapshot %s.", Es: "Bajado de %s: aquí es el snapshot local %s."},
	"cli.sync.pull_missing":   {En: "%d paths had no content in the destination.", Es: "%d rutas no tenían contenido en el destino."},
	"cli.sync.pull_hint":      {En: "Apply it here with: ccp sync apply %s --yes", Es: "Aplícalo aquí con: ccp sync apply %s --yes"},
	"cli.sync.apply_confirm": {
		En: "Nothing was changed. Run it again with --yes to apply it (or with --plan to just see it).",
		Es: "No se cambió nada. Ejecútalo de nuevo con --yes para aplicarlo (o con --plan para solo verlo).",
	},
	"cli.sync.locked_mark": {En: "locked here", Es: "bloqueado aquí"},
}
