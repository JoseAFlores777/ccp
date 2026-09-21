package store

// Grupos de dispositivos en Postgres. Ver el contrato en store.go y las reglas
// que los sostienen en migrations/0007_groups.sql.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// dupName traduce el índice único del nombre a ErrConflict. Se mira el código
// de Postgres y no el texto del error, que depende del idioma del servidor.
func dupName(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}

// setMembers escribe la membresía entera dentro de la transacción. Los
// miembros se comprueban contra los dispositivos de ESTA cuenta: la clave
// foránea solo garantiza que el equipo existe, no de quién es.
func setMembers(ctx context.Context, tx pgx.Tx, userID, groupID string, members []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM device_group_members
		WHERE user_id = $1::uuid AND group_id = $2::uuid`, userID, groupID); err != nil {
		return err
	}
	for _, d := range members {
		if !IsUUID(d) {
			return ErrNotFound
		}
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT true FROM devices
			WHERE user_id = $1::uuid AND id = $2::uuid`, userID, d).Scan(&ok); err != nil {
			return notFound(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO device_group_members (user_id, group_id, device_id)
			VALUES ($1::uuid, $2::uuid, $3::uuid)`, userID, groupID, d); err != nil {
			return err
		}
	}
	return nil
}

func groupMembers(ctx context.Context, q pgx.Tx, userID, groupID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT device_id::text FROM device_group_members
		WHERE user_id = $1::uuid AND group_id = $2::uuid ORDER BY device_id::text`, userID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *PG) CreateGroup(ctx context.Context, userID string, g Group) (Group, error) {
	if !IsUUID(userID) {
		return Group{}, ErrNotFound
	}
	members := normMembers(g.Members)
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Group{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var out Group
	err = tx.QueryRow(ctx, `INSERT INTO device_groups (user_id, name) VALUES ($1::uuid, $2)
		RETURNING id::text, name, created, updated`, userID, g.Name).
		Scan(&out.ID, &out.Name, &out.Created, &out.Updated)
	if err != nil {
		return Group{}, dupName(err)
	}
	if err := setMembers(ctx, tx, userID, out.ID, members); err != nil {
		return Group{}, err
	}
	out.Members = members
	return out, tx.Commit(ctx)
}

func (p *PG) Groups(ctx context.Context, userID string) ([]Group, error) {
	if !IsUUID(userID) {
		return []Group{}, nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id::text, name, created, updated FROM device_groups
		WHERE user_id = $1::uuid ORDER BY name, id::text`, userID)
	if err != nil {
		return nil, err
	}
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Created, &g.Updated); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Members, err = groupMembers(ctx, tx, userID, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, tx.Commit(ctx)
}

func (p *PG) Group(ctx context.Context, userID, id string) (Group, error) {
	if !IsUUID(userID) || !IsUUID(id) {
		return Group{}, ErrNotFound
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Group{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var g Group
	err = tx.QueryRow(ctx, `SELECT id::text, name, created, updated FROM device_groups
		WHERE user_id = $1::uuid AND id = $2::uuid`, userID, id).
		Scan(&g.ID, &g.Name, &g.Created, &g.Updated)
	if err != nil {
		return Group{}, notFound(err)
	}
	if g.Members, err = groupMembers(ctx, tx, userID, g.ID); err != nil {
		return Group{}, err
	}
	return g, tx.Commit(ctx)
}

// UpdateGroup escribe nombre y miembros de una vez: dos llamadas dejarían un
// instante con el nombre nuevo y los miembros viejos, que es justo lo que se
// leería si alguien publicara al grupo en medio.
func (p *PG) UpdateGroup(ctx context.Context, userID string, g Group) (Group, error) {
	if !IsUUID(userID) || !IsUUID(g.ID) {
		return Group{}, ErrNotFound
	}
	members := normMembers(g.Members)
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Group{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var out Group
	err = tx.QueryRow(ctx, `UPDATE device_groups SET name = $3, updated = now()
		WHERE user_id = $1::uuid AND id = $2::uuid
		RETURNING id::text, name, created, updated`, userID, g.ID, g.Name).
		Scan(&out.ID, &out.Name, &out.Created, &out.Updated)
	if err != nil {
		return Group{}, dupName(notFound(err))
	}
	if err := setMembers(ctx, tx, userID, out.ID, members); err != nil {
		return Group{}, err
	}
	out.Members = members
	return out, tx.Commit(ctx)
}

func (p *PG) DeleteGroup(ctx context.Context, userID, id string) error {
	if !IsUUID(userID) || !IsUUID(id) {
		return ErrNotFound
	}
	tag, err := p.pool.Exec(ctx, `DELETE FROM device_groups
		WHERE user_id = $1::uuid AND id = $2::uuid`, userID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GroupRevisions: un groupID vacío no devuelve «todas», devolvería las
// sueltas. Un limit <= 0 sí las devuelve todas (ver el contrato en store.go):
// `LIMIT NULL` es como se dice «sin tope» sin montar dos consultas.
func (p *PG) GroupRevisions(ctx context.Context, userID, groupID string, limit int) ([]Revision, error) {
	if !IsUUID(userID) || groupID == "" {
		return []Revision{}, nil
	}
	var tope *int
	if limit > 0 {
		tope = &limit
	}
	rows, err := p.pool.Query(ctx, `SELECT `+revCols+` FROM revisions r
		JOIN devices d ON d.id = r.device_id
		WHERE r.user_id = $1::uuid AND r.group_id = $2
		ORDER BY r.created DESC, r.id DESC LIMIT $3`, userID, groupID, tope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Revision{}
	for rows.Next() {
		r, err := scanRevision(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
