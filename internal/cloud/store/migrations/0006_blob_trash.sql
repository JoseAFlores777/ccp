-- Un blob liberado por la poda se va de `blobs` pero deja una nota aquí hasta
-- que se confirme que el objeto ya no está en el bucket. Sin la nota, un
-- borrado que falla (un 5xx pasajero del almacenamiento, o el proceso muriendo
-- entre el commit y el borrado) dejaba un objeto que ninguna poda posterior
-- podía volver a nombrar —`DELETE ... RETURNING` solo devuelve lo que sigue en
-- la tabla— y ocupaba sitio para siempre.
CREATE TABLE blob_trash (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    blob_id text NOT NULL,
    since   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, blob_id)
);
