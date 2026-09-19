package cli

// serve_inventory.go — los métodos de `ccp serve` de la Fase A: inventory.scan,
// adopt.plan y adopt.apply. Mismo motor que `ccp scan` y `ccp adopt`.

import (
	"encoding/json"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func srvInventoryScan(s *server, _ json.RawMessage) (any, error) {
	r, err := inventoryRoots(s.home)
	if err != nil {
		return nil, err
	}
	return core.BuildInventory(r), nil
}

// srvAdoptSteps es el plan de esta máquina ahora mismo.
func srvAdoptSteps(s *server) (core.InventoryRoots, []core.AdoptStep, error) {
	r, err := inventoryRoots(s.home)
	if err != nil {
		return r, nil, err
	}
	cfg, err := core.Load(s.home)
	if err != nil {
		return r, nil, err
	}
	apps, _ := core.DesktopAppsDir()
	return r, core.AdoptPlan(core.BuildInventory(r), core.AdoptInputsFor(s.home, r.ClaudeSrc, cfg, apps)), nil
}

func srvAdoptPlan(s *server, _ json.RawMessage) (any, error) {
	_, steps, err := srvAdoptSteps(s)
	if err != nil {
		return nil, err
	}
	return map[string]any{"steps": steps}, nil
}

// srvAdoptApply: `only` ausente = los pasos por defecto; `only: []` = ninguno
// (la GUI sin casillas marcadas no aplica «lo de siempre»).
func srvAdoptApply(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Only *[]string `json:"only"`
	}](raw)
	if err != nil {
		return nil, err
	}
	r, steps, err := srvAdoptSteps(s)
	if err != nil {
		return nil, err
	}
	var only []string
	if p.Only != nil {
		only = append([]string{}, (*p.Only)...)
	}
	return core.AdoptApply(s.home, r, steps, core.AdoptApplyOpts{Only: only, Before: func() error {
		_, err := autoSnapshot(s.home, "pre-adopt")
		return err
	}})
}
