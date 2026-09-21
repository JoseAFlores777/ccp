package store

// Grupos de dispositivos en memoria. La implementación de Postgres (pg.go)
// cumple el mismo contrato (groups_contract_test.go).

import (
	"context"
	"slices"
	"time"
)

// normMembers deja los miembros sin repetidos y ordenados. El orden da igual
// para aplicar una orden, pero no para comparar dos lecturas del mismo grupo:
// sin él, cada almacén devolvería la lista en el orden que le saliera.
func normMembers(ids []string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

// checkMembers comprueba que todos son dispositivos de esta cuenta. Llamar con
// el candado cogido.
func (m *Mem) checkMembers(userID string, ids []string) error {
	for _, id := range ids {
		if _, ok := m.devices[userID][id]; !ok {
			return ErrNotFound
		}
	}
	return nil
}

// nameTaken dice si otro grupo (distinto de skip) ya se llama name.
func (m *Mem) nameTaken(userID, name, skip string) bool {
	for _, g := range m.groups[userID] {
		if g.ID != skip && g.Name == name {
			return true
		}
	}
	return false
}

func cloneGroup(g Group) Group {
	g.Members = slices.Clone(g.Members)
	return g
}

func (m *Mem) CreateGroup(_ context.Context, userID string, g Group) (Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g.Members = normMembers(g.Members)
	if err := m.checkMembers(userID, g.Members); err != nil {
		return Group{}, err
	}
	if m.nameTaken(userID, g.Name, "") {
		return Group{}, ErrConflict
	}
	now := time.Now().UTC()
	g.ID, g.Created, g.Updated = newUUID(), now, now
	if m.groups[userID] == nil {
		m.groups[userID] = map[string]Group{}
	}
	m.groups[userID][g.ID] = g
	return cloneGroup(g), nil
}

func (m *Mem) Groups(_ context.Context, userID string) ([]Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Group, 0, len(m.groups[userID]))
	for _, g := range m.groups[userID] {
		out = append(out, cloneGroup(g))
	}
	slices.SortFunc(out, func(a, b Group) int {
		if a.Name != b.Name {
			return compareStr(a.Name, b.Name)
		}
		return compareStr(a.ID, b.ID)
	})
	return out, nil
}

func (m *Mem) Group(_ context.Context, userID, id string) (Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[userID][id]
	if !ok {
		return Group{}, ErrNotFound
	}
	return cloneGroup(g), nil
}

func (m *Mem) UpdateGroup(_ context.Context, userID string, g Group) (Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.groups[userID][g.ID]
	if !ok {
		return Group{}, ErrNotFound
	}
	members := normMembers(g.Members)
	if err := m.checkMembers(userID, members); err != nil {
		return Group{}, err
	}
	if m.nameTaken(userID, g.Name, g.ID) {
		return Group{}, ErrConflict
	}
	cur.Name, cur.Members, cur.Updated = g.Name, members, time.Now().UTC()
	m.groups[userID][g.ID] = cur
	return cloneGroup(cur), nil
}

func (m *Mem) DeleteGroup(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[userID][id]; !ok {
		return ErrNotFound
	}
	delete(m.groups[userID], id)
	return nil
}

func compareStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
