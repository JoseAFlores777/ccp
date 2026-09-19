package i18n

// catalog_inventory.go — prosa de `ccp scan` y `ccp adopt` (internal/cli/scan.go).

func init() { register(catalogInventory) }

var catalogInventory = map[string]map[Lang]string{
	"cli.scan.usage":       {En: "Usage: ccp scan [--json]", Es: "Uso: ccp scan [--json]"},
	"cli.scan.unknown_opt": {En: "scan: unknown option or extra argument '%s'", Es: "scan: opción desconocida o argumento de más '%s'"},
	"cli.scan.missing_cmd": {En: "the command %s is not on this machine", Es: "el comando %s no está en esta máquina"},
	"cli.scan.unknown": {
		En: "could not read %s (%s): it is shown as unknown, not as empty",
		Es: "no se pudo leer %s (%s): cuenta como desconocido, no como vacío",
	},
	"cli.scan.summary": {En: "%d items · %d unreadable sources", Es: "%d elementos · %d fuentes ilegibles"},

	"cli.adopt.usage": {
		En: "Usage: ccp adopt [--dry-run | --yes] [--only <id|kind>]... [--json]\n  Without --yes it only shows the plan.",
		Es: "Uso: ccp adopt [--dry-run | --yes] [--only <id|tipo>]... [--json]\n  Sin --yes solo enseña el plan.",
	},
	"cli.adopt.unknown_opt":  {En: "adopt: unknown option or extra argument '%s'", Es: "adopt: opción desconocida o argumento de más '%s'"},
	"cli.adopt.no_such_step": {En: "adopt: the plan has no step with id or kind '%s'", Es: "adopt: el plan no tiene ningún paso con id o tipo '%s'"},
	"cli.adopt.empty":        {En: "Nothing to adopt: ccp already sees everything on this machine.", Es: "Nada que adoptar: ccp ya ve todo lo de esta máquina."},
	"cli.adopt.plan_header":  {En: "Adoption plan:", Es: "Plan de adopción:"},
	"cli.adopt.legend": {
		En: "[x] applied with --yes · [ ] only with --only <id> · pending steps are done by hand",
		Es: "[x] se aplica con --yes · [ ] solo con --only <id> · los pendientes se hacen a mano",
	},
	"cli.adopt.confirm": {
		En: "Nothing was changed. Run it again with --yes to apply it (or --dry-run to just see it).",
		Es: "No se cambió nada. Ejecútalo de nuevo con --yes para aplicarlo (o con --dry-run para solo verlo).",
	},
	"cli.adopt.nothing":        {En: "Nothing to apply.", Es: "Nada que aplicar."},
	"cli.adopt.no_snapshot":    {En: "could not save the safety snapshot", Es: "no se pudo guardar el snapshot de seguridad"},
	"cli.adopt.pending_header": {En: "Still to do by hand:", Es: "Queda por hacer a mano:"},
}
