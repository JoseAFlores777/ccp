-- Esquema inicial de ccp-cloud. Nada de lo que hay aquí se puede abrir sin la
-- clave de cuenta del usuario: bóvedas, manifiestos y firmas son opacos.

CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sub        text NOT NULL UNIQUE,
    email      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE vaults (
    user_id         uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    kdf             jsonb NOT NULL,
    passphrase_wrap bytea NOT NULL,
    recovery_wrap   bytea NOT NULL,
    sign_pub        bytea NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        text NOT NULL,
    platform    text NOT NULL DEFAULT '',
    ccp_version text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    last_seen   timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz
);
CREATE INDEX devices_user ON devices (user_id);

CREATE TABLE blobs (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    id         text NOT NULL,
    size       bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);

CREATE TABLE snapshots (
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    id          text NOT NULL,
    parent      text NOT NULL DEFAULT '',
    device_id   uuid NOT NULL REFERENCES devices (id),
    created     timestamptz NOT NULL,
    manifest    bytea NOT NULL,
    sig         bytea NOT NULL,
    size        bigint NOT NULL DEFAULT 0,
    pinned      boolean NOT NULL DEFAULT false,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);
CREATE INDEX snapshots_user_created ON snapshots (user_id, created DESC);

CREATE TABLE snapshot_blobs (
    user_id     uuid NOT NULL,
    snapshot_id text NOT NULL,
    blob_id     text NOT NULL,
    PRIMARY KEY (user_id, snapshot_id, blob_id),
    FOREIGN KEY (user_id, snapshot_id) REFERENCES snapshots (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, blob_id) REFERENCES blobs (user_id, id)
);

-- Solo inserción: el servidor nunca actualiza ni borra filas de aquí.
CREATE TABLE audit_log (
    id        bigserial PRIMARY KEY,
    user_id   uuid REFERENCES users (id) ON DELETE SET NULL,
    device_id uuid,
    action    text NOT NULL,
    detail    jsonb NOT NULL DEFAULT '{}',
    at        timestamptz NOT NULL DEFAULT now()
);
