package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrateLock es el lock consultivo del migrador: dos instancias que arrancan a
// la vez migran una detrás de otra.
const migrateLock = 727001

// PG es el Store de producción.
type PG struct{ pool *pgxpool.Pool }

// OpenPG abre el pool. dsn es de palabras clave (host=… user=… dbname=…) y la
// contraseña va aparte: una contraseña generada con «@» o «/» rompería una URL.
func OpenPG(ctx context.Context, dsn, password string) (*PG, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: DSN inválida: %w", err)
	}
	if password != "" {
		cfg.ConnConfig.Password = password
	}
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: no se pudo abrir Postgres: %w", err)
	}
	return &PG{pool: pool}, nil
}

// Close cierra el pool.
func (p *PG) Close() { p.pool.Close() }

// Ping comprueba que Postgres responde.
func (p *PG) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// Migrate aplica, en orden, las migraciones que falten.
func (p *PG) Migrate(ctx context.Context) error {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLock); err != nil {
		return fmt.Errorf("store: lock del migrador: %w", err)
	}
	defer func() {
		// Contexto propio: si el de la llamada ya venció, soltar el lock sigue
		// siendo obligatorio o la siguiente instancia se queda esperando.
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrateLock)
	}()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if err := p.applyMigration(ctx, conn.Conn(), e.Name()); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration aplica una migración si falta, en su propia transacción junto
// con la fila que la da por aplicada: o entra entera, o no entra.
func (p *PG) applyMigration(ctx context.Context, conn *pgx.Conn, name string) error {
	v, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
	if err != nil {
		return fmt.Errorf("store: migración con nombre inválido: %s", name)
	}
	var done bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, v).Scan(&done); err != nil {
		return err
	}
	if done {
		return nil
	}
	sql, err := migrationFS.ReadFile("migrations/" + name)
	if err != nil {
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op tras Commit
	// Sin argumentos, pgx usa el protocolo simple: admite varias sentencias.
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("store: migración %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, v); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (p *PG) UpsertUser(ctx context.Context, sub, email string) (User, error) {
	var u User
	err := p.pool.QueryRow(ctx, `INSERT INTO users (sub, email) VALUES ($1, $2)
		ON CONFLICT (sub) DO UPDATE SET email = EXCLUDED.email
		RETURNING id::text, sub, email`, sub, email).Scan(&u.ID, &u.Sub, &u.Email)
	return u, err
}

func (p *PG) Vault(ctx context.Context, userID string) (Vault, error) {
	if !IsUUID(userID) {
		return Vault{}, ErrNotFound
	}
	var v Vault
	var kdf string
	err := p.pool.QueryRow(ctx, `SELECT kdf::text, passphrase_wrap, recovery_wrap, sign_pub, created_at
		FROM vaults WHERE user_id = $1::uuid`, userID).
		Scan(&kdf, &v.PassphraseWrap, &v.RecoveryWrap, &v.SignPub, &v.Created)
	v.KDF = []byte(kdf)
	return v, notFound(err)
}

func (p *PG) CreateVault(ctx context.Context, userID string, v Vault) error {
	if !IsUUID(userID) {
		return ErrNotFound
	}
	if !json.Valid(v.KDF) {
		return fmt.Errorf("store: KDF no es JSON")
	}
	tag, err := p.pool.Exec(ctx, `INSERT INTO vaults (user_id, kdf, passphrase_wrap, recovery_wrap, sign_pub)
		VALUES ($1::uuid, $2::jsonb, $3, $4, $5) ON CONFLICT (user_id) DO NOTHING`,
		userID, string(v.KDF), v.PassphraseWrap, v.RecoveryWrap, v.SignPub)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// revoked_at IS NOT NULL: el instante exacto no sale de aquí, solo si lo está.
const deviceCols = `id::text, name, platform, ccp_version, created_at, last_seen, revoked_at IS NOT NULL, session_id`

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.Name, &d.Platform, &d.CCPVersion, &d.Created, &d.LastSeen, &d.Revoked, &d.SessionID)
	return d, notFound(err)
}

