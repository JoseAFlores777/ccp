#!/bin/sh
# Primer arranque de postgres (docker-entrypoint-initdb.d): una base y un usuario por consumidor.
# keycloak y el backend de ccp no comparten credenciales ni esquema.
# Sin llaves en las variables: Dokploy interpreta el patrón dólar-llave de los file mounts como
# variables de plantilla.
set -eu

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
  -v kc_pass="$KEYCLOAK_DB_PASSWORD" -v ccp_pass="$CCP_DB_PASSWORD" <<'SQL'
CREATE ROLE keycloak LOGIN PASSWORD :'kc_pass';
CREATE DATABASE keycloak OWNER keycloak;
CREATE ROLE ccp LOGIN PASSWORD :'ccp_pass';
CREATE DATABASE ccp OWNER ccp;
REVOKE ALL ON DATABASE keycloak FROM PUBLIC;
REVOKE ALL ON DATABASE ccp FROM PUBLIC;
SQL
