# 6. Reescritura a Go conservando la cola shell bash emitida por el binario

Fecha: 2026-06-02

## Estado

Aceptada

## Contexto

`ccp` es hoy un binario Bash (~2.3k líneas: `bin/ccp` + 5 libs) más una **cola shell** (la función `ccp` y el hook `_ccp_autocheck`) que `ccp install` inyecta en el rc del usuario. El binario corre en un proceso hijo, por lo que **no puede mutar el entorno del shell padre**: la única vía de mutación es que el binario *emita* texto `unset…/export…` y que la cola shell lo haga `eval` (`eval "$(command ccp _env <perfil>)"`, y por PWD `eval "$(command ccp _hook "$PWD")"`).

Queremos migrar a Go por mantenibilidad, tipos, tests y para habilitar una TUI rica (bubbletea/huh) y manejo de YAML. La pregunta de fondo: ¿qué parte puede volverse Go?

Alternativas consideradas:

1. **Solo el binario a Go; la cola shell sigue siendo bash, ahora emitida por el binario Go.** El contrato observable (`_env`, `_hook`, `completion`, `completion-shellinit`) se conserva byte-a-byte.
2. **Todo Go con un daemon residente** que mute el entorno vía socket, eliminando el re-`eval` por prompt.
3. **Híbrido incremental (strangler)**: subcomandos migran uno a uno mientras bash sigue despachando el resto.

## Decisión

Tomamos la alternativa **1**. Go reescribe el binario y las libs como un paquete `core` más front-ends; la función shell y el hook **siguen siendo texto bash**, porque corren *dentro* de zsh/bash del usuario y son lo único capaz de `export`/`unset`. Ese texto pasa a ser **emitido por el binario Go** (`ccp completion-shellinit`), idéntico al actual.

El daemon (alternativa 2) rompe el modelo "perfil por PWD vía hook", introduce un proceso residente y un canal IPC, y no aporta nada que el `eval` por prompt no resuelva ya. El híbrido (alternativa 3) es inviable: el dispatch es un único `case` y dos binarios conviviendo en un mismo PATH es frágil.

## Consecuencias

- La frontera binario↔shell **no cambia**: el binario EMITE entorno, el shell lo EVALúa. Esto queda como invariante permanente, independiente del lenguaje del binario.
- Un rc ya instalado (bloque entre `# >>> ccp shell init >>>` / `# <<<`) **sigue funcionando sin reinstalar**, porque solo llama `command ccp …`; basta que Go implemente esos subcomandos con salida equivalente.
- Go debe reproducir el quoting de bash `%q` de forma que zsh/bash evalúen igual cualquier valor (ver [ADR 0007] y el gate golden-diff). Esto es la fuente principal de riesgo de equivalencia.
- La TUI y todo el core viven en Go; la cola shell queda mínima y estable.