// RewrapVault reemplaza las envolturas y nada más. El WHERE sign_pub exige
// que la clave pública de firma sea la misma: es la prueba de que la AK no
// cambió, y una AK nueva no es rotar las llaves sino cambiar la caja dejando
// dentro todo lo que ya no abre. Created no se toca: la bóveda es la misma.
func (p *PG) RewrapVault(ctx context.Context, userID string, v Vault) error {
	if !IsUUID(userID) {
		return ErrNotFound
	}
	if !json.Valid(v.KDF) {
		return fmt.Errorf("store: KDF no es JSON")
	}
	tag, err := p.pool.Exec(ctx, `UPDATE vaults SET kdf = $2::jsonb, passphrase_wrap = $3, recovery_wrap = $4
		WHERE user_id = $1::uuid AND sign_pub = $5`,
		userID, string(v.KDF), v.PassphraseWrap, v.RecoveryWrap, v.SignPub)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// O no hay bóveda, o la firma no cuadra: son cosas distintas y quien
		// llama necesita distinguirlas.
		var existe bool
		if err := p.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vaults WHERE user_id = $1::uuid)`,
			userID).Scan(&existe); err != nil {
			return err
		}
		if !existe {
			return ErrNotFound
		}
		return ErrConflict
	}
	return nil
}

func (p *PG) CreateDevice(ctx context.Context, userID string, d Device) (Device, error) {
	if !IsUUID(userID) {
		return Device{}, ErrNotFound
	}
	return scanDevice(p.pool.QueryRow(ctx, `INSERT INTO devices (user_id, name, platform, ccp_version, session_id)
		VALUES ($1::uuid, $2, $3, $4, $5) RETURNING `+deviceCols, userID, d.Name, d.Platform, d.CCPVersion, d.SessionID))
}

func (p *PG) Devices(ctx context.Context, userID string) ([]Device, error) {
	if !IsUUID(userID) {
		return []Device{}, nil
	}
	rows, err := p.pool.Query(ctx, `SELECT `+deviceCols+`
		FROM devices WHERE user_id = $1::uuid ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *PG) SeenDevice(ctx context.Context, userID, deviceID string, at time.Time) (Device, error) {
	if !IsUUID(userID) || !IsUUID(deviceID) {
		return Device{}, ErrNotFound
	}
	return scanDevice(p.pool.QueryRow(ctx, `UPDATE devices SET last_seen = $3
		WHERE user_id = $1::uuid AND id = $2::uuid RETURNING `+deviceCols, userID, deviceID, at))
}

