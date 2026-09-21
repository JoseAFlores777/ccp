package i18n

// catalog_cloud.go — prosa de `ccp cloud` (internal/cli/cloud.go). Prefijo
// `cli.cloud.` y nada más.

func init() { register(catalogCloud) }

var catalogCloud = map[string]map[Lang]string{
	"cli.cloud.usage": {
		En: `Usage: ccp cloud <subcommand>

  login <server> [--name <machine>]   sign in (device code) and register this machine
  logout                              revoke this machine and delete its token and local vault
  status [--json]                     server, account, machine, vault, pending uploads
  init                                create the vault (first machine) and show the recovery code
  unlock [--recovery]                 unlock the vault on this machine
  push [<snapshot>] [--json]          upload the local snapshots the cloud does not have
  pull [<id>|latest] [--device <n>]   download a snapshot into the local store
       [-o <file>] [--decrypted]      …or into a file: .ccpsnap, or a readable .tar.gz with --yes
  restore [<id>|latest] [--yes]       bring this machine to that snapshot (any machine's)
       [--only <lpath>] [--map k=dir] …or only those paths; --map says where a project lives here
  list [--json]                       snapshots in the cloud, from every machine
  verify [--json]                     check the whole signed history for tampering
  devices [--json]                    machines of the account
  revoke <device>                     revoke another machine
  agent [--once] [--interval <d>]     apply the revisions the portal publishes for this machine
  review [--yes|--reject] [--json]    confirm what runs code here before it is applied
  policy [auto|manual]                this machine's policy for incoming revisions

Everything is encrypted on this machine before it leaves: the server cannot read it.
CCP_CLOUD_PASSPHRASE and CCP_CLOUD_RECOVERY give the secrets without asking.`,
		Es: `Uso: ccp cloud <subcomando>

  login <servidor> [--name <equipo>]  inicia sesión (código de dispositivo) y registra este equipo
  logout                              revoca este equipo y borra su token y su bóveda local
  status [--json]                     servidor, cuenta, equipo, bóveda, pendientes de subir
  init                                crea la bóveda (primera máquina) y enseña el código de recuperación
  unlock [--recovery]                 desbloquea la bóveda en este equipo
  push [<snapshot>] [--json]          sube los snapshots locales que la nube no tiene
  pull [<id>|latest] [--device <n>]   baja un snapshot al almacén local
       [-o <archivo>] [--decrypted]   …o a un archivo: .ccpsnap, o un .tar.gz legible con --yes
  restore [<id>|latest] [--yes]       deja esta máquina en ese snapshot (de cualquier equipo)
       [--only <ruta>] [--map c=dir]  …o solo esas rutas; --map dice dónde vive aquí un proyecto
  list [--json]                       snapshots en la nube, de todos los equipos
  verify [--json]                     comprueba que nadie ha tocado la historia firmada
  devices [--json]                    equipos de la cuenta
  revoke <dispositivo>                revoca otro equipo
  agent [--once] [--interval <d>]     aplica las revisiones que el portal publica para este equipo
  review [--yes|--reject] [--json]    confirma lo que ejecuta código aquí antes de aplicarlo
  policy [auto|manual]                política de este equipo ante las revisiones que llegan

Todo se cifra en este equipo antes de salir: el servidor no puede leerlo.
CCP_CLOUD_PASSPHRASE y CCP_CLOUD_RECOVERY dan los secretos sin preguntarlos.`,
	},
	"cli.cloud.verify_ok": {
		En: "The history is intact: %d links, every signature from this account.",
		Es: "La historia está intacta: %d eslabones, todas las firmas de esta cuenta.",
	},
	"cli.cloud.verify_pruned": {
		En: "%d link(s) keep only their signature: the server's retention took their content, and they still hold the chain together.",
		Es: "%d eslabón(es) conservan solo su firma: la retención del servidor se llevó su contenido, y siguen sosteniendo la cadena.",
	},
	"cli.cloud.snapshot_pruned": {
		En: "The server's retention took that snapshot: its link is still in the chain, its content is not. If this machine made it, it is still here: ccp snapshot list",
		Es: "La retención del servidor se llevó ese snapshot: su eslabón sigue en la cadena, su contenido no. Si lo hizo esta máquina, sigue aquí: ccp snapshot list",
	},
	"cli.cloud.verify_bad": {
		En: "The history does NOT add up: %d problem(s) in %d links.",
		Es: "La historia NO cuadra: %d problema(s) en %d eslabones.",
	},
	"cli.cloud.verify_hint": {
		En: "Nothing was deleted here: your snapshots are on this machine. Do not trust that server until you know why.",
		Es: "Aquí no se ha borrado nada: tus snapshots están en esta máquina. No te fíes de ese servidor hasta saber por qué.",
	},
	"cli.cloud.fault.bad_signature": {
		En: "this account did not sign it, or its id, parent or manifest was changed",
		Es: "no la firmó esta cuenta, o le cambiaron el id, el padre o el manifiesto",
	},
	"cli.cloud.fault.broken_link": {
		En: "its parent %s is not in the chain",
		Es: "su padre %s no está en la cadena",
	},
	"cli.cloud.fault.dropped": {
		En: "this machine uploaded it and the server no longer has it",
		Es: "esta máquina lo subió y el servidor ya no lo tiene",
	},
	"cli.cloud.fault.cycle": {
		En: "following its parents never reaches a beginning",
		Es: "siguiendo a sus padres no se llega a ningún principio",
	},
	"cli.cloud.fault.out_of_order": {
		En: "it says it is older than its parent %s",
		Es: "dice ser anterior a su padre %s",
	},
	"cli.cloud.fault.duplicate_id": {
		En: "the same id arrived twice",
		Es: "el mismo id llegó dos veces",
	},
	"cli.cloud.unknown_sub": {En: "cloud: unknown subcommand '%s'", Es: "cloud: subcomando desconocido '%s'"},
	"cli.cloud.unknown_opt": {En: "cloud: unknown option or extra argument '%s'", Es: "cloud: opción desconocida o argumento de más '%s'"},
	"cli.cloud.need_server": {En: "cloud: which server? ccp cloud login https://…", Es: "cloud: ¿qué servidor? ccp cloud login https://…"},
	"cli.cloud.need_https": {
		En: "cloud: the server must use https (plain http is only accepted for localhost)",
		Es: "cloud: el servidor tiene que usar https (http solo se acepta para localhost)",
	},
	"cli.cloud.login_open": {
		En: "Open %s in your browser and confirm the code %s. Waiting…",
		Es: "Abre %s en el navegador y confirma el código %s. Esperando…",
	},
	"cli.cloud.login_ok":       {En: "Signed in as %s; this machine is «%s».", Es: "Sesión iniciada como %s; este equipo es «%s»."},
	"cli.cloud.login_switched": {En: "It is another account: the previous vault was forgotten on this machine.", Es: "Es otra cuenta: se ha olvidado la bóveda anterior en este equipo."},
	"cli.cloud.next_init":      {En: "Next, on your first machine: ccp cloud init", Es: "Siguiente paso, en tu primera máquina: ccp cloud init"},
	"cli.cloud.next_unlock":    {En: "This account already has a vault. Unlock it here: ccp cloud unlock", Es: "Esta cuenta ya tiene bóveda. Desbloquéala aquí: ccp cloud unlock"},
	"cli.cloud.logged_out": {
		En: "Signed out: this machine was revoked and its token and local vault deleted.",
		Es: "Sesión cerrada: este equipo quedó revocado y se borraron su token y su bóveda local.",
	},
	"cli.cloud.not_logged_in": {
		En: "This machine is not signed in to the cloud: ccp cloud login <server>",
		Es: "Este equipo no ha iniciado sesión en la nube: ccp cloud login <servidor>",
	},
	"cli.cloud.locked": {En: "The vault is locked on this machine: ccp cloud unlock", Es: "La bóveda está bloqueada en este equipo: ccp cloud unlock"},

	"cli.cloud.status_server":  {En: "Server:   %s", Es: "Servidor:   %s"},
	"cli.cloud.status_account": {En: "Account:  %s", Es: "Cuenta:     %s"},
	"cli.cloud.status_device":  {En: "Machine:  %s", Es: "Equipo:     %s"},
	"cli.cloud.status_vault":   {En: "Vault:    %s", Es: "Bóveda:     %s"},
	"cli.cloud.status_pending": {En: "Pending:  %d snapshots to upload", Es: "Pendientes: %d snapshots por subir"},
	"cli.cloud.vault_unlocked": {En: "unlocked on this machine", Es: "desbloqueada en este equipo"},
	"cli.cloud.vault_locked":   {En: "locked (ccp cloud unlock)", Es: "bloqueada (ccp cloud unlock)"},
	"cli.cloud.vault_missing":  {En: "not created (ccp cloud init)", Es: "sin crear (ccp cloud init)"},
	"cli.cloud.vault_unknown":  {En: "unknown (no connection)", Es: "desconocido (sin conexión)"},

	"cli.cloud.vault_exists": {
		En: "This account already has a vault. Unlock it with: ccp cloud unlock",
		Es: "Esta cuenta ya tiene bóveda. Desbloquéala con: ccp cloud unlock",
	},
	"cli.cloud.no_vault": {
		En: "This account has no vault yet. Create it on your first machine with: ccp cloud init",
		Es: "Esta cuenta aún no tiene bóveda. Créala en tu primera máquina con: ccp cloud init",
	},
	"cli.cloud.vault_created": {En: "Vault created and unlocked on this machine.", Es: "Bóveda creada y desbloqueada en este equipo."},
	"cli.cloud.vault_created_locked": {
		En: "The vault was created, but this machine could not save its key: unlock it with ccp cloud unlock",
		Es: "La bóveda se creó, pero este equipo no pudo guardar su clave: desbloquéalo con ccp cloud unlock",
	},
	"cli.cloud.recovery_title": {En: "RECOVERY CODE — shown only this once:", Es: "CÓDIGO DE RECUPERACIÓN — se enseña solo esta vez:"},
	"cli.cloud.recovery_hint": {
		En: "Keep it off this machine (a password manager, paper). It opens the vault if you forget the passphrase; without the passphrase or this code, your cloud data cannot be recovered.",
		Es: "Guárdalo fuera de este equipo (un gestor de contraseñas, papel). Abre la bóveda si olvidas la frase; sin la frase ni este código, tus datos en la nube no se pueden recuperar.",
	},
	"cli.cloud.pass_prompt":     {En: "Vault passphrase: ", Es: "Frase de la bóveda: "},
	"cli.cloud.pass_confirm":    {En: "Repeat it: ", Es: "Repítela: "},
	"cli.cloud.pass_mismatch":   {En: "the passphrases do not match", Es: "las frases no coinciden"},
	"cli.cloud.pass_short":      {En: "the passphrase needs at least %d characters", Es: "la frase necesita al menos %d caracteres"},
	"cli.cloud.pass_needed":     {En: "a secret is needed: set %s or run it in a terminal", Es: "hace falta un secreto: define %s o ejecútalo en una terminal"},
	"cli.cloud.recovery_prompt": {En: "Recovery code: ", Es: "Código de recuperación: "},
	"cli.cloud.unlocked":        {En: "Vault unlocked on this machine.", Es: "Bóveda desbloqueada en este equipo."},
	"cli.cloud.unlock_wrong":    {En: "That passphrase (or code) does not open the vault.", Es: "Esa frase (o código) no abre la bóveda."},

	"cli.cloud.pushed":         {En: "Uploaded %d snapshots (%d blobs, %s).", Es: "Subidos %d snapshots (%d blobs, %s)."},
	"cli.cloud.push_nothing":   {En: "Nothing to upload: the cloud has every snapshot of this machine.", Es: "Nada que subir: la nube tiene todos los snapshots de este equipo."},
	"cli.cloud.push_missing":   {En: "%d items were not uploaded because their data is not on this machine.", Es: "%d elementos no se subieron porque sus datos no están en este equipo."},
	"cli.cloud.push_too_large": {En: "Not uploaded (over 64 MiB): %s", Es: "No se subieron (más de 64 MiB): %s"},
	"cli.cloud.pulled":         {En: "Snapshot %s downloaded (here it is %s).", Es: "Snapshot %s bajado (en este equipo es %s)."},
	"cli.cloud.pull_hint":      {En: "To apply it: ccp snapshot restore %s", Es: "Para aplicarlo: ccp snapshot restore %s"},
	"cli.cloud.pull_missing":   {En: "%d items had no data in the cloud; they cannot be restored.", Es: "%d elementos no tenían datos en la nube; no se podrán restaurar."},
	"cli.cloud.pull_none":      {En: "The cloud has no snapshots (from that machine).", Es: "La nube no tiene snapshots (de ese equipo)."},
	"cli.cloud.pull_ambiguous": {En: "No single cloud snapshot starts with %s.", Es: "No hay un único snapshot en la nube que empiece por %s."},
	"cli.cloud.list_header":    {En: "ID\tDATE\tMACHINE\tSIZE\tHERE", Es: "ID\tFECHA\tEQUIPO\tTAMAÑO\tAQUÍ"},
	"cli.cloud.list_here":      {En: "yes", Es: "sí"},
	"cli.cloud.devices_header": {En: "ID\tNAME\tPLATFORM\tLAST SEEN\tSTATUS", Es: "ID\tNOMBRE\tPLATAFORMA\tÚLTIMO CONTACTO\tESTADO"},
	"cli.cloud.device_this":    {En: "this machine", Es: "este equipo"},
	"cli.cloud.device_revoked": {En: "revoked", Es: "revocado"},
	"cli.cloud.device_active":  {En: "active", Es: "activo"},
	"cli.cloud.device_unknown": {En: "No single device starts with %s.", Es: "No hay un único dispositivo que empiece por %s."},
	"cli.cloud.device_needed":  {En: "cloud revoke: which device? ccp cloud devices lists them.", Es: "cloud revoke: ¿qué dispositivo? ccp cloud devices los enseña."},
	"cli.cloud.revoked":        {En: "Device «%s» revoked: its session can no longer use the cloud, nor register another device.", Es: "Dispositivo «%s» revocado: su sesión ya no puede usar la nube ni dar de alta otro equipo."},
	"cli.cloud.revoke_self":    {En: "That is this machine; to disconnect it use: ccp cloud logout", Es: "Ese es este equipo; para desconectarlo usa: ccp cloud logout"},
}

