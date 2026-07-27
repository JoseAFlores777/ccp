# Auto-Handoff: Handoff automático por límite de uso

**Status**: implementado (fases 1-3) · **Date**: 2026-07-25 · **Author**: ccp workflow (4 arquitecturas rivales + crítica adversarial + síntesis)

> Este documento es el DISEÑO, congelado tal y como se decidió. Lo que de verdad
> se construyó difiere en varios puntos: la lista está al final, en
> [«Desviaciones respecto al diseño»](#desviaciones-respecto-al-diseño). Cuando
> el texto de arriba y el código no coincidan, manda el código.

## Problema

Sesiones de Claude Code de varias horas mueren por límite de uso (5-hour, weekly, Opus). El usuario tiene 3 cuentas oficiales + 1 perfil DeepSeek. `ccp handoff` ya mueve sesiones manualmente entre perfiles. Falta **detección automática del límite + cambio de perfil sin intervención humana**.

## Hard constraint

**Cambiar de perfil REQUIERE reiniciar `claude`.** `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CONFIG_DIR` se leen una sola vez al arrancar. No hay failover multi-provider en Claude Code. `--fallback-model` es mismo proveedor y explícitamente excluye rate limits.

→ Todo diseño requiere un **supervisor externo** que mate y re-lance `claude`.

---

## Señales de detección (verificadas empíricamente, Claude Code 2.1.220)

| Sensor | Proactivo? | Interactive? | Headless? | Calidad |
|--------|-----------|-------------|-----------|---------|
| **statusLine** `rate_limits` | ✅ % + `resets_at` epoch | ✅ | ❌ (sin TTY) | Mejor señal. Solo suscripción. `refreshInterval` configurable. |
| **StopFailure hook** | ❌ (reactivo) | ⚠️ **NO VERIFICADO** | ✅ | Hook con `error` enum. Interactive podría no disparar — el proceso no termina el turno. |
| **stream-json** `api_retry` | ❌ | N/A | ✅ | Documentado. `error:"rate_limit"` + `error_status:429`. |
| **Transcript tail** `.error=="rate_limit"` | ❌ | ✅ | ✅ | Funciona pero reactivo — el turno ya falló. |

**Signal gap más crítico**: StopFailure en interactive con límite de suscripción. 89/89 eventos reales en esta máquina son `five_hour` en cuentas oficiales OAuth. Interactive NO termina el turno — se queda vivo con `/rate-limit-options`. Si StopFailure no dispara, el sensor reactivo interactivo está ciego.

### Discriminación de tipo de límite

El transcript tiene `"error":"rate_limit"` + `"isApiErrorMessage":true` + `"apiErrorStatus":429`. El tipo de ventana está solo en el texto: `"You've hit your {session|weekly|Opus|Sonnet|Fable 5|usage credit} limit"`. El enum completo de `.error`: `authentication_failed, oauth_org_not_allowed, billing_error, rate_limit, overloaded, invalid_request, model_not_found, server_error, unknown, max_output_tokens`.

### Estado predictivo real

`<CLAUDE_CONFIG_DIR>/.claude.json` → `.cachedUsageUtilization` tiene `utilization` por ventana, `resets_at` ISO, `severity` ("normal"/"warning"). El statusLine recibe lo mismo live como `rate_limits.{five_hour,seven_day}.{used_percentage,resets_at}` con `resets_at` en epoch seconds.

**Caveat**: `five_hour.utilization` lee `0` en ambos caches poblados localmente mientras `seven_day` lee 83 y 13. Posiblemente no se refresca entre muestras del statusline. Necesita verificación empírica.

---

## 4 arquitecturas rivales evaluadas

Las 4 fueron diseñadas por subagentes independientes y sometidas a crítica adversarial. Veredictos:

### 1. Supervisor (`ccp session`) — **Elegida como base**

Go binary fork/exec `claude` como hijo. Cero cambios al rc. Llega por `*) command ccp "$@"` (ya existe en shellinit.go).

**Crítica**: viable-with-changes. Quitar interactive de v1, ship solo headless. Pre-asignar session uuid con `--session-id`.

### 2. Budget scheduler — **Graft: capa de política**

Pre-emptive: muestrear `.cachedUsageUtilization`, armar handoff al 85-95%, frenar en Stop hook.

**Crítica**: viable-with-changes. `five_hour.utilization` marca 0 — posible ventana equivocada. Mejor detector que scheduler completo.

### 3. Sentinel Relay — **Graft: detección por hooks**

Hooks en cc-home/settings.json detectan, `_ccp_autocheck` en prompt loop actúa.

**Crítica**: viable-with-changes. Actuación desde precmd es frágil (flock bloqueante, no funciona dentro del subshell de handoff). Conservar detección, mover actuación al supervisor.

### 4. Router proxy — **Feature separado**

localhost HTTP proxy multiplexa API keys. Sin restart.

**Crítica**: viable-with-changes. Pero **0 de 89 límites reales** son API-key — todos son OAuth/suscripción. El proxy no puede servir ninguno. Feature independiente para pool de API keys. Borrar daemon long-lived, hacerlo per-session foreground child.

---

## Arquitectura elegida: `ccp session` + capas

```
ccp session [--policy <name>] [--headless] [--max-hops N] [--yolo] [-- claude args...]
```

**Zero rc changes**. `internal/core/shellinit.go:35` ya tiene `*) command ccp "$@"` — el subcomando nuevo llega al binario sin reinstalar.

### Flujo

```
1. ccp session lee política → orden de perfiles + umbrales
2. Fork/exec claude (profile[0] env) con stdio heredado (interactive) o pipe (headless)
3. Monitorea 3 sensores en goroutines:
   a. statusLine → rate_limits.five_hour.used_percentage ≥ umbral → armar handoff
   b. StopFailure hook → sentinel file → handoff inmediato (backstop)
   c. stream-json api_retry (headless) → handoff inmediato
4. Al detectar límite:
   a. SIGTERM a claude (exit 143, SessionEnd hooks corren)
   b. core.HandoffForward(home, from, to, cwd, session, ...) en-process
   c. Re-exec claude --resume <uuid> con profile[i+1] env
5. Si no hay más perfiles → exit con mensaje, marker parked
```

### Detección en 3 capas

| Capa | Mecanismo | Dispara en |
|------|-----------|-----------|
| **Proactiva** | statusLine `rate_limits.used_percentage ≥ threshold` | Antes del límite, turn boundary limpio |
| **Reactiva** | StopFailure hook → sentinel `/tmp/ccp-limit-<sid>` | Límite alcanzado, API error |
| **Headless** | stream-json `api_retry` event `error:"rate_limit"` | Igual que StopFailure, más limpio |

### Principio de retorno al primario

El perfil **primario** de un proyecto es el que `ccp resolve $PWD` devuelve — el dueño natural según las reglas de path. Es donde el usuario quiere estar. Los demás perfiles en la cadena son **préstamos temporales**.

El supervisor siempre intenta **volver al primario** apenas su cooldown expira, sin importar en qué eslabón de la cadena esté. No es un round-robin circular: es un péndulo que oscila hacia afuera y vuelve al origen.

```
primario (personal-cc)
  │
  ├──[agotado]──→ fallback[0] (app-cc)
  │                    │
  │                    ├──[agotado]──→ fallback[1] (personal-deepseek)
  │                    │                    │
  │                    │                    ├──[agotado]──→ ...más fallback...
  │                    │                    │
  │                    │                    ├──[primario resets_at pasó?]──→ ✅ VOLVER
  │                    │                    │
  │                    ├──[primario resets_at pasó?]──→ ✅ VOLVER
  │                    │
  └──[resets_at pasó]──→ ✅ se reanuda aquí, nunca se fue
```

El primario **no está en `fallback`** — es implícito, resuelto por las reglas del path. `fallback` solo lista los préstamos en orden de preferencia.

### Política (nuevas keys en ccp.yaml)

```yaml
auto_handoff:
  enabled: true
  policies:
    default:
      # Préstamos en orden. El primario es implícito (ccp resolve $PWD).
      fallback: [app-cc, personal-deepseek]
      threshold: 90          # % de rate_limits para handoff proactivo
      min_dwell: 20m         # mínimo en cada perfil antes de rotar
      max_hops: 6            # máximo de préstamos antes de rendirse
      return_check: 10m      # cada cuánto verificar si el primario ya es elegible
      return_idle: 90s       # …y cuánto tiene que llevar CALLADA la sesión para
                             # que ese regreso proactivo pueda matar al hijo
                             # (0s = opt-out: volver aunque sea a mitad de turno)
      cooldown:
        strategy: resets_at  # usar epoch resets_at del rate_limits del primario
        fallback: 1h         # si no hay resets_at (API-key), espera fija

    overnight:
      fallback: [personal-cc, app-cc]
      # primario implícito: personal-deepseek (el path /Prueba-deepseek tiene regla)
      threshold: 85
      max_hops: 12
      return_check: 10m
      cooldown:
        strategy: resets_at
        fallback: 2h

    trabajo:
      fallback: []            # sin préstamos. Solo primario (emco-cc)
      # Si el primario se agota → bloqueo, sin handoff automático

  allow_from:                 # compliance gate: default deny
    emco-cc: [emco-cc]        # solo a sí mismo (no rota)
    app-cc: [app-cc, personal-cc]
    personal-cc: [personal-cc, app-cc, personal-deepseek]
    personal-deepseek: [personal-deepseek, personal-cc]
```

**Compliance gate**: `allow_from` explícito. Default REFUSE para paths no matcheados. EMCO/Applaudo no rotan a personal/DeepSeek automáticamente. La historia ya muestra 5 handoffs manuales cruzando esta línea — automatizar sin gate la cruzaría a las 3am sin supervisión.

### Lógica de retorno al primario

> Este pseudocódigo es el **diseño**; el regreso implementado exige tres guards
> más (préstamo vivo, `min_dwell` cumplido y la sesión ociosa `return_idle`) y su
> única acción al disparar es matar y relanzar al hijo. Ver **R-2** y **R-4** en
> «Desviaciones respecto al diseño».

```
1. ccp session arranca con primario = ccp resolve $PWD
2. Supervisor lanza claude en el primario
3. Al detectar límite:
   a. Si hay fallback disponible → handoff al siguiente fallback
   b. En cada iteración del loop de monitoreo (cada return_check):
      - Verificar si primario.resets_at ya pasó
      - Si pasó → handoff de vuelta al primario
      - Si no → seguir en el fallback actual
4. Si primario se agota estando ya en fallback:
   - Mismo flujo: avanzar al siguiente fallback
   - Pero return_check sigue mirando al primario, no al fallback[i-1]
5. Si todos los fallback están agotados Y primario sigue en cooldown:
   - Exit con tabla de cooldowns. El usuario relanza cuando primario se libera.
```

### Qué ve el usuario

> Formato del **diseño**. La traza real usa un solo vocabulario para el
> presupuesto y explica por qué la vuelta a casa no lo consume: ver **R-5**.

```
$ ccp session -- claude
# primario = personal-cc (resuelto por reglas del path ~/Documents/Personal)

personal-cc (4h) ──[94%]──→ ⚠️ handoff a app-cc (préstamo 1/2)
app-cc (2h)     ──[return_check: primario resets_at pasó ✅]──→ ⚠️ volviendo a personal-cc
personal-cc     ──[continúa hasta terminar]──→ ✅ exit 0
```

Si el primario no se libera rápido:

```
personal-cc     ──[agotado]──→ app-cc
app-cc          ──[agotado]──→ personal-deepseek
personal-deepseek ──[return_check cada 10min: primario sigue en cooldown...]
personal-deepseek ──[return_check: primario resets_at pasó ✅]──→ ⚠️ volviendo a personal-cc
personal-cc     ──[termina]──→ ✅ exit 0
```

---

## Casos de uso cubiertos

| UC | Cómo | Limitación |
|----|------|-----------|
| **UC-1** Interactive refactor largo | `ccp session` con TTY. statusLine monitorea % → handoff proactivo | ⚠️ Gated en experimento StopFailure-interactive (ver abajo) |
| **UC-2** Overnight headless batch | `ccp session -p --policy overnight`. stream-json api_retry | ✅ Listo en v1 |
| **UC-3** Opus agotado → Sonnet | `per_profile.model` en chain. Mismo perfil, `ANTHROPIC_MODEL=sonnet` | Feature separado, no usa handoff |
| **UC-4** Round-robin 3 cuentas | `fallback: [app-cc, personal-cc]`. Primario implícito. Forward por hop | ⚠️ Invariante no-chain actual bloquea A→B→C misma sesión. Requiere lift |
| **UC-5** Oficial → DeepSeek | Último fallback. `--model` explícito. Advertencia cross-provider | ⚠️ Compatibilidad tool_use DeepSeek NO VERIFICADA |
| **UC-6** Cost-policy: cheap first | `fallback: [personal-cc]`. Primario = deepseek. Escalar a Opus si difícil | Disparador es dificultad, no 429. Fuera de scope v1 |
| **UC-7** Retorno al primario | `return_check: 10m` verifica `resets_at` del primario cada 10 min. Si expiró **y** la sesión lleva `return_idle` en silencio → handoff de vuelta (mata y relanza el hijo) | ✅ Con cooldowns + resets_at del statusLine |
| **UC-8** Múltiples handoffs vivos | v2 ya soporta N activos. Auto-markers self-cleaning | ✅ |
| **UC-9** Cron/CI | `ccp session -p` no necesita shell function | ✅ |

### Edge cases

| Edge | Manejo |
|------|--------|
| **EDGE-1** Mid tool-call | SIGTERM aborta turn. `--resume` re-ejecuta tool (puede no ser idempotente) |
| **EDGE-2** Git/filesystem sucio | Misma cwd, mismo repo, mismo dirty tree. Handoff es migración de conversación, no de working tree |
| **EDGE-3** Subagent fleet corriendo | ⚠️ No entendido. Handoff mueve UN `.jsonl`. Subagents en archivos separados se pierden |
| **EDGE-4** Transcript >29MB | `CopyTranscript` slurpea archivo entero bajo flock. 8MB cap por línea en scanner |
| **EDGE-5** Todos agotados | Exit limpio. Marker parked. No loop infinito |
| **EDGE-6** Usuario dormido | `--yolo` obligatorio para unattended. No se persiste |
| **EDGE-7** Compliance leak | `allow_from` gate. Default deny |
| **EDGE-8** Zombie markers | `auto: true` en Marker. Auto-end/discard al terminar run. `prune --keep N` |
| **EDGE-9** Señal de detección no confiable | 3 capas redundantes. Si todas fallan → exit, notificar |

---

## Plan de implementación

### Fase 1: `ccp session` headless (2-3 semanas)

**Nuevo comando**: `ccp session -p [--policy <name>] [--max-hops N] [--yolo] [--session-id <uuid>] [-- <claude args>]`

**Archivos**:
- `internal/cli/session.go` — dispatch, flags, `cmdSession`
- `internal/supervisor/supervisor.go` — loop: launch → watch → detect → handoff → relaunch
- `internal/supervisor/detect.go` — stream-json `api_retry` parser + exit code interpretation
- `internal/supervisor/policy.go` — carga `auto_handoff` de ccp.yaml, resuelve chain
- `internal/core/auto.go` — `AutoConfig` struct, load/save, defaults

**Sin `--auto-handoff`** se comporta como `ccp run` (un solo launch, sin rotar). `--dry-run` imprime la política resuelta sin lanzar nada.

**Tests**:
- `internal/supervisor/supervisor_test.go` — loop sintético con fake claude bin (exit codes + ndjson stdout)
- `internal/supervisor/detect_test.go` — api_retry parsing fixtures
- Integración con `handoff_store` real en tmp home

**Sin cambios a**: shellinit.go, bash oracle, golden gate, parity test.

### Fase 2: Detección por hooks + monitoreo (1-2 semanas)

**Nuevos comandos**:
- `ccp auto install [--include-default]` — escribe `StopFailure` + `statusLine` en cada cc-home/settings.json (merge capa gestionada en `CfgRegenerate`)
- `ccp auto status [--json]` — hooks instalados, cooldowns, sensores activos
- `ccp auto test --profile <n>` — synthetic StopFailure stdin → verifica ruta de detección

**Archivos**:
- `internal/cli/auto.go` — dispatch
- `internal/core/autohooks.go` — fragmento gestionado de settings.json, merge en `CfgRegenerate`
- `internal/core/autohooks_test.go`

**Experimento crítico**: agotar cuenta interactivamente, verificar si StopFailure dispara. Si no: notificación sin actuación automática.

### Fase 3: Interactive + lift invariantes (2-3 semanas)

**Features**:
- `ccp session` sin `-p` → hereda TTY, launch interactivo
- statusLine como sensor proactivo primario
- StopFailure como backstop reactivo
- Lift invariante no-chain: `HandoffForward` acepta `a.To == from && a.Session == sessionUUID` con flag `--chain` (solo desde supervisor)
- `ccp handoff prune --keep N` para archived ilimitado
- `ccp handoff sessions [--json]` público

**Archivos**:
- `internal/supervisor/tty.go` — TIOCSPGRP, termios save/restore, foreground process group
- `internal/core/handoff.go` — `HandoffHop` (forward encadenado en misma sesión)
- `internal/cli/handoff_prune.go` — prune de archived

### Fase 4: Router proxy (feature independiente, 2-3 semanas)

**Feature**: per-session foreground child (NO daemon). Pool de API keys con failover in-process.

**Nuevo comando**: `ccp pool run [--pool <name>] [-- claude args...]`

**Archivos**:
- `internal/router/server.go` — http.Server en 127.0.0.1:0
- `internal/router/pool.go` — selección, cooldown map, health
- `internal/router/rewrite.go` — model map, header swap, SSE relay
- `internal/router/classify.go` — 429/402/529 → acción

---

## Decisiones y trade-offs

| Decisión | Elegido | Descartado | Por qué |
|----------|---------|-----------|---------|
| **Supervisor location** | Go binary (`ccp session`) | Shell function (`_ccp_autocheck`) | Cron/CI necesitan binario. precmd con flock bloqueante + subshells de handoff es frágil. |
| **Detección primaria** | statusLine `rate_limits` | Transcript tail parsing | Proactivo > reactivo. `resets_at` epoch. Solo suscripción. |
| **Headless v1, interactive v3** | Ship `-p` primero | Ship interactive sin verificar | StopFailure en interactive no está verificado. |
| **Actuación** | SIGTERM → handoff → re-exec | Hook `continue: false` | Hook no puede cambiar provider. SIGTERM documentado (exit 143). |
| **Compliance gate** | `allow_from` por perfil, default deny | Derivar del rules tree | Reglas son geográficas, no de confianza. |
| **No-chain invariant** | Lift con `--chain` explícito | Dejar invariante | Sin lift, round-robin de 3 cuentas = 2× RewriteSession (29MB) + 4 nuevas sesiones. |
| **Router proxy** | Feature independiente, per-session | Daemon compartido | 0/89 límites reales son API-key. Daemon tiene riesgo de puerto/token invalida otras sesiones. |
| **`--yolo`** | Flag per-run, nunca persistido | Auto-detectado | Unattended permission skip es peligroso. |

---

## Riesgos y rollback

| Riesgo | Mitigación | Rollback |
|--------|-----------|----------|
| StopFailure no dispara en interactive | Experimento antes de phase 3 | Phase 3 cancela, headless sigue |
| `five_hour.utilization` lee 0 | Medir en statusline real antes de phase 2 | Degradar a solo reactivo |
| Transcript tool_use no compatible con DeepSeek | Prueba empírica manual | Deshabilitar cross-provider en política |
| Auto-markers contaminan `handoffs.yaml` | `auto: true`, prune, auto-end/discard | `ccp handoff discard` manual |
| Laptop duerme mid-rotación | `min_dwell`, markers sobreviven | Session siguiente arranca fresco |
| Infinite handoff loop | `max_hops` hard cap | N/A |

---

## Unknowns a verificar (experimentos más baratos primero)

> ⚠️ **Los cuatro siguen SIN correr a fecha de esta revisión.** Se implementaron
> igualmente las fases 1-3 (incluida la interactive, que el diseño dejaba
> *gated* tras el experimento #1), asumiendo el riesgo de forma explícita: la
> mitigación fue **redundancia de sensores**, no certeza. La consecuencia
> práctica es que el comportamiento del auto-handoff **en interactive depende de
> estos experimentos**: si StopFailure no dispara (#1), la detección interactiva
> queda apoyada solo en el porcentaje del statusLine (proactivo) y en el tail del
> transcript (reactivo); si además `five_hour.utilization` llega siempre a 0
> (#3), el sensor proactivo se queda mudo para la ventana que causa el 100% de
> los límites reales y solo quedaría el transcript. `ccp auto test` verifica el
> CABLEADO (que el hook llegue, escriba el sentinel y el supervisor lo lea), no
> el comportamiento de Claude Code — no sustituye a ninguno de estos cuatro.

1. **¿StopFailure dispara en interactive con límite de suscripción?** — Agotar cuenta de bajo valor, hook dumpea stdin a archivo. 20 min. **Gate para phase 3.**
2. **¿Transcript Anthropic tool_use → DeepSeek/Kimi/GLM?** — Handoff manual de sesión pequeña, `--resume` en DeepSeek. 10 min. **Gate para cross-provider.**
3. **¿`five_hour.utilization` poblado en statusline?** — Script dumpea stdin de statusline. Ya existe `rate_limits` en docs, verificar que no sea siempre 0.
4. **¿`-p` exit code exacto en límite de suscripción?** — Mismo experimento que #1 pero headless. 5 min extra.

---

## Referencias

- `internal/core/shellinit.go:21-34` — shell function handoff branch, `( eval "$out"; claude --resume )`
- `internal/core/handoff.go` — `HandoffForward`, `HandoffEnd`, `HandoffResume`, invariantes
- `internal/core/handoff_store.go` — `handoffs.yaml` schema v2, `UpdateHandoffs`, flock
- `internal/core/transcript.go` — `CopyTranscript`, `RewriteSession`, `ListSessions`
- `internal/cli/handoff.go` — flags, pickers, `parseHandoffFlags`
- `docs/superpowers/specs/2026-07-25-handoff-multi-design.md` — spec multi-activo v2
- `docs/superpowers/specs/2026-06-19-ccp-handoff-design.md` — spec original v1
- https://code.claude.com/docs/en/hooks — StopFailure, statusLine, hook contracts
- https://code.claude.com/docs/en/headless — stream-json, api_retry, --resume, --session-id
- https://code.claude.com/docs/en/statusline — rate_limits schema, refreshInterval
- https://code.claude.com/docs/en/model-config — --fallback-model (excluye rate limits)
- https://code.claude.com/docs/en/env-vars — env read once at startup

---

## Desviaciones respecto al diseño

Lo que cambió al construirlo. Cada entrada dice **qué se hizo distinto**, **por
qué** y **qué consecuencia práctica tiene**. Las rutas apuntan al código, que es
la fuente de verdad cuando contradice a este documento.

### Alcance

**F4-1 · La fase 4 (router proxy) NO se implementó.** No hay `internal/router/`
ni `ccp pool run`. El motivo estaba ya en la crítica adversarial y se confirmó al
priorizar: **0 de 89 límites reales** de esta máquina son de API key — todos son
OAuth/suscripción, y un proxy que multiplexa API keys no puede servir ninguno de
ellos. Además es un feature **independiente**: no comparte código con el
supervisor (no hay handoff, no hay restart, no hay marcador) y su valor no
depende de que el auto-handoff exista. Consecuencia: quien tenga un pool de API
keys sigue sin failover in-process; su camino es la cadena de `fallback` con
perfiles de provider, que sí funciona pero paga un restart por salto.

**F1-1 · `ccp session` no degrada a `ccp run`.** El diseño decía «sin
`--auto-handoff` se comporta como `ccp run`». No existe tal flag: `ccp session`
SIEMPRE supervisa, y si `auto_handoff` no está configurado o está `enabled:
false` falla con exit 1 y nombra `ccp auto init`
(`internal/cli/session.go`). Un comando que a veces supervisa y a veces no,
según un flag, es un comando que el usuario ejecuta creyendo tener protección
cuando no la tiene. Para lanzar sin supervisión ya está `ccp run`.

### Retorno al primario

**R-1 · La vuelta a casa es `HandoffEndSession`, no un forward inverso.** Cuando
`Chain.Next()` elige el primario y hay un marcador vivo cuyo `From` es ese
primario, el supervisor **cierra el préstamo** en vez de hacer otro salto
(`internal/supervisor/supervisor.go`, rama `home`). `HandoffEndSession` hace el
back-sync del transcript al perfil origen **como sesión NUEVA**: uuid nuevo,
`sessionId` reescrito, marcador archivado con `returned_as`.

*Consecuencia práctica, y es la más visible de toda la lista*: **el uuid de la
sesión cambia al volver a casa**. El `claude --resume <uuid>` que el usuario
tenía apuntado deja de valer a partir de ese momento; por eso el supervisor
imprime el nuevo (`sesión devuelta a <perfil> como <uuid corto>`,
`internal/supervisor/trace.go:traceReturned`) y el bucle continúa con él. La
alternativa —un forward inverso que conservara el uuid— habría dejado la sesión
del primario *prestada a sí misma*, con un marcador que ya no describe nada, y
habría hecho falta inventar una operación de «cerrar sin mover» que no existe.
El precio del uuid nuevo se paga una vez; el marcador incoherente se paga para
siempre.

**R-2 · `return_check` ES un temporizador, y es el único vigilante que no nace
de un sensor.** (Esta entrada describía una brecha —`return_check` parseado,
mostrado y jamás usado como periodo de sondeo— que ya está cerrada; se conserva
reescrita porque el *cómo* condiciona el resto del bucle.) El sondeo vive en
`launchAndWatch` (`internal/supervisor/supervisor.go`) y responde a la pregunta
que ningún sensor sabe hacer: los tres detectores dicen «ESTE perfil se agotó»,
pero nadie avisa de que una cuenta AJENA reabrió su ventana. Sin el temporizador,
la vuelta a casa solo se reevaluaba cuando algo más disparaba — y a las 3am, con
el usuario dormido y el hijo trabajando tranquilo en el préstamo, no dispara
nada: la sesión pasaba la noche entera en la cuenta equivocada.

Cómo está construido, y por qué así:

- **Al vencer, su ÚNICA acción es matar al hijo.** No es un reloj pasivo que
  «compara dos timestamps»: cuando la decisión sale «a casa», el supervisor hace
  `Terminate(10s)` sobre un `claude` **sano** (la rotación por límite mata uno que
  ya no puede trabajar; este, no), cierra el préstamo y **relanza** en el
  primario con el uuid nuevo. Todo lo demás de esta entrada —los guards, el
  orden— existe porque la acción es esa.
- **Se arma con préstamo vivo, no siempre.** La condición vive en
  `armReturnTicker(onLoan, noReturn, check)`: `onLoan` = hay marcador activo *y*
  su `From` es el primario, más `--no-return` desactivado y `return_check > 0`.
  Estando en casa no hay nada que devolver, y un marcador cuyo `From` no es el
  primario (rotación degradada, sin transcript) no describe un camino de vuelta
  — es la misma condición que ya usaba la rama `home` de la rotación. Está
  extraída como función con nombre porque armar de más **no tiene efecto
  observable** en casi ningún estado (estando en casa `ReturnDue` es false y la
  corrida sale idéntica), así que ningún test de integración puede distinguir «no
  se armó» de «se armó y calló»: hay que preguntarle a la condición
  (`TestArmReturnTickerSoloConPrestamoVivo`).
- **Tick de `o.Poll`, periodo de la política.** El ticker corre al ritmo del
  bucle y la comparación contra `o.Now()` impone el periodo real. Así el reloj
  del supervisor no depende de `return_check` y los tests inyectan un reloj falso
  que salta horas sin dormirlas (`supervisor_test.go`, bloque «(h)»).
- **Cede ante un límite pendiente, desencolado o no.** Si ya hay un evento
  esperando a `min_dwell`, el temporizador se abstiene: la rotación por límite
  acaba yendo al primario de todas formas (`Next()` lo prefiere) y así no se
  pierde el agotamiento que alimenta el cooldown de ese perfil. Y justo antes de
  irse a casa **drena el canal sin bloquear** (`pendingLimit`): un `LimitEvent`
  que el `select` todavía no había sacado se perdería entero al morir el hijo
  (Go elige al azar entre canales listos), el perfil que dejamos parecería fresco
  y el siguiente `Next()` quemaría un préstamo en una cuenta que sigue en 429.
- **Respeta `min_dwell`.** Igual que la rotación: no se arranca al usuario de un
  préstamo al que llegó hace treinta segundos.
- **Exige SILENCIO (`return_idle`, 90s por defecto).** Es el guard que separa
  este movimiento de todos los demás. Con solo `min_dwell: 20m` + `return_check:
  10m`, cualquier préstamo de más de veinte minutos se terminaba en el instante
  en que vencía el cooldown del primario — con el usuario tecleando, a mitad de
  un turno o con una tool call en vuelo, que al reanudar se re-ejecuta y puede no
  ser idempotente. La señal es el **mtime del transcript** (crece con cada turno
  y cada resultado de herramienta), comparado contra el reloj de **pared** y no
  contra `o.Now()`, porque un mtime es una marca del sistema de archivos. Dos
  reglas que parecen detalles: un transcript AUSENTE no cuenta como ocioso (no
  saber no es estar callado), y `return_idle: 0s` es el opt-out explícito. Si la
  sesión nunca calla no se fuerza nada: se vuelve cuando el usuario pare, cuando
  el hijo salga solo, o al siguiente límite. Aplica igual en headless: nadie
  delante no hace menos frágil una tool call a medias.
- **NO llama a `MarkExhausted` sobre el perfil que se deja.** No está agotado; se
  abandona voluntariamente. Marcarlo lo desterraría una hora por nada, y ese
  préstamo puede hacer falta dentro de diez minutos.
- **Reutiliza el camino de vuelta.** `childOutcome.returnHome` distingue el caso
  del límite; el bucle lo trata ANTES de la rama de rotación y sin exigir
  `out.limit`, y ejecuta el mismo `HandoffEndSession` + uuid nuevo + resume de
  R-1. La traza lo dice explícitamente porque es el único movimiento que ocurre
  sin que nada haya fallado: `p2 (2h 00m) ──[return_check: p1 ya liberó su
  ventana]──→ volviendo a p1 (vuelta a casa, no gasta préstamo: siguen 1/6)`.

El orden de las cinco reglas vive extraído en `runner.decideReturn` (límite ya
desencolado → `ReturnDue` → `min_dwell` → inactividad → límite encolado → a
casa), fuera del `select`, precisamente para poder probar cada una sin lanzar un
proceso: es la única decisión del supervisor que mata algo sano.

**R-3 · `max_hops` cuenta PRÉSTAMOS: la vuelta a casa ni consume presupuesto ni
lo respeta.** El diseño lo definía como «máximo de saltos» y el primer código lo
implementó literalmente: `Advance` cobraba cualquier movimiento y `Next`
comprobaba el tope *antes* que el regreso. Con el temporizador de R-2 eso se
volvió insostenible en los dos sentidos: con `max_hops: 6`, tres idas y vueltas
agotaban el presupuesto sin haber usado más de dos cuentas; y un presupuesto
consumido dejaba la conversación **encerrada en la cuenta ajena** justo cuando
el primario acababa de liberarse, que es el estado exacto que la política existe
para evitar. Ahora `Advance` no cobra el regreso al primario y `Next` consulta
`ReturnDue` antes del tope (`internal/supervisor/policy.go`). El backstop
anti-bucle sigue intacto: para volver hay que haber salido, y salir SIEMPRE
cuesta un préstamo, así que los regresos están acotados por los préstamos y una
corrida nunca supera `2·max_hops + 1` lanzamientos.

**R-4 · Volver al primario NUNCA crea un marcador invertido: existe una tercera
vía, la adopción.** El diseño solo contemplaba dos caminos de vuelta
(`HandoffEndSession` si hay marcador, `HandoffChain` si no). El segundo era un
fallo: desde un préstamo **degradado** —rotación disparada antes de que el
transcript existiera, así que `HandoffChain` no llegó a correr y nadie abrió
marcador— hacer `HandoffChain(préstamo → primario)` abre un marcador ACTIVO
`{From: préstamo, To: primario}` que ya no se cierra nunca estando en casa, y en
la siguiente salida limpia back-sincroniza la conversación **hacia** la cuenta
ajena: el transcript acaba donde el usuario no está y la traza le dice que
reanude allí. Ahora el salto separa `backHome` (`target == Primary()`, con
marcador o sin él) de `home` (`backHome` **con** marcador vivo), y el primero va
a `runner.adoptHome`: sin transcript en el perfil actual ⇒ uuid nuevo y
`resume=false`; con transcript —la conversación nació DENTRO del préstamo, es
tan real como cualquier otra— ⇒ `core.HandoffAdoptHome`, que la lleva al
primario como sesión nueva (`RewriteSession`, aiTitle prefijado `[de <perfil>]`,
no destructivo) y deliberadamente **no toca `handoffs.yaml`**: no hay préstamo
que cerrar ni que historiar. La rama `returnHome` del temporizador degrada al
mismo helper si alguna vez corriera sin marcador, en vez de llamar a
`HandoffEndSession` y morir con un `%s` vacío.

**R-5 · La traza cuenta el presupuesto con un solo vocabulario.** Consecuencia
directa de R-3, y se pagó tarde: al dejar de cobrar la vuelta a casa, la línea
que la anunciaba siguió imprimiendo el contador con el nombre de antes. El
resultado era una traza sobre la que el usuario **no podía saber cuánto
presupuesto le quedaba**: la vuelta por límite salía como «(vuelta a casa, salto
N/M)» y la vuelta por temporizador como «(vuelta a casa, préstamos N/M)», y
como el regreso no consume presupuesto, dos movimientos consecutivos mostraban
el mismo número sin una palabra de explicación. Ahora los dos caminos comparten
formateador (`trace.go:traceMove`) y las reglas son tres: el contador se llama
siempre «préstamo(s)» —es lo único que se cuenta—, la vuelta a casa no se
presenta jamás como un préstamo (es su cierre), y cuando el número se repite la
línea dice por qué. El texto es contrato y tiene test propio sobre una corrida
ida-vuelta-ida (`trace_test.go`), no solo sobre el estado interno:

```
p1 (0s)     ──[…]──→ handoff a p2 (préstamo 1/4)
p2 (2h 00m) ──[return_check: p1 ya liberó su ventana]──→ volviendo a p1 (vuelta a casa, no gasta préstamo: siguen 1/4)
p1 (10m)    ──[…]──→ handoff a p2 (préstamo 2/4)
```

### Encadenado de handoffs

**C-1 · No se creó ningún flag `--chain`, ni se tocó `HandoffForward`.** El
diseño hablaba de «`HandoffForward` acepta `a.To == from && a.Session ==
sessionUUID` con flag `--chain` (solo desde supervisor)». Lo que existe es una
función **aparte**, `core.HandoffChain` en `internal/core/handoff_chain.go`, que
el CLI manual no expone. Razón: un flag en el comando manual es un flag que
alguien va a usar a mano, y la invariante no-chain existe justamente porque
encadenar con un humano al mando casi siempre es un error de operación.

**C-2 · El encadenado NO crea un marcador nuevo: muta el que hay.** Si la sesión
ya está prestada y el marcador apunta a `from`, `HandoffChain` actualiza ese
marcador en sitio (`To = to`, `Hops += to`) y **conserva `From` y `Since`**. La
cadena `personal-cc → app-cc → kimi` sigue siendo **un solo marcador**
`personal-cc → kimi` cuyo `From` es el primario. Consecuencia: `ccp handoff end`
devuelve la conversación a casa **en un paso**, sin deshacer N niveles, que es
justo lo que hace falta a las 3am; y `Hops` conserva el rastro completo para que
`handoff list` no pierda por dónde pasó. El **fan-out sigue prohibido**: si la
sesión está prestada a un tercero, `HandoffChain` falla en vez de duplicarla.
`Auto` solo se enciende, nunca se apaga.

**C-3 · Los marcadores no llevan campo `parked`.** El diseño decía «marker
parked» para EDGE-5. Lo que hay es `Result.Parked` en memoria + exit 75
(`supervisor.ParkedExitCode`, EX_TEMPFAIL) + la tabla de cooldowns en la traza;
el marcador se deja **vivo tal cual**. Un estado `parked` en disco habría que
limpiarlo, y nadie lo hace: la sesión aparcada es indistinguible de una prestada,
que es exactamente lo que es.

### Terminal y proceso hijo

**T-1 · No existe `internal/supervisor/tty.go`, y no se usa `TIOCSPGRP`.** Todo
el manejo de proceso vive en `launcher.go`. El hijo corre en el **mismo process
group** que el supervisor: no se llama a `Setpgid` ni se le cede la terminal. Ser
un shell de verdad (ceder y recuperar el grupo de primer plano) obliga a manejar
`SIGTTOU` al ceder, `SIGTSTP`, y las reanudaciones — tres modos de fallo nuevos a
cambio de nada, porque compartiendo grupo el hijo YA es el proceso de primer
plano y hereda los fds sin ceremonia.

Lo que sí se hace: (a) el supervisor **ignora `SIGINT`** mientras el hijo vive
(`signal.Notify` sin lector), para que el `Ctrl-C` del teclado —que llega a los
dos por ser del mismo grupo— mate la sesión de claude y deje vivo al supervisor
para reportar el 130 y dejar el marcador coherente; y (b) **guarda y restaura el
`termios`** de stdin, porque matar a claude en mitad de su TUI (que es
literalmente lo que hace una rotación) dejaría el modo raw puesto y el usuario
recuperaría una shell sin eco. El ioctl se resuelve en runtime por `GOOS`/`GOARCH`
(`termiosReqs`) porque `x/sys/unix` no expone un nombre portable y el paquete no
se puede partir con build tags. Consecuencia: en una plataforma fuera de las
cuatro que ccp publica no se restaura el termios — se pierde una comodidad, no
una funcionalidad.

**T-2 · `Terminate` señala al PID exacto, nunca a `-PID`.** Corolario directo de
T-1: compartir process group significa que un envío al grupo se mataría a sí
mismo (y potencialmente a la shell del usuario).

### El gate `allow_from`

**G-1 · La regla real tiene TRES estados, no dos.** El diseño decía «explícito,
default REFUSE». Lo implementado (`core.ResolveAutoChain`, documentado en su
comentario) es:

| `allow_from` | Efecto |
|---|---|
| **ausente o vacío** | **no hay gate**: pasa todo el `fallback` |
| **declarado, con entrada para el primario** | pasan solo los de esa entrada; el resto va a `Denied` |
| **declarado, sin entrada para el primario** | **deny total**: `Fallback` vacío, TODOS los candidatos en `Denied` |

El primer estado no estaba en el diseño y es lo que evita fricción sin valor:
quien no separa clientes no debería tener que declarar el mapa entero solo para
permitir todo. El tercero es el «default deny» de la spec, pero enunciado con
precisión: **declarar el mapa es declarar la intención de gobernar los
préstamos**, así que un perfil que se te olvidó añadir se queda quieto en vez de
heredar barra libre. `--dry-run` y `ccp auto status` imprimen `Denied` siempre,
para que «no rota» nunca sea silencioso.

**G-2 · `AutoInit` siembra `allow_from` solo con perfiles OFICIALES.** Cada
perfil recibe `[sí mismo] + el resto de oficiales`; los de provider
(deepseek/kimi/glm) quedan fuera y el usuario los añade a mano. Sembrar el gate
permisivo lo convertiría en decoración, y el destino que más importa vetar por
defecto es justamente el que manda contexto a una API de terceros.

### Sensores y detección

**S-1 · Son CUATRO sensores, y son TRES por modo.** El diseño listaba «detección
en 3 capas» (statusLine / StopFailure / stream-json) y ponía el tail de
transcript solo en la tabla de señales. En el código el transcript es un sensor
de primera clase que corre en **los dos** modos, y el reparto real
(`supervisor.go:launchAndWatch`) es:

- **headless**: stream-json + transcript + sentinel.
- **interactive**: usage (statusLine) + transcript + sentinel.

El usage watcher **no** corre en headless (sin TTY no hay barra de estado que
alimente la muestra) y el stream-json **no** existe en interactive (la salida se
la queda la TUI). Consecuencia: el único sensor **proactivo** es el que no
funciona headless, así que el modo desatendido —el que más se beneficiaría de
avisar antes de fallar— es puramente reactivo. Lo compensa que el `api_retry`
del stream-json es la señal más limpia de todas.

**S-2 · Los sentinels NO viven en `/tmp/ccp-limit-<sid>`.** Viven en
`<CCP_HOME>/state/auto/sentinels/*.json` (`core.AutoStateDir`). `/tmp` es
world-writable y se limpia por su cuenta: un sentinel es una orden de rotar de
perfil, y aceptar órdenes de un archivo que cualquiera puede escribir no es
aceptable. Además el perfil y la sesión pasan por un filtro `[a-zA-Z0-9._-]`
antes de convertirse en nombre de archivo — los valores vienen del payload de un
hook, es decir de fuera.

**S-3 · `_limit-hook` detecta en dos pasadas, y no siempre escribe.** Primero
marca el payload como error y lo pasa por `core.ParseTranscriptLine`; si eso no
ve nada, cae a un filtro de prosa propio (`autoLooksLikeLimitText` +
`ClassifyLimitText`), porque la forma más probable del hook es un `"error":
"You've hit your weekly limit"` suelto que el parser interpreta como *clase* de
error y descarta. Y **no escribe sentinel si el payload no trae `session_id` ni
`transcript_path`**: el watcher filtra por sesión, así que un sentinel sin sesión
es inconsumible y solo dejaría basura permanente.

**S-4 · El supervisor deduplica eventos por CONTENIDO durante toda la corrida**
(`runner.seen`, clave = ventana + `resets_at` + detalle). No estaba en el diseño
y no es una optimización: `HandoffChain` **copia el `.jsonl` al cc-home del
destino**, y el `NewTranscriptWatcher` del siguiente lanzamiento lo lee desde el
byte 0 — incluida la línea del rate limit que provocó el salto anterior. Sin el
filtro, esa línea vieja quemaría la cadena entera en segundos sin gastar un
token. El precio: si dos perfiles topan su límite con un mensaje byte-idéntico,
el segundo se ignora (improbable, y el hijo muere igual con código no-cero, que
también rota).

**S-5 · `min_dwell` espera con el hijo VIVO.** Al detectar un límite antes de
cumplir la permanencia mínima, el supervisor **no** mata a claude y espera
apagado: lo deja trabajando y re-comprueba cada `poll`. Puede que termine el
turno en curso, y si no, al menos el usuario ve progreso en vez de una terminal
congelada.

**S-6 · Hay una ventana de drenaje de 150ms (`exitDrain`) que el diseño no
contemplaba.** `claude -p` imprime la línea `api_retry` y muere acto seguido; el
evento y el `Wait` quedan listos casi a la vez y `select` elige al azar entre
canales listos. Sin esa espera, la mitad de las veces se propagaría el exit code
en vez de rotar.

**S-7 · Los dos primeros desenlaces se deciden por el CÓDIGO, no por quién mató
al hijo.** `exit 0` termina (y cierra el préstamo si lo hay) y `exit 130`
(Ctrl-C) termina **sin rotar** dejando el marcador vivo y diciéndolo, aunque un
sensor haya reportado un límite en el mismo instante. Rotar tras un Ctrl-C sería
exactamente lo contrario de lo que el usuario pidió.

### Superficie del CLI

**CLI-1 · Flags de `ccp session` distintos de los de la fase 1.** Es
`--session <uuid>` (no `--session-id`), y se añadieron `--dry-run`,
`--no-return`, `--claude-bin <path>` (interno, documentado en la ayuda a
propósito: un flag que existe pero no se documenta es una trampa para quien lee
un stack trace) y `-h/--help`. `--yolo` es alias de
`--dangerously-skip-permissions` y **nunca se persiste**. `--` es una frontera
absoluta y un posicional suelto antes de `--` es un **error**, no algo que se
reenvíe en silencio: `ccp session "borra todo"` y `ccp session --yolo` se leerían
igual de bien y solo uno haría lo que parece.

**CLI-2 · `ccp auto install` toma perfiles, no `--include-default`.** La firma es
`ccp auto install [<perfil>…]` (sin args = todos los no-`default`) y **rechaza
`default` explícitamente**: `~/.claude` no lo regenera ccp, así que ahí la capa
no es gestionable ni reversible. Se añadieron además `ccp auto test` y los dos
internos `_statusline` / `_limit-hook`, que el diseño daba por implícitos.

**CLI-3 · `ccp handoff prune` usa `--keep N` con default 50**, y `keep == 0`
borra todo el historial. Un `prune` que no cambia nada **no reescribe el
archivo** (`ErrHandoffsUnchanged`): no debe tocar el mtime ni arriesgar una
escritura por nada. `prune` y `sessions` necesitaron además un guardia que el
diseño no anticipaba: la función shell reenvía todo lo que no reconoce como
solo-lectura, así que un rc instalado **antes** de que existieran convierte
`ccp handoff prune` en «préstale la sesión al perfil llamado prune». El binario
lo detecta y dice que refresque el rc, en vez de dejar que core conteste «perfil
inexistente: prune».

### Capa gestionada de `settings.json`

**L-1 · El statusLine del usuario se ENVUELVE, no se reemplaza.** La capa escribe
`<ccp> _statusline -- <el statusLine que hubiera>`, y
`ExtractStatusLineCommand` devuelve `""` cuando el que hay ya es el nuestro —
sin eso, cada regeneración añadiría una capa (`ccp _statusline -- ccp _statusline
-- …`) hasta un comando absurdo que además muestrearía N veces por refresco. El
`--` separa lo nuestro de lo suyo por si el comando ajeno empieza por `-`.

**L-2 · Se añadió `keepForeignStopFailure`, que el diseño no pedía.**
`MergeJSON` **reemplaza** arrays (semántica de `jq . * $x`), así que instalar la
capa habría borrado del `settings.json` generado cualquier `hooks.StopFailure`
propio del usuario. Se reinsertan detrás del nuestro, filtrando por
`_limit-hook` para no duplicarlo. El overlay nunca se toca, así que el daño era
recuperable — pero un hook que deja de dispararse sin decir nada es la clase de
fallo que nadie diagnostica.

**L-3 · El perfil NO viaja en la línea de comandos de los hooks.**
`AutoHooksFragment` recibe `profile` pero solo lo usa para validar y para nombrar
errores: el perfil activo llega por `CCP_PROFILE`, que el delta de entorno ya
exporta y los hooks de CC heredan. Duplicarlo crearía una segunda verdad que se
puede desincronizar con la primera.

**L-4 · La ruta del binario es estado de paquete inyectado desde el CLI**
(`core.SetAutoHooksBin` / `AutoHooksBin`, fijado en un `func init()` de
`internal/cli/auto.go` con `os.Executable`), no un parámetro de
`CfgRegenerate`. Añadir el argumento habría obligado a cada llamador (`profile
add`, `profile config`, `profile sync`, el TUI) a resolver el binario por su
cuenta, y entonces `auto install` y `profile sync` escribirían valores distintos
y el `settings.json` bailaría entre los dos en cada regeneración. El default es
el nombre pelado `ccp` (resuelto por PATH), no una ruta inventada.
*Consecuencia*: se escribe la ruta **absoluta**, así que mover el binario sin
`ccp upgrade` / `ccp auto install` / `ccp profile sync` deja los sensores
apuntando al sitio viejo.

**L-5 · `applyAutoLayer` nunca falla.** Está en el camino de `profile add`,
`profile config` y `profile sync`: que regenerar un cc-home fallara porque
`ccp.yaml` está a medio escribir cambiaría un problema menor (sensores ausentes
esta vez) por uno grave (perfil sin `settings.json`). Igual, `AutoHooksEnabled`
mira **solo** `auto_handoff.hooks` y NO `enabled`: medir el consumo y autorizar
rotaciones son decisiones separadas.

### Esquemas y compatibilidad

**V-1 · No se subió ninguna versión de esquema.** `ccp.yaml` sigue en `version:
2` con `auto_handoff` como bloque aditivo (un binario viejo lo preserva vía
`Config.Extra`), y `handoffs.yaml` sigue en `HandoffsVersion = 2` con `auto:` y
`hops:` como campos `omitempty`. Subir la versión haría que el binario viejo se
negara a leer el archivo — un precio absurdo por una feature que él no necesita
entender.

**V-2 · Se tocaron `shellinit.go`, el oráculo bash y el golden.** El diseño de
la fase 1 decía «sin cambios a: shellinit.go, bash oracle, golden gate, parity
test». `session` y `auto` sí están en las listas `top` de las dos completions
(`internal/core/shellinit.go`, y las mismas dos en `legacy/bin/ccp`), con el
golden regenerado.

La promesa de verdad detrás de aquella frase —«no hace falta reinstalar el rc»—
se cumple **solo para `session` y `auto`**: no son shell-only y llegan al binario
por el `*) command ccp "$@"` que ya existía. El **bloque de la función shell sí
cambió**, pero por otra cosa: la lista de subcomandos de `handoff` que se
reenvían tal cual (`status|list|discard`) tuvo que crecer a
`status|list|discard|prune|sessions`, porque `handoff` sí lo intercepta la
función. Consecuencia: `ccp handoff prune` y `ccp handoff sessions` **necesitan
un rc refrescado** (`ccp install`); ver CLI-3, que es el guardia que lo explica
en vez de dejar que falle como «perfil inexistente: prune».
