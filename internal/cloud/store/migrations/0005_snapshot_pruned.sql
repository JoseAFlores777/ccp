-- La retención del servidor (spec §10.3.1). Podar se lleva el manifiesto y los
-- blobs, que es todo lo que ocupa, pero la FILA se queda: su id, su padre, su
-- fecha, su digest y su firma son el eslabón, y sin él la cadena tendría un
-- hueco indistinguible del que deja un servidor que te quita un snapshot.
ALTER TABLE snapshots ADD COLUMN pruned_at timestamptz;
CREATE INDEX snapshots_live ON snapshots (user_id, created DESC) WHERE pruned_at IS NULL;
