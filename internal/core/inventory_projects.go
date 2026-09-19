package core

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Proyectos conocidos y directorios de config que ccp no gestiona (spec §5.1).
// Van aparte del recorrido global porque su lista no es fija: sale de las
// reglas de carpeta, de las claves `projects` de cada .claude.json y de lo que
// haya en el home y en los rc.

// invExists dice si una ruta existe sin dejar sonda. Se usa donde una ausencia
// es lo normal (un proyecto sin .mcp.json, uno que se borró hace meses) y
// cien sondas «missing» taparían las que importan.
func invExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// invProjectPaths son los proyectos conocidos, sin repetir y solo los que
// siguen en disco: ~/.claude.json guarda para siempre cada carpeta en la que se
// abrió Claude Code, y la mayoría ya no existen.
func (w *invWalker) invProjectPaths() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range w.projects {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// projectID es la identidad portable de un proyecto, la misma que usan los
// snapshots (projectKey/gitOriginURL), cacheada por ruta.
func (w *invWalker) projectID(p string) *InvProject {
	if w.ids == nil {
		w.ids = map[string]*InvProject{}
	}
	if id, ok := w.ids[p]; ok {
		return id
	}
	remote := gitOriginURL(p)
	id := &InvProject{Path: p, Key: projectKey(p, remote), Remote: remote}
	w.ids[p] = id
	return id
}

// tagProject pone la identidad del proyecto p en los items añadidos desde
// `from`: los recorridos comunes (settings, markdown, MCP) no saben de
// proyectos y así no hace falta enhebrarles un parámetro más.
func (w *invWalker) tagProject(from int, p string) {
	if !invExists(p) {
		return
	}
	id := w.projectID(p)
	for i := from; i < len(w.inv.Items); i++ {
		w.inv.Items[i].Project = id
	}
}

// walkProjects recorre la configuración de repo de cada proyecto conocido: la
// versionada (.claude/settings.json, CLAUDE.md, .claude/{agents,commands,skills})
// y la local (settings.local.json, CLAUDE.local.md). El .mcp.json ya lo leyó
// walkMCP. Un archivo que el proyecto no tiene no deja sonda.
//
// Un proyecto cuyo .claude es un config dir con dueño (el caso de siempre: el
// home, que ~/.claude.json guarda en cuanto se abre claude allí) no aporta su
// .claude: ese directorio ES el global (o el cc-home de un perfil) y ya se
// recorrió como tal. Leerlo otra vez duplicaba cada item global en una capa de
// proyecto, con doble sonda. Lo que el proyecto tiene fuera de .claude
// (CLAUDE.md, CLAUDE.local.md) sí es suyo.
func (w *invWalker) walkProjects(owned map[string]bool) {
	for _, p := range w.invProjectPaths() {
		sc := InvScope{Level: "project", Name: p}
		from := len(w.inv.Items)
		cd := filepath.Join(p, ".claude")
		// repoCD: el .claude es del proyecto y no un config dir con dueño.
		repoCD := !owned[invRealPath(cd)]
		for _, f := range []string{
			filepath.Join(p, ".claude", "settings.json"),
			filepath.Join(p, ".claude", "settings.local.json"),
		} {
			if !repoCD || !invExists(f) {
				continue
			}
			if m, ok := w.readJSONObject(f); ok {
				w.invSettings(f, sc, false, m)
			}
		}
		for _, n := range []string{"CLAUDE.md", "CLAUDE.local.md"} {
			f := filepath.Join(p, n)
			if !invExists(f) {
				continue
			}
			if b, ok := w.readFile(f); ok {
				w.add(InvItem{Kind: "rule-instr", Scope: sc, Name: n, Source: f, Editable: true, Hash: invHashText(b)})
			}
		}
		if !repoCD {
			w.tagProject(from, p)
			continue
		}
		// En orden fijo: el inventario sale igual en cada ejecución.
		for _, sk := range [][2]string{{"agents", "agent"}, {"commands", "command"}} {
			if d := filepath.Join(cd, sk[0]); invExists(d) {
				w.walkMarkdown(d, "", sk[1], sc)
			}
		}
		if d := filepath.Join(cd, "skills"); invExists(d) {
			w.walkSkills(d, sc)
		}
		w.tagProject(from, p)
	}
}

// invOwnedConfigDirs son los config dirs que ya tienen dueño: el ~/.claude del
// usuario y el cc-home de cada perfil, por ruta real. Se recorren como global o
// como perfil, y nadie más (ni un proyecto ni un candidato a adoptar) los
// vuelve a contar.
func invOwnedConfigDirs(r InventoryRoots, cfg *Config) map[string]bool {
	owned := map[string]bool{}
	if r.ClaudeSrc != "" {
		owned[invRealPath(r.ClaudeSrc)] = true
	}
	if cfg != nil {
		for _, n := range invSortedProfiles(cfg) {
			owned[invRealPath(ccHomePath(r.CCPHome, n))] = true
		}
	}
	return owned
}

// invRealPath resuelve enlaces para comparar rutas; si no puede (no existe),
// la deja limpia tal cual.
func invRealPath(p string) string {
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		return rp
	}
	return filepath.Clean(p)
}

