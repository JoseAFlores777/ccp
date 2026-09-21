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
  list [--json]                       snapshots in the cloud, from every machine
  devices [--json]                    machines of the account
  revoke <device>                     revoke another machine

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
  list [--json]                       snapshots en la nube, de todos los equipos
  devices [--json]                    equipos de la cuenta
  revoke <dispositivo>                revoca otro equipo

Todo se cifra en este equipo antes de salir: el servidor no puede leerlo.
CCP_CLOUD_PASSPHRASE y CCP_CLOUD_RECOVERY dan los secretos sin preguntarlos.`,
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
	"cli.cloud.vault_created":  {En: "Vault created and unlocked on this machine.", Es: "Bóveda creada y desbloqueada en este equipo."},
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
	"cli.cloud.revoked":        {En: "Device «%s» revoked: it can no longer use the cloud.", Es: "Dispositivo «%s» revocado: ya no puede usar la nube."},
	"cli.cloud.revoke_self":    {En: "That is this machine; to disconnect it use: ccp cloud logout", Es: "Ese es este equipo; para desconectarlo usa: ccp cloud logout"},
}
