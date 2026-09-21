-- La auditoría de §10.5 deja de ser solo escritura: el portal y `ccp cloud
-- audit` la leen. Se lee siempre igual —lo último de una cuenta, a veces
-- acotado a un equipo o a una acción—, así que el índice va por (cuenta,
-- fecha) y (cuenta, equipo, fecha). Sin él, mirar quién publicó qué recorre
-- el registro entero de todas las cuentas.
CREATE INDEX audit_log_user_at ON audit_log (user_id, at DESC, id DESC);
CREATE INDEX audit_log_user_device_at ON audit_log (user_id, device_id, at DESC, id DESC)
    WHERE device_id IS NOT NULL;