// RevokeDevice es idempotente y conserva el primer instante: COALESCE deja la
// revocación original en pie si alguien vuelve a revocar el mismo equipo.
//
// Va en una transacción con el cierre de su orden pendiente porque son el
// mismo hecho: un equipo revocado no vuelve a preguntar, así que una orden
// suya que siguiera abierta se pintaría como «pendiente» para siempre. El
// WHERE state = 'pending' toca como mucho una fila (el índice único de las
// pendientes) y no reescribe un resultado ya contado.
func (p *PG) RevokeDevice(ctx context.Context, userID, deviceID string, at time.Time) error {
	if !IsUUID(userID) || !IsUUID(deviceID) {
		return ErrNotFound
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op tras Commit

	tag, err := tx.Exec(ctx, `UPDATE devices SET revoked_at = COALESCE(revoked_at, $3)
		WHERE user_id = $1::uuid AND id = $2::uuid`, userID, deviceID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE revisions SET state = $3, updated = $4
		WHERE user_id = $1::uuid AND device_id = $2::uuid AND state = $5`,
		userID, deviceID, api.RevRevoked, at, api.RevPending); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *PG) KnownBlobs(ctx context.Context, userID string, ids []string) (map[string]int64, error) {
	out := map[string]int64{}
	if !IsUUID(userID) || len(ids) == 0 {
		return out, nil
	}
	rows, err := p.pool.Query(ctx, `SELECT id, size FROM blobs
		WHERE user_id = $1::uuid AND id = ANY($2::text[])`, userID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var size int64
		if err := rows.Scan(&id, &size); err != nil {
			return nil, err
		}
		out[id] = size
	}
	return out, rows.Err()
}

// insertSnapshotSQL está fuera para poder comprobarlo sin base de datos: lo
// que esta sentencia omite se queda en el DEFAULT del esquema, y el barrido de
// la misma petición ya lee la fila escrita.
const insertSnapshotSQL = `INSERT INTO snapshots (user_id, id, parent, device_id, created, manifest, sig, size, pinned, manifest_sha256)
	VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6, $7, $8, $9, sha256($6))`

// CommitSnapshot es una transacción, y de ahí salen las dos garantías: un
// snapshot a medias no existe, y una referencia a un blob sin registrar la
// rechaza la clave foránea antes de que nada se haya confirmado.
func (p *PG) CommitSnapshot(ctx context.Context, userID string, s Snapshot, newBlobs []Blob, refs []string) (bool, error) {
	if !IsUUID(userID) || !IsUUID(s.DeviceID) {
		return false, ErrNotFound
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op tras Commit

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM snapshots
		WHERE user_id = $1::uuid AND id = $2)`, userID, s.ID).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, tx.Commit(ctx)
	}
	var devOK bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices
		WHERE user_id = $1::uuid AND id = $2::uuid)`, userID, s.DeviceID).Scan(&devOK); err != nil {
		return false, err
	}
	if !devOK {
		return false, ErrNotFound
	}
	if len(newBlobs) > 0 {
		ids := make([]string, len(newBlobs))
		sizes := make([]int64, len(newBlobs))
		for i, b := range newBlobs {
			ids[i], sizes[i] = b.ID, b.Size
		}
		if _, err := tx.Exec(ctx, `INSERT INTO blobs (user_id, id, size)
			SELECT $1::uuid, unnest($2::text[]), unnest($3::bigint[])
			ON CONFLICT DO NOTHING`, userID, ids, sizes); err != nil {
			return false, err
		}
	}
	// El digest lo calcula Postgres sobre el mismo valor que escribe: así el
	// hash con el que se verifica la firma no puede describir otros bytes que
	// los guardados.
	if _, err := tx.Exec(ctx, insertSnapshotSQL,
		userID, s.ID, s.Parent, s.DeviceID, s.Created, s.Manifest, s.Sig, s.Size, s.Pinned); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return false, nil // otro commit del mismo snapshot ganó la carrera
		}
		return false, err
	}
	if len(refs) > 0 {
		// La clave foránea rechaza una referencia a un blob sin registrar.
		if _, err := tx.Exec(ctx, `INSERT INTO snapshot_blobs (user_id, snapshot_id, blob_id)
			SELECT $1::uuid, $2, unnest($3::text[])
			ON CONFLICT DO NOTHING`, userID, s.ID, refs); err != nil {
			return false, fmt.Errorf("store: referencias del snapshot: %w", err)
		}
	}
	return true, tx.Commit(ctx)
}

const snapCols = `s.id, s.parent, s.device_id::text, d.name, s.created, s.size, s.pinned,
	s.pruned_at IS NOT NULL`

func (p *PG) Snapshots(ctx context.Context, userID, deviceID string, limit int) ([]Snapshot, error) {
	if !IsUUID(userID) {
		return []Snapshot{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	// El desempate por id no es cosmético: dos snapshots del mismo instante
	// salían en orden arbitrario y `limit` se quedaba con cualquiera de ellos.
	rows, err := p.pool.Query(ctx, `SELECT `+snapCols+` FROM snapshots s
		JOIN devices d ON d.id = s.device_id
		WHERE s.user_id = $1::uuid AND s.pruned_at IS NULL AND ($2 = '' OR s.device_id::text = $2)
		ORDER BY s.created DESC, s.id DESC LIMIT $3`, userID, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Snapshot{}
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(&s.ID, &s.Parent, &s.DeviceID, &s.DeviceName, &s.Created, &s.Size, &s.Pinned, &s.Pruned); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PruneSnapshots: ver el contrato en store.go. Todo en una transacción, y los
// blobs solo se borran de la base: quitarlos del bucket es cosa de quien llama,
// después, porque el orden inverso dejaría filas apuntando a objetos que ya no
// están. La fila liberada pasa a `blob_trash` en la misma transacción y se
// devuelve la basura entera, no solo la de esta vuelta: un borrado que falló no
// lo nombraría nadie más (el DELETE ... RETURNING solo devuelve lo que sigue en
// la tabla) y el objeto se quedaría huérfano para siempre.
func (p *PG) PruneSnapshots(ctx context.Context, userID string, ids []string, before time.Time) ([]string, error) {
	if !IsUUID(userID) {
		return []string{}, nil
	}
	if ids == nil {
		ids = []string{}
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op tras Commit

	if _, err := tx.Exec(ctx, `UPDATE snapshots SET pruned_at = now(), manifest = ''::bytea, size = 0
		WHERE user_id = $1::uuid AND id = ANY($2::text[]) AND pruned_at IS NULL`, userID, ids); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM snapshot_blobs
		WHERE user_id = $1::uuid AND snapshot_id = ANY($2::text[])`, userID, ids); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `WITH libres AS (
		DELETE FROM blobs b
		WHERE b.user_id = $1::uuid AND b.created_at < $2
		  AND NOT EXISTS (SELECT 1 FROM snapshot_blobs sb
			WHERE sb.user_id = b.user_id AND sb.blob_id = b.id)
		RETURNING b.id)
		INSERT INTO blob_trash (user_id, blob_id) SELECT $1::uuid, id FROM libres
		ON CONFLICT DO NOTHING`, userID, before); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT blob_id FROM blob_trash WHERE user_id = $1::uuid`, userID)
	if err != nil {
		return nil, err
	}
	libres := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		libres = append(libres, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(libres)
	return libres, tx.Commit(ctx)
}

// ForgetBlobs: ver el contrato en store.go.
func (p *PG) ForgetBlobs(ctx context.Context, userID string, ids []string) error {
	if !IsUUID(userID) || len(ids) == 0 {
		return nil
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM blob_trash
		WHERE user_id = $1::uuid AND blob_id = ANY($2::text[])`, userID, ids)
	return err
}

