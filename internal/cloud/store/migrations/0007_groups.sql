-- Grupos de dispositivos (spec §10.3, «todas mis Macs»). Un grupo es una
-- etiqueta con miembros: NO autoriza nada. Cada orden sigue yendo firmada a
-- una máquina concreta y publicar «al grupo» es publicar N revisiones, una
-- por miembro. Si el grupo mandara, cambiar quién está dentro —algo que el
-- servidor sí puede hacer, porque la membresía no va firmada— cambiaría a
-- quién obedece una orden ya firmada.
CREATE TABLE device_groups (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    id      uuid NOT NULL DEFAULT gen_random_uuid(),
    name    text NOT NULL,
    created timestamptz NOT NULL DEFAULT now(),
    updated timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);

-- El nombre es el asa del grupo en el CLI, así que no se puede repetir dentro
-- de una cuenta.
CREATE UNIQUE INDEX device_groups_name ON device_groups (user_id, name);

CREATE TABLE device_group_members (
    user_id   uuid NOT NULL,
    group_id  uuid NOT NULL,
    device_id uuid NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, group_id, device_id),
    FOREIGN KEY (user_id, group_id) REFERENCES device_groups (user_id, id) ON DELETE CASCADE
);

-- La etiqueta del grupo en la revisión. Sin clave foránea a propósito: borrar
-- un grupo no borra las órdenes que se publicaron con él, porque esas órdenes
-- pasaron de verdad y su historia no cambia por deshacer la etiqueta.
ALTER TABLE revisions ADD COLUMN group_id text NOT NULL DEFAULT '';
CREATE INDEX revisions_group ON revisions (user_id, group_id, created DESC)
    WHERE group_id <> '';
