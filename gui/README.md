# ccp — app de escritorio

La interfaz gráfica de ccp: qué cuenta de Claude usa cada carpeta, cuánto uso le
queda a cada una, la rotación entre cuentas, las conversaciones y los préstamos,
las ventanas de Desktop y el diagnóstico. Todo lo que hace tiene su comando en la
CLI, y cada pantalla enseña cuál es.

Está hecha con [Tauri 2](https://tauri.app) (Rust) y React. El diseño de
referencia es `docs/ui-design/ccp Interfaz - standalone.html`.

## Cómo está montada

```
React (gui/src)  ──invoke──▶  Rust (gui/src-tauri)  ──stdin/stdout──▶  ccp serve --stdio
   pantallas                    puente + terminal                        el motor de siempre
```

- **Toda la lógica es la de ccp.** La app no lee ni escribe `ccp.yaml` por su
  cuenta: arranca `ccp serve --stdio` y le habla en JSON, una línea por petición
  (protocolo en `internal/cli/serve.go`). Lo que funciona en la CLI funciona aquí
  igual, y viceversa.
- **Rust solo hace de puente.** Elige qué `ccp` arrancar, le da un entorno limpio
  (sin el `CCP_PROFILE` ni el `ANTHROPIC_*` de la terminal desde la que se abrió)
  y el `PATH` de tu shell de login, y reparte las respuestas.
- **Lo que necesita una terminal, se abre en una terminal.** Un `/login`, un
  préstamo (`ccp handoff`) o una sesión supervisada (`ccp session`) no se pueden
  hacer desde fuera: `claude` es interactivo y el perfil de una terminal solo lo
  cambia la función shell. La app lo explica, enseña el comando y abre una ventana
  de Terminal ya situada en la carpeta. Solo acepta comandos `ccp`, y cada
  argumento se cita por separado.
- **Las keys solo se escriben.** El campo de API key no vuelve a enseñar la key,
  y la key viaja directa a `ccp`, que la guarda fuera de `ccp.yaml` con permisos 600.

### Qué `ccp` usa

1. `CCP_GUI_BIN`, si está definida.
2. `CCP_SANDBOX` (desarrollo): el binario y el HOME de pruebas de `npm run sandbox`.
3. El `ccp` instalado, si ya trae `ccp serve`.
4. El que viaja dentro de la app. En ese caso los sensores de la rotación siguen
   apuntando al instalado, que es el que seguirá ahí cuando la app se cierre.

Ajustes → «Acerca de esta app» dice cuál se está usando y por qué.

## Desarrollo

Requisitos: Node 20+, Go (el del repo) y Rust estable (`rustup`).

```bash
cd gui
npm install
npm run sandbox        # crea gui/.sandbox: un HOME y un CCP_HOME de pruebas con datos de ejemplo
npm run dev:sandbox    # la interfaz en el navegador, en http://localhost:1420, contra el sandbox
```

En el navegador no hay Rust: Vite hace de puente con el mismo protocolo, y lo que
solo existe en la app (abrir una terminal, los diálogos de archivo) se simula.
**Nada de esto toca tu `~/.config/ccp` ni tu `~/.claude`**: el sandbox tiene su
propio HOME.

La app de escritorio en modo desarrollo, con `npm run dev:sandbox` ya corriendo
en otra terminal (usa ese mismo servidor, así que los cambios se ven a la vez en
el navegador y en la app):

```bash
npm run app:sandbox      # la app nativa contra el sandbox
npm run tauri dev        # sin servidor previo y contra tu configuración real
```

En el sandbox, «Abrir en Terminal» abre tu Terminal de verdad pero con el HOME,
el CCP_HOME y el `ccp` del sandbox: el comando no toca tu configuración.

Comprobaciones:

```bash
npm run typecheck                                   # TypeScript
cd src-tauri && cargo test && cargo clippy --all-targets && cargo fmt --check
```

## Compilar la app

```bash
cd gui
npm run tauri build    # compila el ccp de este repo como sidecar, la interfaz y el .app/.dmg
```

Queda en `gui/src-tauri/target/release/bundle/` (`macos/ccp.app` y `dmg/`). El
build no está firmado: la primera vez macOS pide abrirlo con clic derecho → Abrir.

## Estructura

```
gui/
├── src/
│   ├── lib/          puente (bridge.ts), tipos del protocolo (api.ts), estado (store.tsx),
│   │                 operaciones compartidas (actions.ts), editores de configuración
│   │                 (config_edit.ts), textos de diagnóstico (findings.ts), i18n
│   ├── components/   marco (Shell), modal de confirmación, hoja de terminal, avisos, piezas de UI
│   └── screens/      una pantalla por archivo (P-01 … P-20). P-20 Configuración se reparte
│                     en tres: la pantalla, su tabla de MCP y su vista efectiva
├── src-tauri/        el puente en Rust (bridge.rs), terminal y Finder (native.rs), config de Tauri
└── scripts/          sandbox.sh (datos de prueba) y build-sidecar.sh (ccp para el .app)
```

Los textos se escriben en español dentro de `t('…')` y su traducción inglesa vive
en `src/lib/i18n_en.ts`. El idioma es el mismo de la CLI (`ccp lang`), y se
cambia desde la barra superior.
