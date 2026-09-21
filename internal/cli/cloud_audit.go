package cli

// `ccp cloud audit` — el registro de auditoría de la cuenta (spec §10.5).
// Cuenta quién hizo qué y cuándo, y nada de lo que hizo: es lo único que el
// servidor puede ofrecer sobre una configuración que no puede leer.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// auditSinceFormats: un día suelto es lo que se escribe a mano, y la fecha
// entera lo que copia un script de una entrada anterior.
var auditSinceFormats = []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04", "2006-01-02"}

// parseAuditSince interpreta --since en la zona local: quien escribe «desde
// ayer» piensa en su reloj, no en UTC.
func parseAuditSince(v string) (time.Time, bool) {
	for _, f := range auditSinceFormats {
		if t, err := time.ParseInLocation(f, v, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (c cloudCmd) audit(args []string) int {
	a, ok := c.args(args, []string{"--json"}, []string{"--device", "--action", "--since", "--limit"}, 0)
	if !ok {
		return 1
	}
	var q client.AuditQuery
	q.Action = a.val("--action")
	if v := a.val("--since"); v != "" {
		t, ok := parseAuditSince(v)
		if !ok {
			fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.audit_bad_since"))
			return 1
		}
		q.Since = t
	}
	if v := a.val("--limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > api.MaxAuditEntries {
			fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.audit_bad_limit", api.MaxAuditEntries))
			return 1
		}
		q.Limit = n
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	// Un equipo que no se sabe cuál es no se convierte en «toda la cuenta»:
	// una auditoría que enseña de más se lee como si enseñara lo pedido.
	if ref := a.val("--device"); ref != "" {
		d, err := c.findDevice(cl, ref)
		if err != nil {
			return c.fail(err)
		}
		q.Device = d.ID
	}
	log, err := cl.Audit(c.ctx, q)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		if log == nil {
			log = []api.AuditEntry{}
		}
		return snapJSON(c.out, c.err, log)
	}
	if len(log) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.audit_none"))
		return 0
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T(c.lang, "cli.cloud.audit_header"))
	for _, e := range log {
		who := e.DeviceName
		switch {
		case e.Device == "":
			who = "-"
		case who == "":
			// El id sigue ahí aunque el equipo ya no: decirlo es más honrado
			// que dejar la celda vacía, que se lee como «nadie».
			who = i18n.T(c.lang, "cli.cloud.audit_gone", shortID(e.Device))
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			e.At.Local().Format("2006-01-02 15:04"), who, e.Action, auditDetail(e.Detail))
	}
	_ = tw.Flush()
	return 0
}

// auditDetail pinta el detalle en una línea, ordenado por clave: el registro
// se lee para comparar dos entradas, y un orden que baila lo impide.
func auditDetail(d map[string]any) string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := fmt.Sprintf("%v", d[k])
		if len(v) > 12 && isHexID(v) {
			v = v[:8]
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

// isHexID dice si v parece un id (blob, snapshot, revisión): solo entonces se
// recorta, porque recortar un nombre o un motivo lo estropearía.
func isHexID(v string) bool {
	for _, r := range v {
		if !strings.ContainsRune("0123456789abcdef-", r) {
			return false
		}
	}
	return true
}
