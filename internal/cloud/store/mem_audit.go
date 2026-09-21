package store

// El registro de auditoría del Store de memoria. Mismas reglas que el de
// Postgres: solo inserta, se lee del más nuevo al más viejo y el detalle pasa
// siempre por SanitizeAuditDetail.

import (
	"context"
	"sort"
	"time"
)

func (m *Mem) Audit(_ context.Context, userID, deviceID, action string, detail map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditN++
	m.audit[userID] = append(m.audit[userID], AuditEntry{
		ID: m.auditN, At: time.Now().UTC(), DeviceID: deviceID,
		Action: action, Detail: SanitizeAuditDetail(detail),
	})
	return nil
}

func (m *Mem) AuditLog(_ context.Context, userID string, f AuditFilter) ([]AuditEntry, error) {
	if !IsUUID(userID) {
		return nil, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	lim := f.Limit
	if lim <= 0 {
		lim = DefaultAuditLimit
	}
	out := []AuditEntry{}
	for _, e := range m.audit[userID] {
		if f.DeviceID != "" && e.DeviceID != f.DeviceID {
			continue
		}
		if f.Action != "" && e.Action != f.Action {
			continue
		}
		if !f.Since.IsZero() && e.At.Before(f.Since) {
			continue
		}
		if d, ok := m.devices[userID][e.DeviceID]; ok {
			e.DeviceName = d.Name
		}
		e.Detail = cloneDetail(e.Detail)
		out = append(out, e)
	}
	// Del más nuevo al más viejo, desempatando por id: dos entradas del mismo
	// instante tienen que salir siempre en el mismo orden o el límite se
	// queda con cualquiera de las dos.
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > lim {
		out = out[:lim]
	}
	return out, nil
}

func cloneDetail(d map[string]any) map[string]any {
	out := make(map[string]any, len(d))
	for k, v := range d {
		out[k] = v
	}
	return out
}
