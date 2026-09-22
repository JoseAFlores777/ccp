# 17. La cadena de rotación es de cada perfil, no de la política

Fecha: 2026-09-22

## Estado

Aceptada. Aditiva sobre el bloque `auto_handoff` que describe el README
(«Configuring it»): `version` del esquema sigue en **2**. No toca el gate
`allow_from` ni la máquina de estados de `internal/supervisor`.

## Contexto

Hasta aquí la cadena de préstamos era **una sola**: `policies.<n>.fallback`. La
política se elige con `--policy` y por defecto es `default`, así que en la
práctica todos los perfiles rotaban por la misma lista, en el mismo orden.

Eso no es lo que la gente tiene. Quien usa ccp tiene cuentas de sitios distintos
y la compatibilidad entre ellas es **suya**, no derivable de nada que ccp pueda
mirar: una cuenta de trabajo puede prestarle a su gemela del mismo sitio y a
nadie más; la personal le presta a su proveedor; la de un cliente no le presta a
nadie. Son tres cadenas distintas.

Existía ya un mecanismo por perfil, `allow_from`, y no sirve para esto:

- **Solo resta.** Filtra la lista compartida, así que esa lista es el TECHO de
  todos. Un destino que no esté en ella no se le puede dar a nadie.
- **No ordena.** El orden ES la preferencia (`Next()` recorre la cadena de
  arriba abajo) y el gate no lo toca: dos perfiles no pueden preferir destinos
  distintos.
- **Es un gate de cumplimiento.** Está para negar préstamos que no deben
  ocurrir, no para expresar preferencias. Recargarlo con la segunda función
  habría mezclado «no quiero» con «no debe», que es justo la distinción que
  `allow_from` existe para mantener.

La otra salida sin tocar el esquema era «una política por perfil», que ya se
puede escribir: N políticas y `--policy` en cada invocación. Se descartó porque
el perfil lo decide la **carpeta** (`ccp resolve $PWD`) y la política la decide
una **bandera**: nada las ata, así que `ccp session` en el repo del cliente
usaría la política del último comando que alguien escribió. Una cadena que
depende de acordarse de una bandera no es una cadena por perfil.

## Decisión

Una clave nueva, `auto_handoff.chains`, con la cadena propia de cada perfil.

```yaml
auto_handoff:
  policies:
    default:
      fallback: [personal-cc, personal-deepseek]   # la lista COMPARTIDA
  chains:
    a-cc: [a-cc-2]              # forma corta: su cadena y ya
    e-cc: []                    # declarada y vacía: no presta a nadie
    personal-cc:                # forma larga: cadena + política propia
      fallback: [personal-deepseek]
      policy: lenta
```

El reparto de responsabilidades queda así, y es el que no conviene borrar:

| clave | responde a | alcance |
|---|---|---|
| `chains[<perfil>]` | a quién le presta, **en orden** | por perfil |
| `policies[<n>]` | cuándo y cuánto (umbral, permanencia, hops, cooldown) | compartida |
| `allow_from[<p>]` | si ese préstamo **debe** ocurrir | por perfil, se aplica después |

Cuatro decisiones dentro de esa, cada una con su porqué:

**1. Herencia, no declaración obligatoria.** Un perfil sin entrada usa el
`fallback` de su política, que es exactamente lo que hace hoy. Así una
configuración que ya funcionaba sigue funcionando sin migrar nada, y un perfil
nuevo arranca rotando en vez de arrancar mudo. Obligar a declararlo todo habría
cambiado el significado de los `ccp.yaml` existentes, que es el precio que no
toca pagar por una feature aditiva.

**2. Tres estados, no dos** — la misma lección que `allow_from` ya había
aprendido. Entrada **ausente** es «hereda»; entrada **declarada y vacía** es
«este perfil no presta a nadie». Lo segundo es una decisión y hay que poder
escribirla: si `[]` se leyera como ausente, apagar la rotación de un solo perfil
no se podría expresar y habría que apagarla entera. Por eso `AutoChain` guarda
`hasFallback` y no deduce el estado de `len(Fallback) == 0`, y por eso al
serializar se elige entre dos structs: con `omitempty` una cadena vacía se
escribiría sin la clave y al releerla la rotación volvería sola.

**3. La cadena SUSTITUYE, no se suma.** Si se sumara, un perfil no podría
quitarse un destino que la lista compartida trae — o sea, seguiría sin poder
expresar la mitad del problema.

**4. Las mutaciones apuntan por defecto a la cadena del primario.** `ccp auto
chain add x` dentro de un repo edita `chains[<primario>]`, no la lista
compartida. Es un cambio de comportamiento de un comando que ya existía, y se
elige porque es lo que significa la frase que el usuario escribe: dentro de un
repo, «añade x a la cadena» habla de ese perfil. La lista compartida sigue
alcanzable con `--shared` (o nombrando `--policy`, que apunta a la suya), y otro
perfil con `--for <perfil>`. `--for` junto a `--shared`/`--policy` es un error,
no un desempate: son dos destinos y escribir en cualquiera de los dos
sorprendería.

## Consecuencias

**La bifurcación es visible o no existe.** La primera mutación sobre un perfil
que hereda crea su cadena propia **sembrada con la heredada** (añadir uno no
puede significar quitar todos los demás) y a partir de ahí ese perfil deja de
recibir los cambios de la lista compartida. Es una consecuencia que nadie pidió
y que no se nota hasta meses después, cuando alguien añade un destino a la
compartida y a ese repo no le llega — así que se reporta en el momento
(`ChainResult.Forked`, línea `[warn]` en el CLI, aviso previo en la GUI) y
`ccp auto chain reset` la deshace.

**Toda superficie que escribía apuntaba a la lista compartida y había que
moverla.** La TUI (`chainOpts`) y la GUI (`chainSnapshot`, `chainAddModal`, el
lienzo, el arreglo de un clic del diagnóstico) pasaban `policy`, que en el motor
significa «la lista compartida»: sin tocarlas, la app le habría cambiado la
cadena a todos los perfiles mientras enseñaba la de uno. El diagnóstico además
solo revisaba las listas compartidas, o sea justo las que un perfil con cadena
propia ya no usa: daba verde sobre la configuración que nadie lee.

**`ccp auto status` tiene que decir de dónde sale la cadena.** Sin eso,
`fallback: []` no distingue «este perfil no presta a nadie» de «la lista
compartida está vacía», y cada una se arregla en un sitio distinto del yaml. De
ahí `chain_own` y `policy_pinned` en el JSON, y la línea `origen` en las tres
interfaces, con las mismas palabras en las tres.

**Un ccp anterior preserva la clave pero no la entiende.** `AutoHandoff.Extra`
la captura y la reescribe intacta —para eso se puso ese catch-all—, así que no
hay pérdida de datos; pero ese binario rotará por la lista compartida. Es
degradación de comportamiento, visible en `ccp auto chain show`, no corrupción.

**Lo que sigue pendiente.** `chains` decide destinos y orden; los knobs siguen
siendo de la política, y ligar una (`chains[<p>].policy`) es reusar `policies`,
no una tercera capa. Un perfil con API key que quiera `cooldown: fixed` necesita
por tanto una política con nombre, no un ajuste suelto en su entrada. Se deja
así a propósito: una tercera capa de parámetros obligaría a decidir quién gana
entre ella, la política y `--policy`, y eso es un empate que hoy no hace falta
romper.