// SetPinned: ver el contrato en store.go.
func (p *PG) SetPinned(ctx context.Context, userID, id string, pinned bool) error {
	if !IsUUID(userID) {
		return ErrNotFound
	}
	tag, err := p.pool.Exec(ctx, `UPDATE snapshots SET pinned = $3
		WHERE user_id = $1::uuid AND id = $2`, userID, id, pinned)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Chain: ver el contrato en store.go.
func (p *PG) Chain(ctx context.Context, userID string, limit int) ([]Snapshot, error) {
	if !IsUUID(userID) {
		return []Snapshot{}, nil
	}
	if limit <= 0 {
		limit = api.MaxChainLinks
	}
	rows, err := p.pool.Query(ctx, `SELECT s.id, s.parent, s.device_id::text, s.created,
		encode(s.manifest_sha256, 'hex'), s.sig, s.pinned, s.pruned_at IS NOT NULL FROM snapshots s
		WHERE s.user_id = $1::uuid
		ORDER BY s.created DESC, s.id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Snapshot{}
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(&s.ID, &s.Parent, &s.DeviceID, &s.Created, &s.Digest, &s.Sig, &s.Pinned, &s.Pruned); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *PG) Snapshot(ctx context.Context, userID, id string) (Snapshot, error) {
	if !IsUUID(userID) {
		return Snapshot{}, ErrNotFound
	}
	var s Snapshot
	err := p.pool.QueryRow(ctx, `SELECT `+snapCols+`, s.manifest, s.sig FROM snapshots s
		JOIN devices d ON d.id = s.device_id
		WHERE s.user_id = $1::uuid AND s.id = $2`, userID, id).
		Scan(&s.ID, &s.Parent, &s.DeviceID, &s.DeviceName, &s.Created, &s.Size, &s.Pinned, &s.Pruned, &s.Manifest, &s.Sig)
	return s, notFound(err)
}

const revCols = `r.id, r.prev, r.device_id::text, d.name, r.snapshot, r.base, r.created,
	r.state, r.reason, r.updated, r.created_by::text, r.group_id`

// scanRevision lee revCols (+ body y sig si se piden) en una Revision.
func scanRevision(row pgx.Row, full bool) (Revision, error) {
	var r Revision
	dst := []any{&r.ID, &r.Prev, &r.DeviceID, &r.DeviceName, &r.Snapshot, &r.Base, &r.Created,
		&r.State, &r.Reason, &r.Updated, &r.By, &r.Group}
	if full {
		dst = append(dst, &r.Body, &r.Sig)
	}
	return r, row.Scan(dst...)
}

// PublishRevision: ver el contrato en store.go. Todo en una transacción, que
// es lo que hace que reemplazar la cabeza y colocar la nueva sean un solo
// paso: entre los dos no existe un instante con dos pendientes ni con ninguna.
func (p *PG) PublishRevision(ctx context.Context, userID string, r Revision) (Revision, error) {
	if !IsUUID(userID) || !IsUUID(r.DeviceID) || !IsUUID(r.By) {
		return Revision{}, ErrNotFound
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Revision{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op tras Commit

	var devName string
	err = tx.QueryRow(ctx, `SELECT name FROM devices WHERE user_id = $1::uuid AND id = $2::uuid`,
		userID, r.DeviceID).Scan(&devName)
	if err != nil {
		return Revision{}, notFound(err)
	}
	// La cabeza es el eslabón que nadie encadena, no la fila más reciente: el
	// `created` lo pone quien publica y una fecha atrasada dejaría la cadena
	// rota para siempre (la siguiente revisión no encontraría de dónde
	// colgar). No hay bifurcaciones que desempatar porque publicar exige que
	// `prev` sea justo esta fila. El FOR UPDATE solo ordena a dos que lleguen
	// a la vez; quien de verdad impide dos pendientes es el índice único.
	var head string
	err = tx.QueryRow(ctx, `SELECT r.id FROM revisions r
		WHERE r.user_id = $1::uuid AND r.device_id = $2::uuid
		  AND NOT EXISTS (SELECT 1 FROM revisions c
			WHERE c.user_id = r.user_id AND c.device_id = r.device_id AND c.prev = r.id)
		FOR UPDATE`, userID, r.DeviceID).Scan(&head)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Revision{}, err
	}
	if head != r.Prev {
		return Revision{}, ErrConflict
	}
	if head != "" {
		if _, err := tx.Exec(ctx, `UPDATE revisions SET state = $3, reason = $4, updated = $5
			WHERE user_id = $1::uuid AND id = $2 AND state = $6`,
			userID, head, api.RevSuperseded, "reemplazada por "+r.ID, r.Created, api.RevPending); err != nil {
			return Revision{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO revisions
		(user_id, id, prev, device_id, snapshot, base, body, sig, created, state, reason, updated, created_by, group_id)
		VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6, $7, $8, $9, $10, '', $9, $11::uuid, $12)`,
		userID, r.ID, r.Prev, r.DeviceID, r.Snapshot, r.Base, r.Body, r.Sig, r.Created, api.RevPending, r.By, r.Group)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Revision{}, ErrConflict
		}
		return Revision{}, err
	}
	r.DeviceName, r.State, r.Reason, r.Updated = devName, api.RevPending, "", r.Created
	return r, tx.Commit(ctx)
}

