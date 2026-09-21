package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"sort"
	"sync"
	"time"
)

// Mem es un Store en memoria: los tests del servidor y del cliente corren sin
// Postgres. Cumple el mismo contrato (contract_test.go).
type Mem struct {
	mu      sync.Mutex
	users   map[string]User              // por sub
	vaults  map[string]Vault             // por usuario
	devices map[string]map[string]Device // usuario -> id
	blobs   map[string]map[string]int64  // usuario -> blob -> tamaño
	snaps   map[string]map[string]Snapshot
	refs    map[string]map[string][]string // usuario -> snapshot -> blobs
	audit   []string
}

// NewMem devuelve un Store vacío.
func NewMem() *Mem {
	return &Mem{
		users: map[string]User{}, vaults: map[string]Vault{}, devices: map[string]map[string]Device{},
		blobs: map[string]map[string]int64{}, snaps: map[string]map[string]Snapshot{}, refs: map[string]map[string][]string{},
	}
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (m *Mem) UpsertUser(_ context.Context, sub, email string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[sub]
	if !ok {
		u = User{ID: newUUID(), Sub: sub}
	}
	u.Email = email
	m.users[sub] = u
	return u, nil
}

func (m *Mem) Vault(_ context.Context, userID string) (Vault, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vaults[userID]
	if !ok {
		return Vault{}, ErrNotFound
	}
	return v, nil
}

func (m *Mem) CreateVault(_ context.Context, userID string, v Vault) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.vaults[userID]; ok {
		return ErrConflict
	}
	v.Created = time.Now().UTC()
	m.vaults[userID] = v
	return nil
}

func (m *Mem) CreateDevice(_ context.Context, userID string, d Device) (Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	d.ID, d.Created, d.LastSeen, d.Revoked = newUUID(), now, now, false
	if m.devices[userID] == nil {
		m.devices[userID] = map[string]Device{}
	}
	m.devices[userID][d.ID] = d
	return d, nil
}

func (m *Mem) Devices(_ context.Context, userID string) ([]Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Device, 0, len(m.devices[userID]))
	for _, d := range m.devices[userID] {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

func (m *Mem) SeenDevice(_ context.Context, userID, deviceID string, at time.Time) (Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[userID][deviceID]
	if !ok {
		return Device{}, ErrNotFound
	}
	d.LastSeen = at
	m.devices[userID][deviceID] = d
	return d, nil
}

func (m *Mem) RevokeDevice(_ context.Context, userID, deviceID string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[userID][deviceID]
	if !ok {
		return ErrNotFound
	}
	d.Revoked = true
	m.devices[userID][deviceID] = d
	return nil
}

func (m *Mem) KnownBlobs(_ context.Context, userID string, ids []string) (map[string]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]int64{}
	for _, id := range ids {
		if size, ok := m.blobs[userID][id]; ok {
			out[id] = size
		}
	}
	return out, nil
}

func (m *Mem) CommitSnapshot(_ context.Context, userID string, s Snapshot, newBlobs []Blob, refs []string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.snaps[userID][s.ID]; ok {
		return false, nil
	}
	d, ok := m.devices[userID][s.DeviceID]
	if !ok {
		return false, ErrNotFound
	}
	// Todo o nada: se comprueba antes de escribir nada. En Postgres esto es una
	// transacción; aquí basta con no tocar los mapas hasta haber validado.
	incoming := map[string]int64{}
	for _, b := range newBlobs {
		incoming[b.ID] = b.Size
	}
	for _, r := range refs {
		if _, known := m.blobs[userID][r]; !known {
			if _, now := incoming[r]; !now {
				return false, fmt.Errorf("store: el snapshot referencia el blob %s, que no está registrado", r)
			}
		}
	}
	// Cada mapa se crea por su cuenta: inicializarlos juntos deja a los otros
	// dos en nil el día que alguien escriba solo en uno.
	if m.blobs[userID] == nil {
		m.blobs[userID] = map[string]int64{}
	}
	if m.snaps[userID] == nil {
		m.snaps[userID] = map[string]Snapshot{}
	}
	if m.refs[userID] == nil {
		m.refs[userID] = map[string][]string{}
	}
	maps.Copy(m.blobs[userID], incoming)
	s.DeviceName = d.Name
	s.Manifest, s.Sig = bytes.Clone(s.Manifest), bytes.Clone(s.Sig)
	m.snaps[userID][s.ID] = s
	m.refs[userID][s.ID] = append([]string(nil), refs...)
	return true, nil
}

func (m *Mem) Snapshots(_ context.Context, userID, deviceID string, limit int) ([]Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Snapshot
	for _, s := range m.snaps[userID] {
		if deviceID != "" && s.DeviceID != deviceID {
			continue
		}
		s.Manifest, s.Sig = nil, nil
		out = append(out, s)
	}
	// Desempata por id: dos snapshots del mismo instante salían en orden
	// aleatorio, y entonces `limit` se quedaba con cualquiera de los dos.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Created.Equal(out[j].Created) {
			return out[i].Created.After(out[j].Created)
		}
		return out[i].ID > out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Mem) Snapshot(_ context.Context, userID, id string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.snaps[userID][id]
	if !ok {
		return Snapshot{}, ErrNotFound
	}
	// Se devuelve copia: quien llama no debe poder reescribir el manifiesto
	// guardado, que en Postgres sería una fila.
	s.Manifest, s.Sig = bytes.Clone(s.Manifest), bytes.Clone(s.Sig)
	return s, nil
}

func (m *Mem) Audit(_ context.Context, userID, deviceID, action string, _ map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, userID+" "+deviceID+" "+action)
	return nil
}

func (m *Mem) Ping(context.Context) error { return nil }