// F3-2: bajar un snapshot a un archivo (spec §10.3.1). Lo cifrado se importa
// en otra máquina; lo descifrado se lee con cualquier tar y por eso avisa.
func init() { register(catalogCloudDownload) }

var catalogCloudDownload = map[string]map[Lang]string{
	"cli.cloud.pull_needs_output": {
		En: "--decrypted and --yes only make sense with -o <file>: what goes into the local store is always sealed.",
		Es: "--decrypted y --yes solo tienen sentido con -o <archivo>: lo que baja al almacén local va siempre sellado.",
	},
	"cli.cloud.pull_plain_warn": {
		En: "This snapshot contains your keys and they would be written in the clear. Repeat with --yes if that is what you want.",
		Es: "Este snapshot contiene tus claves y se escribirían en claro. Repite con --yes si es lo que quieres.",
	},
	"cli.cloud.pulled_file":       {En: "Snapshot %s downloaded to %s (encrypted).", Es: "Snapshot %s bajado a %s (cifrado)."},
	"cli.cloud.pulled_file_plain": {En: "Snapshot %s downloaded to %s IN THE CLEAR.", Es: "Snapshot %s bajado a %s EN CLARO."},
	"cli.cloud.pull_file_hint":    {En: "To open it on another machine: ccp snapshot import %s", Es: "Para abrirlo en otra máquina: ccp snapshot import %s"},
}

