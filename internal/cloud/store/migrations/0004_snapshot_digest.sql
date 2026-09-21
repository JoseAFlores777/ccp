-- El digest del manifiesto (spec §10.3.1): con él se verifica la firma de un
-- eslabón sin bajar el manifiesto, que es lo que permite comprobar la cadena
-- entera de un tirón. Va en columna y no en un `sha256(manifest)` por consulta
-- porque servir la cadena leería entonces todos los manifiestos —megabytes—
-- para devolver unos cuantos kilobytes de hashes.
ALTER TABLE snapshots ADD COLUMN manifest_sha256 bytea NOT NULL DEFAULT '';
-- Los ya publicados: el hash sale de los bytes que ya están guardados, así que
-- nadie tiene que volver a subir nada.
UPDATE snapshots SET manifest_sha256 = sha256(manifest);