// walkConfigDirs busca los CLAUDE_CONFIG_DIR que ccp no gestiona: otros
// ~/.claude* con pinta de config (settings.json, .claude.json o projects/) y
// los que declaran los rc con `export CLAUDE_CONFIG_DIR=`. Son candidatos a
// adoptar como perfil (spec §5.2, D5); el ~/.claude del usuario y el cc-home
// de cada perfil ya tienen dueño y no salen.
func (w *invWalker) walkConfigDirs(r InventoryRoots, owned map[string]bool) {
	idx := map[string]int{}
	add := func(dir string) *InvItem {
		k := invRealPath(dir)
		if owned[k] {
			return nil
		}
		if i, ok := idx[k]; ok {
			return &w.inv.Items[i]
		}
		why, _ := invConfigDirSummary(dir)
		w.add(InvItem{Kind: "config-dir", Scope: InvScope{Level: "global"}, Name: filepath.Base(dir),
			Source: dir, Why: why, AppliesTo: []string{}, Hash: invHashJSON(dir)})
		idx[k] = len(w.inv.Items) - 1
		return &w.inv.Items[idx[k]]
	}

	if r.Home != "" {
		if es, ok := w.readDir(r.Home); ok {
			for _, e := range es {
				n := e.Name()
				if !strings.HasPrefix(n, ".claude") || n == ".claude" || n == ".claude.json" {
					continue
				}
				d := filepath.Join(r.Home, n)
				if !w.isDir(e, d) {
					continue
				}
				if _, looks := invConfigDirSummary(d); looks {
					add(d)
				}
			}
		}
	}

	for _, rc := range r.RCFiles {
		// Un rc que no existe es lo normal (.bashrc en una máquina con zsh).
		if !invExists(rc) {
			continue
		}
		b, ok := w.readFile(rc)
		if !ok {
			continue
		}
		for _, d := range invRCConfigDirs(string(b), r.Home) {
			it := add(d)
			if it == nil {
				continue
			}
			// Declarado en un rc, la CLI lo usa en cada terminal nueva.
			if !slices.Contains(it.AppliesTo, InvAppliesCLI) {
				it.AppliesTo = append(it.AppliesTo, InvAppliesCLI)
			}
			decl := "declarado en " + rc
			if !invExists(d) {
				decl += "; la carpeta no existe"
			}
			if it.Why == "" {
				it.Why = decl
			} else if !strings.Contains(it.Why, decl) {
				it.Why += " · " + decl
			}
		}
	}
}

// invRCConfigDirs saca los CLAUDE_CONFIG_DIR que exporta un rc, con `~`,
// `$HOME` y `${HOME}` expandidos contra home. Un valor que usa otra variable
// no se puede resolver sin ejecutar el rc, así que se omite en vez de inventar
// una ruta.
func invRCConfigDirs(rc, home string) []string {
	var out []string
	for _, line := range strings.Split(rc, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "#") {
			continue
		}
		l = strings.TrimSpace(strings.TrimPrefix(l, "export "))
		v, ok := strings.CutPrefix(l, "CLAUDE_CONFIG_DIR=")
		if !ok {
			continue
		}
		// Un comentario al final de la línea no es parte del valor.
		if i := strings.Index(v, " #"); i >= 0 {
			v = v[:i]
		}
		v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), ";"))
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		switch {
		case v == "~" || strings.HasPrefix(v, "~/"):
			v = home + v[1:]
		case strings.HasPrefix(v, "${HOME}"):
			v = home + strings.TrimPrefix(v, "${HOME}")
		case strings.HasPrefix(v, "$HOME"):
			v = home + strings.TrimPrefix(v, "$HOME")
		}
		if v == "" || strings.Contains(v, "$") || !filepath.IsAbs(v) {
			continue
		}
		out = append(out, filepath.Clean(v))
	}
	return out
}

// invConfigDirSummary dice qué hay en un posible CLAUDE_CONFIG_DIR y si lo
// parece: lo parece si tiene settings.json, .claude.json o projects/. El
// resumen cuenta lo que traería al adoptarlo (MCP, skills y agentes) sin dejar
// sondas: es un candidato, no una fuente del inventario.
func invConfigDirSummary(dir string) (string, bool) {
	var has []string
	for _, m := range []string{"settings.json", ".claude.json", "projects"} {
		if invExists(filepath.Join(dir, m)) {
			if m == "projects" {
				m += "/"
			}
			has = append(has, m)
		}
	}
	if len(has) == 0 {
		return "", false
	}
	mcp := 0
	if b, err := os.ReadFile(filepath.Join(dir, ".claude.json")); err == nil {
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			mcp = len(invMCPMap(m))
		}
	}
	skills := 0
	if es, err := os.ReadDir(filepath.Join(dir, "skills")); err == nil {
		for _, e := range es {
			if invExists(filepath.Join(dir, "skills", e.Name(), "SKILL.md")) {
				skills++
			}
		}
	}
	agents := 0
	ad := filepath.Join(dir, "agents")
	_ = filepath.WalkDir(ad, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.Count(strings.TrimPrefix(p, ad), string(filepath.Separator)) > invMaxDepth {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			agents++
		}
		return nil
	})
	return fmt.Sprintf("%s · trae %d MCP, %d %s, %d %s", strings.Join(has, ", "),
		mcp, skills, invPlural(skills, "skill", "skills"), agents, invPlural(agents, "agente", "agentes")), true
}

func invPlural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
