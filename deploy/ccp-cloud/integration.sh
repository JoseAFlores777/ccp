#!/usr/bin/env bash
# Levanta Postgres + Alarik locales, corre los tests de integración de la nube y
# lo desmonta todo (también si fallan). Uso: bash deploy/ccp-cloud/integration.sh
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "${here}/../.." && pwd)"
compose=(docker compose -f "${here}/test-stack.yml")

cleanup() { "${compose[@]}" down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT

"${compose[@]}" up -d --wait postgres
"${compose[@]}" up -d alarik
# Alarik no trae healthcheck: está listo cuando responde algo (un 403 sin firma
# vale; lo que no vale es 000, que es "no hay nadie escuchando").
ready=""
for _ in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:58080/ || true)"
  if [[ "${code}" != "000" ]]; then ready=1; break; fi
  sleep 1
done
if [[ -z "${ready}" ]]; then
  echo "alarik no respondió en 60s" >&2
  "${compose[@]}" logs alarik >&2 || true
  exit 1
fi

export CCP_CLOUD_TEST_DSN='host=127.0.0.1 port=55432 user=postgres password=test dbname=postgres sslmode=disable'
export CCP_CLOUD_TEST_S3_ENDPOINT='http://127.0.0.1:58080'
export CCP_CLOUD_TEST_S3_ACCESS_KEY='CCPTESTACCESSKEY0001'
export CCP_CLOUD_TEST_S3_SECRET_KEY='ccp-test-secret-key-0000000000000000000'

# -p 1: el contrato de la persistencia borra las tablas al empezar, así que los
# paquetes tienen que ir de uno en uno. -count=1 para que nada venga de caché.
cd "${root}"
go test -tags integration -p 1 -count=1 ./internal/cloud/...
