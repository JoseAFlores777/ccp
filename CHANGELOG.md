# Changelog

## [2.1.0]
- Autocompletado de shell (`dsctl completion bash|zsh`): subcomandos, llaves de
  config, rutas y verbos de `ds`. Se auto-carga vía el shell init.
- Cache en el hook `_ds_autocheck`: evita el fork de `dsctl _resolve` cuando el
  `$PWD` no cambió desde el último prompt.
- Salida machine-readable: `dsctl status --json`, exit codes en `dsctl path test`
  (0=deepseek, 1=official) y comando público `dsctl resolve`.
- `doctor` reporta si el autocompletado está cargado.
- Fix: `path list` no mostraba reglas en macOS (BSD grep no soporta `grep -P`).
  Reemplazado por filtrado con `read` (portable). El ruteo nunca estuvo afectado.

## [2.0.0]
- Gestión de paths con reglas `include` / `exclude` y precedencia por especificidad.
- Motor de resolución aislado en `lib/paths.sh` (testeable).
- Hook por carpeta basado en `dsctl _resolve "$PWD"`.
- Submenú interactivo de paths; `path list/test/edit/clear`.
- Soporte bash y zsh.

## [1.0.0]
- Versión inicial: función `ds on/off/run`, lista plana de auto-repos,
  key segura, config, doctor, menú.
