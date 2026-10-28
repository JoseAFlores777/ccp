# ccp-cloud — infraestructura de la nube de ccp

Un solo stack de Docker Compose en Dokploy (proyecto `ccp-cloud`, compose `ccp-cloud`):

| Servicio | Imagen | Dominio | Para qué |
|---|---|---|---|
| `postgres` | `postgres:17.11-alpine` | — (solo interno) | Bases `keycloak` y `ccp`, cada una con su usuario |
| `keycloak` | `quay.io/keycloak/keycloak:26.7.4` | `https://ccp-auth.joseiz.com` | Identidad. Realm `ccp` importado al arrancar |
| `alarik` | `ghcr.io/achtungsoftware/alarik:1.0.0-beta-16` | `https://ccp-s3.joseiz.com` | Blobs S3 (bucket `ccp-blobs`) |
| `api` | `ghcr.io/joseaflores777/ccp-cloud:cloud-v0.1.0` | `https://ccp.joseiz.com` | El backend de ccp: `/v1` y el portal web, servidos desde el mismo origen |

El backend sirve el API y el portal **en el mismo origen** a propósito: es lo que evita negociar
CORS contra tu propio API y lo que permite una CSP estricta en la página que sostiene la clave de
cuenta. Ver `docs/portal.md`.
Diseño completo: `docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md` §10.

## Archivos

- `docker-compose.yml`: el stack. Imágenes fijadas; ningún puerto publicado (enruta Traefik).
- `template.toml`: plantilla de Dokploy. Solo **nombres** de secretos y sus generadores
  (`${password:32}`…): los valores los genera Dokploy en el servidor al importar y los guarda en
  el `.env` del compose. Nunca están en git.
- `files/realm-ccp.json`: el realm `ccp` como código (clientes `ccp-cli` con concesión de
  dispositivo, `ccp-portal` con PKCE, `ccp-api` como audience; fuerza bruta, política de contraseñas,
  roles `ccp-user`/`ccp-admin`, eventos de auditoría).
- `files/init-dbs.sh`: crea las dos bases y sus usuarios en el primer arranque de postgres.
- `import-string.py`: construye la cadena base64 que acepta Dokploy (compose + plantilla + file mounts).
- `Dockerfile`: la imagen del `api` (multi-stage: `golang:1.24-alpine` para construir, distroless sin
  root para correr). Se construye desde la **raíz del repo**, no desde aquí:
  `docker build -f deploy/ccp-cloud/Dockerfile --build-arg VERSION=cloud-v0.1.0 .`
- `test-stack.yml` e `integration.sh`: la pila desechable (Postgres + Alarik en 127.0.0.1) contra la
  que corren los tests de integración. No tiene nada que ver con el despliegue.

## Desplegar el `api` la primera vez

1. **Publicar la imagen**: `git tag cloud-v0.1.0 && git push origin cloud-v0.1.0`. El workflow
   `cloud-image` la construye para amd64 y arm64 y la sube a GHCR.
2. **Visibilidad del paquete**: GitHub → Packages → `ccp-cloud` → *Package settings* → **Public**. Si
   la prefieres privada, registra `ghcr.io` en Dokploy (Settings → Registry) con un token de solo
   lectura.
3. **Actualizar el stack en Dokploy**: pega este `docker-compose.yml` en el compose `ccp-cloud`.
   **No reimportes la plantilla**: regeneraría los secretos que ya están en uso. El servicio `api`
   solo referencia `CCP_DB_PASSWORD`, `CCP_S3_ACCESS_KEY` y `CCP_S3_SECRET_KEY`, que ya existen en el
   `.env`.
4. **Dominio**: en Dokploy, `ccp.joseiz.com` → servicio `api`, puerto `8080`, con HTTPS. Keycloak ya
   trae registrado ese dominio en el cliente `ccp-portal` (redirect y *web origin*).
5. **Comprobar**: `curl -fsS https://ccp.joseiz.com/healthz` (204) y `curl -fsS
   https://ccp.joseiz.com/v1/info` (emisor y clientes). El portal responde en `/`.
6. **Desde tu Mac**: `ccp cloud login https://ccp.joseiz.com`, luego `ccp cloud init` — y guarda el
   código de recuperación fuera de esa máquina.

Las migraciones de Postgres las aplica el propio `api` al arrancar; no hay paso manual.

## Desplegar desde cero

1. En Dokploy: proyecto `ccp-cloud` → compose `ccp-cloud` (raw), con **Isolated Deployment** activado:
   sin él, el `postgres` de otro stack en la red compartida podría responder por el nuestro.
2. Compose → Advanced → Import → pegar la salida de `python3 deploy/ccp-cloud/import-string.py`.
3. Domains: `ccp-auth.joseiz.com` → `keycloak:8080` y `ccp-s3.joseiz.com` → `alarik:8080`, https +
   Let's Encrypt. **Un solo nivel de subdominio**: el certificado universal de Cloudflare cubre
   `*.joseiz.com`, no `*.algo.joseiz.com`.
4. Deploy.

> **Reimportar la plantilla regenera todos los secretos.** Con datos ya creados eso deja a keycloak
> y al backend sin acceso a postgres. Para cambiar el compose o un archivo de `files/` después del
> primer despliegue, edita el compose y los file mounts en Dokploy (o con su API), no reimportes.

## Después del primer despliegue

- **Admin de Keycloak**: usuario `ccp-admin` en el realm `master`, contraseña en Dokploy → compose
  `ccp-cloud` → Environment → `KEYCLOAK_ADMIN_PASSWORD`. Es un admin *temporal* (Keycloak lo avisa):
  crea un admin permanente con MFA y borra el temporal.
- **Tu usuario de ccp**: consola → realm `ccp` → Users → Add user (email), Credentials → contraseña,
  y en *Required user actions* marca «Configure OTP». Hereda `ccp-user` por el rol por defecto.
- **El realm solo se importa si no existe**: lo que cambies en la consola se conserva en cada
  reinicio. Si cambias `realm-ccp.json`, aplícalo en la consola (Realm settings → Partial import) o
  borra el realm para que se reimporte.
- **SMTP**: sin configurar; por eso `verifyEmail` y el reset de contraseña están apagados. Al
  configurarlo, enciéndelos.

## Comprobaciones

```bash
curl -s https://ccp-auth.joseiz.com/realms/ccp/.well-known/openid-configuration | python3 -m json.tool | grep device_authorization
curl -s -o /dev/null -w '%{http_code}\n' https://ccp-s3.joseiz.com/ccp-blobs   # 403 sin firma = vivo
```
