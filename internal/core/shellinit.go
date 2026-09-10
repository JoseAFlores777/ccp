package core

import "io"

// ShellInit es el bloque verbatim que _print_shell_init() emite en el bash oracle.
// Incluye los marcadores # >>> ccp shell init >>> y # <<< ccp shell init <<<.
// Las constantes son BYTE-IDÉNTICAS al output capturado del oracle; no modificar.
const ShellInit = `# >>> ccp shell init >>>
ccp() {
  case "$1" in
    use)
      shift; eval "$(command ccp _env "${1:-default}")" ;;
    default|off)
      eval "$(command ccp _env default)" ;;
    on)   # legacy alias
      eval "$(command ccp _env deepseek)" ;;
    run)
      shift
      eval "$(command ccp _hook "$PWD")"
      if [[ $# -gt 0 ]]; then command "$@"; else command claude; fi ;;
    handoff)
      shift
      case "$1" in
        end)          shift; out=$(command ccp _handoff-end "$PWD" "$@") || return ;;
        resume)       shift; out=$(command ccp _handoff-resume "$PWD" "$@") || return ;;
        status|list|discard|prune|sessions)  command ccp handoff "$@"; return ;;
        *)            out=$(command ccp _handoff "$PWD" "$@") || return ;;
      esac
      ( eval "$out" || exit
        if [[ -n "${CCP_RESUME_YOLO:-}" ]]; then
          claude --resume "$CCP_RESUME_ID" --dangerously-skip-permissions
        else
          claude --resume "$CCP_RESUME_ID"
        fi ) ;;
    *) command ccp "$@" ;;
  esac
}

# Hook: aplica el perfil del PWD al cambiar de carpeta (cache por PWD).
_ccp_autocheck() {
  command -v ccp >/dev/null 2>&1 || return
  [[ "$PWD" == "${_CCP_LAST_PWD:-}" ]] && return
  _CCP_LAST_PWD="$PWD"
  eval "$(command ccp _hook "$PWD" 2>/dev/null)"
}
if [[ -n "$ZSH_VERSION" ]]; then
  typeset -ag precmd_functions
  [[ " ${precmd_functions[*]} " == *" _ccp_autocheck "* ]] || precmd_functions+=(_ccp_autocheck)
elif [[ -n "$BASH_VERSION" ]]; then
  [[ "$PROMPT_COMMAND" == *_ccp_autocheck* ]] || PROMPT_COMMAND="_ccp_autocheck;${PROMPT_COMMAND:-}"
fi

if command -v ccp >/dev/null 2>&1; then
  if [[ -n "$ZSH_VERSION" ]]; then
    eval "$(ccp completion zsh)" 2>/dev/null
  elif [[ -n "$BASH_VERSION" ]]; then
    eval "$(ccp completion bash)" 2>/dev/null
  fi
fi
# <<< ccp shell init <<<
`

// CompletionBash es el bloque verbatim del heredoc COMPLETION_BASH en cmd_completion().
const CompletionBash = `_ccp() {
  local cur prev; cur="${COMP_WORDS[COMP_CWORD]}"; prev="${COMP_WORDS[COMP_CWORD-1]}"
  local top="install uninstall upgrade key path profile instruct status config doctor menu completion resolve lang version help use default on off run handoff session auto desktop"
  if [[ $COMP_CWORD -eq 1 ]]; then COMPREPLY=( $(compgen -W "$top" -- "$cur") ); return; fi
  case "${COMP_WORDS[1]}" in
    profile) [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "add rm rename list show login config sync" -- "$cur") )
             [[ $COMP_CWORD -eq 3 && "${COMP_WORDS[2]}" =~ ^(rm|rename|show|login|config|sync)$ ]] && COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") )
             [[ $COMP_CWORD -eq 4 && "${COMP_WORDS[2]}" == "config" ]] && COMPREPLY=( $(compgen -W "instructions settings" -- "$cur") ) ;;
    path)    [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "set rm list test clear edit" -- "$cur") )
             [[ $COMP_CWORD -eq 3 && "${COMP_WORDS[2]}" =~ ^(set|rm|test)$ ]] && COMPREPLY=( $(compgen -d -- "$cur") )
             [[ $COMP_CWORD -eq 4 && "${COMP_WORDS[2]}" == "set" ]] && COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    config)  [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "show set reset editor gui-editor edit" -- "$cur") )
             [[ $COMP_CWORD -ge 3 && "${COMP_WORDS[2]}" == "edit" ]] && COMPREPLY=( $(compgen -W "--editor --profile --terminal" -- "$cur") ) ;;
    use)     COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    handoff) [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "resume end discard status list prune sessions default $(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    auto)    [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "init install uninstall status test chain" -- "$cur") )
             [[ $COMP_CWORD -eq 3 && "${COMP_WORDS[2]}" == "chain" ]] && COMPREPLY=( $(compgen -W "show add rm mv set help --policy" -- "$cur") )
             [[ $COMP_CWORD -ge 4 && "${COMP_WORDS[2]}" == "chain" && "${COMP_WORDS[3]}" =~ ^(add|rm|mv|set)$ ]] && COMPREPLY=( $(compgen -W "--policy --at --no-allow $(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    session) COMPREPLY=( $(compgen -W "--dry-run --headless --policy --yolo --max-hops --no-return --setup --no-setup" -- "$cur") ) ;;
    desktop) [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "open app list path prepare rm default $(ccp profile list 2>/dev/null)" -- "$cur") )
             [[ $COMP_CWORD -eq 3 && "${COMP_WORDS[2]}" =~ ^(open|app|path|prepare|rm)$ ]] && COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    key)     COMPREPLY=( $(compgen -W "$(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    completion) COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") ) ;;
  esac
}
complete -F _ccp ccp
`

