---
description: Continúa una sesión de Claude Code en otro perfil (handoff multi-activo)
argument-hint: "[<perfil>|resume|end|discard|status|list] [<uuid>] [--yolo]"
---

Ayuda al usuario con `ccp handoff`: presta una sesión de Claude Code a otro perfil
y la devuelve al terminar. Desde v2 puede haber **varios handoffs en vuelo a la
vez** (uno por sesión, en tantos repos como quieras).

Argumento (opcional): $ARGUMENTS

## Qué puedes ejecutar tú y qué no

- **Solo lectura — ejecútalo tú**: `ccp handoff status [--all]` y `ccp handoff list`.
  No tocan estado y funcionan en cualquier lado.
- **Cambia estado pero no lanza `claude`**: `ccp handoff discard [<uuid>]`. Archiva un
  marcador; no lo ejecutes por tu cuenta — proponlo y espera a que el usuario lo pida.
- **Lanzan `claude` — NO los ejecutes**: `ccp handoff <perfil>`, `ccp handoff resume`
  y `ccp handoff end` corren a través de la función shell de ccp y **reemplazan la
  sesión de Claude Code en curso** (esta). Muéstrale al usuario el comando exacto
  para que lo pegue él en su terminal, y explícale qué va a pasar.

## Superficie

```
ccp handoff                       # panel: ves los activos y eliges qué hacer
ccp handoff <perfil>              # nuevo handoff hacia <perfil>
ccp handoff <perfil> --session <uuid> [--no-marker] [--force]
ccp handoff resume  [<uuid>]      # vuelve a entrar a un handoff vivo (no lo cierra)
ccp handoff end     [<uuid>]      # trae el contexto de vuelta al perfil origen
ccp handoff discard [<uuid>]      # suelta un marcador sin traer nada de vuelta
ccp handoff status  [--all]       # activos de este proyecto (o todos)
ccp handoff list                  # activos + historial
```

`ccp handoff` sin argumentos y con TTY abre el panel gestor: los activos de este
repo primero, `enter` reanudar · `e` terminar (pide confirmación) · `n` nuevo ·
`y` toggle skip-permissions · `q` salir. Sin ningún activo el panel ni se abre:
entras directo al wizard de handoff nuevo, perfil → sesión.

## Cómo elige `end` / `resume` / `discard` cuál handoff

Por el **directorio actual**:

- Exactamente **uno** activo en este repo → actúa sobre ese, sin preguntar.
- **Varios** activos en este repo → pregunta (picker con TTY; sin TTY falla
  pidiendo `--session <uuid>`).
- **Ninguno** aquí → falla y te dice en qué repos sí los hay.

Pasar el `<uuid>` explícito (posicional o `--session`) se salta la resolución.

## `discard` — para el marcador zombi

`discard` archiva el marcador **sin back-sync**: no copia ni reescribe transcripts,
no toca el jsonl que quedó en el perfil destino y no cambia el perfil de la shell.
Es la salida cuando ese jsonl ya no existe (limpiaste `~/.claude`, borraste el
perfil destino…): ahí `end` y `resume` fallan siempre y el marcador se quedaría
activo para siempre — secuestrando la resolución por cwd de ese repo, contando
para el aviso de 5 activos y bloqueando el forward desde el perfil destino.

Lo que hubiera en el destino sigue en disco: se puede reanudar a mano con
`claude --resume <uuid>` desde ese perfil. `discard` **no** es la forma normal de
cerrar un handoff — para eso está `end`, que sí devuelve el contexto.

## `--dangerously-skip-permissions`

Las tres operaciones que lanzan `claude` (`handoff`, `resume`, `end`) aceptan
`--dangerously-skip-permissions`, con alias `--yolo`: arranca la sesión reanudada
saltándose los prompts de permiso de Claude Code. **No se recuerda entre
invocaciones** — no se guarda en el marcador ni en `ccp.yaml`; se pide cada vez
(o se activa con `y` en el panel). No lo sugieras tú: solo úsalo si el usuario
lo pide.

## Reglas del feature que conviene explicar

- `end` hace back-sync del contexto actualizado al perfil origen como **sesión
  nueva** (no destructivo); la sesión de vuelta lleva `[de <perfil>]` en el título.
- Una misma sesión **no** se presta a dos perfiles a la vez, y **no** hay cadenas
  multi-nivel (`A → B → C` **sobre la misma sesión**): hay que hacer `end` primero.
  El bloqueo es por sesión, no por repo: prestar hacia adelante *otra* sesión del
  mismo proyecto sí se puede.
- A partir de 5 handoffs sin cerrar, un forward avisa (no bloquea).
- Al entrar (`cd`) a un repo con handoff activo, el hook imprime un recordatorio.
- `ccp handoff status` sale con código **0** si este repo tiene algún handoff
  activo y **1** si no — útil para scripts. `discard` sale **0** si archivó el
  marcador y **1** si no encontró cuál (o hay varios y no hay TTY).
- Todo lo que lanza `claude` (`handoff`, `resume`, `end`) necesita la función shell
  instalada (`ccp install` + recargar el rc); `status`, `list` y `discard` no.
