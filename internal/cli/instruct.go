package cli

// instruct.go — despacho de `ccp instruct add|list|rm|dest|record`, respaldo de
// /ccp:remember-* , /ccp:recall , /ccp:forget. Resuelve el contexto (perfil
// activo, fuente global, raíz del repo) y delega en internal/core, formateando
// la salida y los exit codes en español. Espeja cmd_instruct del bash (bin/ccp).

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// claudeSrc resuelve la fuente global (~/.claude o CCP_CLAUDE_SRC), igual que el
// core. No falla si HOME es resoluble.
func claudeSrc() (string, error) {
	if src := os.Getenv("CCP_CLAUDE_SRC"); src != "" {
		return src, nil
	}
	hd, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo determinar HOME: %w", err)
	}
	return hd + "/.claude", nil
}

// repoRoot replica _instr_repo_root del bash: CCP_REPO_ROOT (override de tests) ->
// `git rev-parse --show-toplevel` -> cwd como fallback. Nunca vacío salvo que
// cwd no se pueda determinar.
func repoRoot() string {
	if r := os.Getenv("CCP_REPO_ROOT"); r != "" {
		return r
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if r := strings.TrimSpace(string(out)); r != "" {
			return r
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

// instructCtx arma el InstructCtx desde el entorno (perfil activo via
// CCP_PROFILE; default si ausente).
func instructCtx() (core.InstructCtx, error) {
	home, err := ccpHome()
	if err != nil {
		return core.InstructCtx{}, err
	}
	src, err := claudeSrc()
	if err != nil {
		return core.InstructCtx{}, err
	}
	active := os.Getenv("CCP_PROFILE")
	if active == "" {
		active = "default"
	}
	return core.InstructCtx{
		Home:          home,
		Src:           src,
		RepoRoot:      repoRoot(),
		ActiveProfile: active,
	}, nil
}

// emitDestError imprime un DestError con el idioma del bash (err + info hint) y
// devuelve su exit code estable (1..5). Otros errores -> "Error: ..." rc 1.
func emitErr(stderr io.Writer, err error) int {
	var de *core.DestError
	if errors.As(err, &de) {
		fmt.Fprintf(stderr, "Error: %s\n", de.Msg)
		if de.Hint != "" {
			fmt.Fprintf(stderr, "  %s\n", de.Hint)
		}
		return de.Code
	}
	fmt.Fprintf(stderr, "Error: %v\n", err)
	return 1
}

func dispatchInstruct(args []string, stdout, stderr io.Writer) int {
	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "add":
		return instructAdd(args[1:], stdout, stderr)
	case "list":
		return instructList(args[1:], stdout, stderr)
	case "rm":
		return instructRm(args[1:], stdout, stderr)
	case "dest":
		return instructDest(args[1:], stdout, stderr)
	case "record":
		return instructRecord(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.instruct.unknown_sub", sub))
		return 1
	}
}

// ccp instruct add <scope> <type> <texto...>
func instructAdd(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	if len(args) < 3 {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.instruct.usage_add"))
		return 1
	}
	scope, typ := args[0], args[1]
	text := strings.Join(args[2:], " ")
	ctx, err := instructCtx()
	if err != nil {
		return emitErr(stderr, err)
	}
	res, err := core.InstructAdd(ctx, scope, typ, text)
	if err != nil {
		return emitErr(stderr, err)
	}
	switch res.Type {
	case "rule":
		if res.Duplicate {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.rule_dup", res.Dest))
		} else {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.rule_added", res.Scope, res.Dest))
		}
	case "mcp":
		fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.mcp_added", res.Name, res.Scope, res.Dest))
	case "hook":
		fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.hook_added", res.Name, res.Scope, res.Dest))
		fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.hook_note"))
		if res.Scope == "profile" {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.hook_rm_profile", ctx.ActiveProfile))
		} else {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.hook_rm_other", res.Dest))
		}
	}
	return 0
}

// ccp instruct list <scope>
func instructList(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	if len(args) < 1 {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.instruct.usage_list"))
		return 1
	}
	scope := args[0]
	ctx, err := instructCtx()
	if err != nil {
		return emitErr(stderr, err)
	}
	rows, err := core.InstructList(ctx, scope)
	if err != nil {
		return emitErr(stderr, err)
	}
	if len(rows) == 0 {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.list_empty", scope))
		return 0
	}
	for _, r := range rows {
		fmt.Fprintf(stdout, "   %2d) [%s] %s\n", r.Index, r.Type, r.Text)
	}
	return 0
}

// ccp instruct rm <scope> <index>
func instructRm(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	if len(args) < 2 {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.instruct.usage_rm"))
		return 1
	}
	scope := args[0]
	idx, err := strconv.Atoi(args[1])
	if err != nil || idx < 1 {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.instruct.usage_rm"))
		return 1
	}
	ctx, err := instructCtx()
	if err != nil {
		return emitErr(stderr, err)
	}
	res, err := core.InstructRm(ctx, scope, idx)
	if err != nil {
		return emitErr(stderr, err)
	}
	if res.WasRule {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.rm_rule", idx, scope))
		return 0
	}
	if res.Type == "hook" {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.rm_hook_note", res.Ref))
	}
	fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.rm_artifact", idx, scope, res.Type, res.Ref))
	return 0
}

// ccp instruct dest <scope> <type>  (imprime la ruta destino; lo usan /ccp:*)
func instructDest(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.instruct.usage_dest"))
		return 1
	}
	ctx, err := instructCtx()
	if err != nil {
		return emitErr(stderr, err)
	}
	dest, err := core.InstructDestCmd(ctx, args[0], args[1])
	if err != nil {
		return emitErr(stderr, err)
	}
	fmt.Fprintln(stdout, dest)
	return 0
}

// ccp instruct record <scope> <type> <ref> <desc...>
func instructRecord(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	if len(args) < 3 {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.instruct.usage_record"))
		return 1
	}
	scope, typ, ref := args[0], args[1], args[2]
	desc := strings.Join(args[3:], " ")
	ctx, err := instructCtx()
	if err != nil {
		return emitErr(stderr, err)
	}
	if err := core.InstructRecord(ctx, scope, typ, ref, desc); err != nil {
		return emitErr(stderr, err)
	}
	fmt.Fprintln(stdout, i18n.T(lang, "cli.instruct.recorded", scope, typ, ref))
	return 0
}
