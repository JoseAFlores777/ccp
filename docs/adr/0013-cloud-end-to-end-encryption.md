# 13. La nube cifra de punta a punta: el servidor no puede leer la configuración

Fecha: 2026-09-21

## Estado

Aceptada. Implementa el §10.1 del spec
[2026-09-18-config-unificada-snapshots-nube](../superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md)
(decisión D1) y sube el formato que fijó el [ADR 0012](0012-snapshots-content-addressed.md). La identidad —quién
eres, frente a qué puedes leer— es del [ADR 0015](0015-identity-keycloak-vault-separate.md).

Implementada en F1: `internal/cloud/crypt` (cliente), `internal/cloud/api` (el protocolo),
`internal/cloud/{server,store,blobs}` (el backend) y `ccp cloud` (`internal/cli/cloud.go`). El despliegue del
API queda pendiente de autorización del usuario (Task 14 del plan `2026-09-18-nube-f1-boveda-y-sync`).

## Contexto

Un snapshot no es un documento: trae claves de API de proveedores, tokens de servidores MCP, y hooks, comandos
y skills que **se ejecutan** en las máquinas del usuario. Un servidor capaz de leerlos y escribirlos es, si lo
comprometen, dos cosas a la vez: una copia en claro de esas credenciales y ejecución remota de código en todas
las máquinas que sincronicen.

El servidor además no es de un tercero al que reclamar: es un contenedor en un Dokploy propio, actualizado a
mano, junto a otros servicios. Tratarlo como de confianza sería apostar la configuración entera —y las cuentas
que hay detrás— a que nunca se descuide un despliegue.

## Decisión

**Nada sale de un equipo sin cifrar, y lo que sale no se puede abrir con nada que el servidor tenga.**

- Todo se cifra en el cliente con subclaves HKDF de una **clave de cuenta (AK)** de 256 bits que el servidor
  nunca recibe (`crypt.NewAccount`). Los usos son parte del formato —`ccp/v1/data`, `ccp/v1/ids`,
  `ccp/v1/sign`—: cambiar uno deja ilegible todo lo ya subido.
- **Blobs y manifiestos** van sellados con XChaCha20-Poly1305 y **su id como dato asociado**, así que un blob
  copiado bajo otro id no abre. Los blobs se comprimen antes de sellar, y al abrirlos el límite
  `snapshot.MaxBlobSize` frena una bomba gzip: el sellado garantiza *quién* lo escribió, no que sea razonable.
- **Los ids de la nube son HMAC** de los hashes locales (`crypt.BlobID`, `crypt.SnapshotID`). El servidor
  deduplica y encadena sin saber qué guarda, y no puede comprobar si una cuenta tiene un archivo concreto. Para
  la cuenta son estables: dos equipos con el mismo archivo lo suben una sola vez.
- **Cada snapshot va firmado con Ed25519** sobre id, padre y hash del manifiesto sellado. Quien no tiene la AK
  no puede fabricar un snapshot ni reordenar la cadena.
- **El cliente verifica con la clave pública derivada de la AK**, nunca con una que diga el servidor
  (`Account.Verify`, y `checkAK` al desbloquear). Un servidor comprometido puede negar el servicio, pero no
  colar, alterar ni reordenar un snapshot.
- **Los blobs van directos entre el cliente y el almacenamiento**, con URLs prefirmadas que pide
  `POST /v1/blobs/presign`. El API no los lee nunca: ni de subida ni de bajada pasan por él.
- **El servidor no importa el paquete que cifra.** `internal/cloud/crypt` lo usa el cliente; `server`, `store`
  y `blobs` no lo tocan. Y al revés: `go list -deps ./cmd/ccp` no nombra ninguno de los tres, así que el
  binario que el usuario instala no lleva dentro el que sabría leer una base de datos.

## Consecuencias

- **Sin la AK no hay recuperación posible.** La frase de la bóveda y el código de recuperación
  ([ADR 0015](0015-identity-keycloak-vault-separate.md)) son la única salida, y `ccp cloud init` lo dice al
  crear la bóveda, con el código en pantalla una sola vez.
- **El servidor no puede ofrecer búsqueda ni diff sobre el contenido.** El portal (F2) descifra en el
  navegador; el API solo sabe ordenar metadatos.
- **Metadatos visibles, y se aceptan**: fechas, tamaños, el nombre que cada equipo se puso y el grafo de
  padres. Son lo que hace falta para listar, deduplicar y encadenar sin abrir nada.
- **Un blob que no cabe no se sube**: el límite son 64 MiB por elemento, y `ccp cloud push` dice cuáles se
  quedaron fuera en lugar de fallar entero.
- La deduplicación es **por cuenta**, no global: dos usuarios con el mismo archivo lo guardan dos veces. Es el
  precio de que el id sea un HMAC y no el hash.