func (p *PG) PendingRevision(ctx context.Context, userID, deviceID string) (Revision, error) {
	if !IsUUID(userID) || !IsUUID(deviceID) {
		return Revision{}, ErrNotFound
	}
	r, err := scanRevision(p.pool.QueryRow(ctx, `SELECT `+revCols+`, r.body, r.sig FROM revisions r
		JOIN devices d ON d.id = r.device_id
		WHERE r.user_id = $1::uuid AND r.device_id = $2::uuid AND r.state = $3`,
		userID, deviceID, api.RevPending), true)
	return r, notFound(err)
}

func (p *PG) Revision(ctx context.Context, userID, id string) (Revision, error) {
	if !IsUUID(userID) {
		return Revision{}, ErrNotFound
	}
	r, err := scanRevision(p.pool.QueryRow(ctx, `SELECT `+revCols+`, r.body, r.sig FROM revisions r
		JOIN devices d ON d.id = r.device_id
		WHERE r.user_id = $1::uuid AND r.id = $2`, userID, id), true)
	return r, notFound(err)
}

func (p *PG) Revisions(ctx context.Context, userID, deviceID string, limit int) ([]Revision, error) {
	if !IsUUID(userID) {
		return []Revision{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT `+revCols+` FROM revisions r
		JOIN devices d ON d.id = r.device_id
		WHERE r.user_id = $1::uuid AND ($2 = '' OR r.device_id::text = $2)
		ORDER BY r.created DESC, r.id DESC LIMIT $3`, userID, deviceID, limit)
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

// SetRevisionState acota por dispositivo en el propio UPDATE: comprobar antes
// y escribir después dejaría hueco para que la revisión cambiara en medio.
func (p *PG) SetRevisionState(ctx context.Context, userID, deviceID, id, state, reason string, at time.Time) (Revision, error) {
	if !IsUUID(userID) || !IsUUID(deviceID) {
		return Revision{}, ErrNotFound
	}
	tag, err := p.pool.Exec(ctx, `UPDATE revisions SET state = $4, reason = $5, updated = $6
		WHERE user_id = $1::uuid AND id = $2 AND device_id = $3::uuid AND state = $7`,
		userID, id, deviceID, state, reason, at, api.RevPending)
	if err != nil {
		return Revision{}, err
	}
	if tag.RowsAffected() == 0 {
		// Distinguir «no es tuya» de «ya no estaba pendiente» es lo que deja
		// al cliente saber si reintentar o callarse.
		r, err := p.Revision(ctx, userID, id)
		if err != nil {
			return Revision{}, err
		}
		if r.DeviceID != deviceID {
			return Revision{}, ErrNotFound
		}
		return Revision{}, ErrConflict
	}
	return p.Revision(ctx, userID, id)
}
