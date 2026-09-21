package cli

// cloud_groups.go — `ccp cloud groups …` (spec §10.3, F4-1). Un grupo es un
// nombre y unos miembros: «todas mis Macs». NO autoriza nada — aplicar a un
// grupo es publicar una revisión FIRMADA por miembro, y lo firmado ata cada
// orden a SU máquina. Aquí solo se gestionan los grupos y se mira cómo les
// fue a sus equipos.

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

func (c cloudCmd) groups(args []string) int {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "":
		return c.groupsList(args)
	case "add":
		return c.groupsAdd(args)
	case "set":
		return c.groupsSet(args)
	case "rm":
		return c.groupsRm(args)
	case "status":
		return c.groupsStatus(args)
	default:
		return c.usage("cli.cloud.groups_unknown_sub", sub)
	}
}

// groupMembers traduce nombres o prefijos de id a dispositivos, como hace
// `ccp cloud revoke`. Se resuelven TODOS antes de tocar el API: media orden
// puesta es peor que ninguna.
func (c cloudCmd) groupMembers(cl *client.API, refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		d, err := c.findDevice(cl, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, d.ID)
	}
	return out, nil
}

// nameOf pinta el equipo por su nombre, y por su id corto si no se conoce:
// un grupo puede nombrar a un equipo que ya no está en el listado.
func nameOf(devs []api.Device, id string) string {
	for _, d := range devs {
		if d.ID == id {
			return d.Name
		}
	}
	return shortID(id)
}

func (c cloudCmd) groupsList(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 0)
	if !ok {
		return 1
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	gs, err := cl.Groups(c.ctx)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, gs)
	}
	if len(gs) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.groups_none"))
		return 0
	}
	devs, err := cl.Devices(c.ctx)
	if err != nil {
		return c.fail(err)
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T(c.lang, "cli.cloud.groups_header"))
	for _, g := range gs {
		nombres := make([]string, 0, len(g.Members))
		for _, id := range g.Members {
			nombres = append(nombres, nameOf(devs, id))
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", shortID(g.ID), g.Name, len(g.Members), strings.Join(nombres, ", "))
	}
	_ = tw.Flush()
	return 0
}

func (c cloudCmd) groupsAdd(args []string) int {
	a, ok := c.args(args, nil, nil, 1+api.MaxGroupMembers)
	if !ok {
		return 1
	}
	if len(a.pos) < 1 {
		return c.usage("cli.cloud.group_name_needed")
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	members, err := c.groupMembers(cl, a.pos[1:])
	if err != nil {
		return c.fail(err)
	}
	g, err := cl.CreateGroup(c.ctx, api.GroupIn{Name: a.pos[0], Members: members})
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.group_added", g.Name, len(g.Members))))
	return 0
}

// groupsSet reescribe los miembros ENTEROS, no por diferencias: mandar la
// lista que se ve es lo único que no depende de qué versión del grupo tenía
// delante quien la manda. Pero omitir la lista NO es mandar una lista vacía:
// `set <grupo> --name <nuevo>` es el comando natural para renombrar y el
// único que existe, así que vaciar el grupo ahí era destruir la membresía sin
// que nadie lo pidiera. Sin posicionales: con --name se conserva lo que había,
// y vaciarlo de verdad se pide con --empty.
func (c cloudCmd) groupsSet(args []string) int {
	a, ok := c.args(args, []string{"--empty"}, []string{"--name"}, 1+api.MaxGroupMembers)
	if !ok {
		return 1
	}
	if len(a.pos) < 1 {
		return c.usage("cli.cloud.group_needed")
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	g, err := c.findGroup(cl, a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	members := g.Members
	switch {
	case len(a.pos) > 1 || a.flags["--empty"]:
		members, err = c.groupMembers(cl, a.pos[1:])
		if err != nil {
			return c.fail(err)
		}
	case a.val("--name") == "":
		return c.usage("cli.cloud.group_set_needs_members")
	}
	name := g.Name
	if n := a.val("--name"); n != "" {
		name = n
	}
	got, err := cl.UpdateGroup(c.ctx, g.ID, api.GroupIn{Name: name, Members: members})
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.group_set", got.Name, len(got.Members))))
	return 0
}

// findGroup busca por id o por nombre y traduce «no existe» a una frase que
// nombra lo que el usuario escribió, en vez de un 404 del API.
func (c cloudCmd) findGroup(cl *client.API, ref string) (api.Group, error) {
	g, err := client.ResolveGroup(c.ctx, cl, ref)
	if errors.Is(err, client.ErrGroupNotFound) {
		return api.Group{}, errors.New(i18n.T(c.lang, "cli.cloud.group_unknown", ref))
	}
	return g, err
}

// groupsRm borra el grupo, nunca las órdenes publicadas con su etiqueta: esas
// pasaron de verdad y siguen su curso en cada máquina. Por eso pide --yes: el
// nombre desaparece y con él la única forma de ver juntas esas órdenes.
func (c cloudCmd) groupsRm(args []string) int {
	a, ok := c.args(args, []string{"--yes"}, nil, 1)
	if !ok {
		return 1
	}
	if len(a.pos) != 1 {
		return c.usage("cli.cloud.group_needed")
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	g, err := c.findGroup(cl, a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	if !a.flags["--yes"] {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.group_rm_confirm", g.Name))
		return 1
	}
	if err := cl.DeleteGroup(c.ctx, g.ID); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.group_removed", g.Name)))
	return 0
}

// groupsStatus enseña cómo le fue a cada equipo la última orden publicada al
// grupo. Un miembro sin ninguna no sale como «pendiente»: no hay ninguna orden
// suya que esté pendiente de nada.
func (c cloudCmd) groupsStatus(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 1)
	if !ok {
		return 1
	}
	if len(a.pos) != 1 {
		return c.usage("cli.cloud.group_needed")
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	g, err := c.findGroup(cl, a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	st, err := cl.GroupStatus(c.ctx, g.ID)
	if err != nil {
		return c.fail(err)
	}
	if st.Members == nil {
		st.Members = []api.GroupMemberStatus{}
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, st)
	}
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.group_status_title", st.Group.Name, len(st.Group.Members)))
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T(c.lang, "cli.cloud.group_status_header"))
	for _, m := range st.Members {
		estado := i18n.T(c.lang, "cli.cloud.group_state_none")
		cuando := ""
		if m.Revision != "" {
			estado = c.revState(m.State)
			cuando = m.Updated.Local().Format("2006-01-02 15:04")
		}
		nota := m.Reason
		if !m.Member {
			// Ya no está en el grupo, pero su orden sigue puesta: esconderla
			// dejaría una orden viva sin nadie que la contara.
			nota = strings.TrimSpace(i18n.T(c.lang, "cli.cloud.group_state_ex") + " " + nota)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", m.DeviceName, estado, cuando, nota)
	}
	_ = tw.Flush()
	return 0
}