// CompletionZsh es el bloque verbatim del heredoc COMPLETION_ZSH en cmd_completion().
const CompletionZsh = `if ! whence compdef >/dev/null 2>&1; then autoload -Uz compinit && compinit -C; fi
_ccp() {
  local -a top; top=(install uninstall upgrade key path profile instruct status config doctor menu completion resolve lang version help use default on off run handoff session auto desktop)
  if (( CURRENT == 2 )); then compadd -- $top; return; fi
  case "${words[2]}" in
    profile) (( CURRENT == 3 )) && compadd -- add rm rename list show login config sync
             (( CURRENT == 4 )) && [[ "${words[3]}" =~ ^(rm|rename|show|login|config|sync)$ ]] && compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"}
             (( CURRENT == 5 )) && [[ "${words[3]}" == config ]] && compadd -- instructions settings ;;
    path)    (( CURRENT == 3 )) && compadd -- set rm list test clear edit
             (( CURRENT == 3 )) || { [[ "${words[3]}" =~ ^(set|rm|test)$ ]] && _path_files -/ }
             (( CURRENT == 4 )) && [[ "${words[3]}" == set ]] && compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    config)  (( CURRENT == 3 )) && compadd -- show set reset editor gui-editor edit
             (( CURRENT >= 4 )) && [[ "${words[3]}" == edit ]] && compadd -- --editor --profile --terminal ;;
    use)     compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    handoff) (( CURRENT == 3 )) && compadd -- resume end discard status list prune sessions default ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    auto)    (( CURRENT == 3 )) && compadd -- init install uninstall status test chain
             (( CURRENT == 4 )) && [[ "${words[3]}" == chain ]] && compadd -- show add rm mv set help --policy
             (( CURRENT >= 5 )) && [[ "${words[3]}" == chain && "${words[4]}" =~ ^(add|rm|mv|set)$ ]] && compadd -- --policy --at --no-allow ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    session) compadd -- --dry-run --headless --policy --yolo --max-hops --no-return --setup --no-setup ;;
    desktop) (( CURRENT == 3 )) && compadd -- open app list path prepare rm default ${(f)"$(ccp profile list 2>/dev/null)"}
             (( CURRENT == 4 )) && [[ "${words[3]}" =~ ^(open|app|path|prepare|rm)$ ]] && compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    key)     compadd -- ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    completion) compadd -- bash zsh ;;
  esac
}
compdef _ccp ccp
`

// WriteShellInit escribe ShellInit en w (sin newline adicional: el const ya termina en \n).
func WriteShellInit(w io.Writer) (int, error) {
	return io.WriteString(w, ShellInit)
}

// WriteCompletionBash escribe CompletionBash en w.
func WriteCompletionBash(w io.Writer) (int, error) {
	return io.WriteString(w, CompletionBash)
}

// WriteCompletionZsh escribe CompletionZsh en w.
func WriteCompletionZsh(w io.Writer) (int, error) {
	return io.WriteString(w, CompletionZsh)
}