func init() { register(catalogCloudRestore) }

// catalogCloudRestore — `ccp cloud restore` (spec §10.3.1, caminos 1 y 3).
var catalogCloudRestore = map[string]map[Lang]string{
	"cli.cloud.restore_bad_map": {
		En: "--map takes <project-key>=<absolute path>; %q is not one. `ccp cloud restore --dry-run` lists the keys.",
		Es: "--map lleva <clave-de-proyecto>=<ruta absoluta>; %q no lo es. `ccp cloud restore --dry-run` enseña las claves.",
	},
	"cli.cloud.restore_confirm": {
		En: "Nothing was written. Repeat with --yes to apply it (a snapshot of the current state is taken first).",
		Es: "No se ha escrito nada. Repite con --yes para aplicarlo (antes se toma un snapshot del estado actual).",
	},
	"cli.cloud.restore_project_at": {
		En: "  project %s → %s",
		Es: "  proyecto %s → %s",
	},
	"cli.cloud.restore_project_missing": {
		En: "  project %s is not on this machine: its %d files are skipped (map it with --map)",
		Es: "  el proyecto %s no está en esta máquina: se saltan sus %d archivos (mapéalo con --map)",
	},
	"cli.cloud.restore_pending": {
		En: "Left to do by hand:",
		Es: "Queda por hacer a mano:",
	},
	"cli.cloud.pending_login": {
		En: "log in to profile %s: ccp profile login %[1]s",
		Es: "inicia sesión en el perfil %s: ccp profile login %[1]s",
	},
	"cli.cloud.pending_command": {
		En: "command %s is not on this machine (named by %s)",
		Es: "el comando %s no está en esta máquina (lo nombra %s)",
	},
	"cli.cloud.pending_project": {
		En: "project %s is not here; clone %s and repeat with --map",
		Es: "el proyecto %s no está aquí; clona %s y repite con --map",
	},
}
