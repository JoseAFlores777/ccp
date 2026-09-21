-- La sesión de Keycloak (claim `sid`) que dio de alta el equipo. Revocar uno
-- revoca a sus hermanos de sesión y bloquea un alta nueva con el mismo token:
-- sin esta columna, revocar solo quemaba un id y la credencial robada seguía
-- viva. Los equipos dados de alta antes se quedan con '' y no atan a nadie.
ALTER TABLE devices ADD COLUMN session_id text NOT NULL DEFAULT '';
CREATE INDEX devices_session ON devices (user_id, session_id) WHERE session_id <> '';
