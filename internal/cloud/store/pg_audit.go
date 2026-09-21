package store

// El registro de auditoría en Postgres. Ver audit.go para las reglas.

import (
	"context"
	"encoding/json"
	"time"
)

// Audit solo inserta. user_id y device_id admiten NULL a propósito: hay cosas
// que apuntar (un login que no llegó a cuajar) sin una cuenta o un equipo aún.
func (p *PG) Audit(ctx context.Context, userID, deviceID, action string, detail map[string]any) error {
	d, err := json.Marshal(SanitizeAuditDetail(detail))
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `INSERT INTO audit_log (user_id, device_id, action, detail)
		VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, $4::jsonb)`,
		userID, deviceID, action, string(d))
	return err
}

// AuditLog lee el registro. El nombre del equipo sale de un LEFT JOIN: el
// registro guarda el id, y un equipo dado de baja deja su id apuntado con el
// nombre en blanco en vez de borrar la línea.
func (p *PG) AuditLog(ctx context.Context, userID string, f AuditFilter) ([]AuditEntry, error) {
	if !IsUUID(userID) {
		return nil, ErrNotFound
	}
	lim := f.Limit
	if lim <= 0 {
		lim = DefaultAuditLimit
	}
	dev := ""
	if IsUUID(f.DeviceID) {
		dev = f.DeviceID
	} else if f.DeviceID != "" {
		return []AuditEntry{}, nil // un equipo que no es un id no apuntó nada
	}
	since := f.Since
	if since.IsZero() {
		since = time.Unix(0, 0).UTC()
	}
	rows, err := p.pool.Query(ctx, `SELECT a.id, a.at, COALESCE(a.device_id::text, ''),
			COALESCE(d.name, ''), a.action, a.detail
		FROM audit_log a LEFT JOIN devices d ON d.id = a.device_id
		WHERE a.user_id = $1::uuid
		  AND ($2 = '' OR a.device_id = NULLIF($2, '')::uuid)
		  AND ($3 = '' OR a.action = $3)
		  AND a.at >= $4
		ORDER BY a.at DESC, a.id DESC LIMIT $5`, userID, dev, f.Action, since, lim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var raw []byte
		if err := rows.Scan(&e.ID, &e.At, &e.DeviceID, &e.DeviceName, &e.Action, &raw); err != nil {
			return nil, err
		}
		e.Detail = map[string]any{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &e.Detail)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
