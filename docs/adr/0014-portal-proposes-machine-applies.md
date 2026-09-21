# 14. El portal propone y la máquina aplica

Fecha: 2026-09-21

## Estado

Aceptada. Implementa el §10.3 del spec
[2026-09-18-config-unificada-snapshots-nube](../superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md)
(decisión D6) sobre el cifrado del [ADR 0013](0013-cloud-end-to-end-encryption.md) y la identidad del
[ADR 0015](0015-identity-keycloak-vault-separate.md).

Implementada en F2: `internal/cloud/api` y `internal/cloud/{server,store}` (las revisiones deseadas y su
cadena), `internal/cloud/agent` (la reconciliación y la política) y `ccp cloud agent|review|policy`
(`internal/cli/cloud_agent.go`).

## Contexto

El portal tiene que poder decir «devuelve esta Mac al snapshot del martes». Las dos formas evidentes de
hacerlo son malas, cada una a su manera:

- **Que el servidor empuje.** Requiere un puerto, un túnel o una conexión permanente hacia máquinas que están
  detrás de NAT, en cafeterías y apagadas la mitad del día. Y convierte al servidor en algo que, comprometido,
  escribe configuración —hooks, `command` de MCP, skills con scripts— en todas ellas. El [ADR 0013](0013-cloud-end-to-end-encryption.md)
  se tomó la molestia de que el servidor no pudiera **leer** nada; dejarle **escribir** lo desharía entero.
- **Que la máquina aplique lo que el servidor le diga.** Sin puertos, pero con el mismo agujero: la orden
  vendría de quien controle el servidor.

Y hay una tercera cosa que ninguna de las dos resuelve: entre que el portal mira un estado y la máquina lo
aplica, la máquina **ha seguido viviendo**. Alguien editó su `CLAUDE.md`, `/config` tocó su `settings.json`.
Aplicar el estado deseado a ciegas pisa ese trabajo sin decirlo.

## Decisión

**La máquina tira, verifica y decide. El portal solo propone.**

1. **Sin conexiones entrantes.** `ccp cloud agent` consulta cada pocos minutos (y la GUI mientras está
   abierta). No hay puerto, túnel ni nada que escuche en la máquina del usuario.
2. **La orden va firmada con la clave de cuenta**, que el servidor no tiene, y **el destinatario va dentro de
   la firma**. El servidor puede negarse a servir una revisión, pero no fabricarla ni desviarla: copiar la
   orden a otra máquina es trivial, que esa máquina se la crea no lo es.
3. **Las revisiones de cada dispositivo forman una cadena** por `prev`: quitar un eslabón o reordenarlos deja
   el siguiente sin cuadrar.
4. **Reconciliación a tres bandas por ruta lógica**: base (la `base` firmada de la revisión, que es el último
   snapshot aplicado), lo vivo aquí y lo deseado. Lo que solo cambió arriba se aplica; lo que cambió en los
   dos sitios es un **conflicto** y se queda como está; lo que solo cambió aquí se conserva. Una revisión sin
   base es una orden absoluta y se aplica como un restore.
5. **Snapshot automático antes de escribir**, el mismo motor que `ccp snapshot restore`.
6. **Lo ejecutable no se aplica solo, nunca.** Hooks, `command`/`args` de MCP, `statusLine`, plugins, skills
   con script y los permisos que **amplían** esperan a una persona de esta máquina (`ccp cloud review` o la
   GUI). La política del dispositivo (`auto`/`manual`) vive en su disco, no en la cuenta.

## Consecuencias

- **Una cuenta robada no basta para ejecutar código en tus máquinas.** Quien entre en el portal con la frase
  de bóveda puede proponer; para que un hook llegue a existir hace falta además alguien delante de la máquina
  diciendo que sí. Es la única barrera que queda cuando el primer factor ya cayó, y por eso no se salta ni con
  una política «confía en mí».
- **Preguntar de menos es peor, pero preguntar de más también.** La clasificación mira el contenido, no la
  ruta: un `settings.json` que solo cambia `model` se aplica solo. Un «¿seguro?» por cada cambio enseña a
  contestar que sí sin leer, y entonces la barrera deja de existir aunque el código siga ahí.
- **Una revisión se cierra una sola vez**, porque el resultado lo informa solo el destinatario y solo mientras
  sigue pendiente. De ahí que el agente **no** la cierre cuando algo espera confirmación: lo haría con un
  «parcial» que ya no se podría corregir cuando la persona conteste. Mientras espera, el portal la ve
  `pendiente`, que es exactamente lo que es.
- **El portal no puede prometer inmediatez.** Si la máquina está apagada, la orden se aplica al encenderla.
  El estado en pantalla es el que informó la máquina, nunca uno que el servidor deduzca.
- **Lo que la revisión borra no se borra.** El motor de restauración no borra nada que exista en vivo (§8.3);
  el agente lo informa como no aplicado en vez de fingir que la orden se cumplió entera.
- **Consultar cuesta un poco de latencia.** Son unos bytes cada pocos minutos y enterarse tarde solo significa
  esperar; a cambio, la máquina del usuario no abre nada. Si alguna vez hace falta, el long-poll se puede
  añadir sin cambiar ninguna de las decisiones de arriba.
