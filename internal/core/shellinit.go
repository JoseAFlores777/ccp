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
  local top="install uninstall upgrade key path profile instruct status config doctor menu completion resolve lang version help use default on off run"
  if [[ $COMP_CWORD -eq 1 ]]; then COMPREPLY=( $(compgen -W "$top" -- "$cur") ); return; fi
  case "${COMP_WORDS[1]}" in
    profile) [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "add rm list show login config sync" -- "$cur") )
             [[ $COMP_CWORD -eq 3 && "${COMP_WORDS[2]}" =~ ^(rm|show|login|config|sync)$ ]] && COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") )
             [[ $COMP_CWORD -eq 4 && "${COMP_WORDS[2]}" == "config" ]] && COMPREPLY=( $(compgen -W "instructions settings" -- "$cur") ) ;;
    path)    [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "set rm list test clear edit" -- "$cur") )
             [[ $COMP_CWORD -eq 3 && "${COMP_WORDS[2]}" =~ ^(set|rm|test)$ ]] && COMPREPLY=( $(compgen -d -- "$cur") )
             [[ $COMP_CWORD -eq 4 && "${COMP_WORDS[2]}" == "set" ]] && COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    use)     COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    key)     COMPREPLY=( $(compgen -W "$(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    completion) COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") ) ;;
  esac
}
complete -F _ccp ccp
`

// CompletionZsh es el bloque verbatim del heredoc COMPLETION_ZSH en cmd_completion().
const CompletionZsh = `if ! whence compdef >/dev/null 2>&1; then autoload -Uz compinit && compinit -C; fi
_ccp() {
  local -a top; top=(install uninstall upgrade key path profile instruct status config doctor menu completion resolve lang version help use default on off run)
  if (( CURRENT == 2 )); then compadd -- $top; return; fi
  case "${words[2]}" in
    profile) (( CURRENT == 3 )) && compadd -- add rm list show login config sync
             (( CURRENT == 4 )) && [[ "${words[3]}" =~ ^(rm|show|login|config|sync)$ ]] && compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"}
             (( CURRENT == 5 )) && [[ "${words[3]}" == config ]] && compadd -- instructions settings ;;
    path)    (( CURRENT == 3 )) && compadd -- set rm list test clear edit
             (( CURRENT == 3 )) || { [[ "${words[3]}" =~ ^(set|rm|test)$ ]] && _path_files -/ }
             (( CURRENT == 4 )) && [[ "${words[3]}" == set ]] && compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    use)     compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"} ;;
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
