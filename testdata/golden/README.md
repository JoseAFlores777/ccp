# Golden fixtures — oráculo bash

Arnés de **golden-diff** del rewrite Go (plan §9, Fase 0). El binario bash
(`bin/ccp`) es el **oráculo** del contrato hasta lograr paridad; estos fixtures
congelan su salida para que las slices Go siguientes diff-een contra ella.

## Layout

```
testdata/golden/
├── capture.sh            # regenera/verifica los expected desde el oráculo
└── basic/
    ├── ccp-home/         # fixture CCP_HOME (rutas-regla tokenizadas __ROOT__)
    │   ├── profiles.tsv
    │   ├── rules.tsv      # __ROOT__/... -> materializado a ruta real al correr
    │   └── profiles/{work(official),deepseek}/...
    └── expected/         # <caso>.out (stdout redactado) + <caso>.code (exit)
```

## Uso

```bash
bash testdata/golden/capture.sh           # regenera expected/ desde el oráculo
bash testdata/golden/capture.sh --check   # falla si el oráculo ya no reproduce
```

`capture.sh` copia el fixture a un `CCP_HOME` temporal, sustituye `__ROOT__`
por la ruta real, corre el oráculo y **redacta** esa ruta a `__CCP_HOME__`
para que el golden sea estable entre máquinas. Scrub de `CCP_PROFILE` para que
`status.active` sea determinista.

## Superficie capturada (contrato congelado)

`_env` (default/deepseek/official/inexistente) · `_hook` · `resolve` ·
`path test` (con exit codes 0=no-default / 1=default) · `status --json` ·
`completion bash|zsh` · `completion-shellinit`.

`version` **no** entra: el oráculo reporta su propia versión (v3.1.0), distinta
de la del binario Go (v2.0.0). La versión no es parte del contrato.

La equivalencia byte-a-byte Go↔oráculo (y el **efecto del eval** en zsh+bash) se
añade como gate en las Fases 2–4, cuando el core Go emite esta misma superficie.
