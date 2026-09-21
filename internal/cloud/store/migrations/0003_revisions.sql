-- Revisiones deseadas (spec §10.3): «el portal propone, la máquina aplica».
-- El cuerpo y la firma son opacos para el servidor, que no tiene con qué
-- firmar: puede negarse a servir una revisión, pero no fabricarla ni cambiarle
-- el destinatario sin que la máquina lo note al verificar.
CREATE TABLE revisions (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    id         text NOT NULL,
    prev       text NOT NULL DEFAULT '',
    device_id  uuid NOT NULL REFERENCES devices (id),
    snapshot   text NOT NULL DEFAULT '',
    base       text NOT NULL DEFAULT '',
    body       bytea NOT NULL,
    sig        bytea NOT NULL,
    created    timestamptz NOT NULL,
    state      text NOT NULL,
    reason     text NOT NULL DEFAULT '',
    updated    timestamptz NOT NULL,
    created_by uuid NOT NULL REFERENCES devices (id),
    PRIMARY KEY (user_id, id)
);
CREATE INDEX revisions_device ON revisions (user_id, device_id, created DESC);

-- Como mucho una pendiente por dispositivo. Es la regla de la cadena escrita
-- en el esquema: sin ella, dos publicaciones simultáneas dejarían al equipo
-- dos órdenes vigentes y ninguna forma de saber cuál obedecer.
CREATE UNIQUE INDEX revisions_one_pending ON revisions (user_id, device_id)
    WHERE state = 'pending';
