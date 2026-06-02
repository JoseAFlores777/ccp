package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// install.go cablea el ciclo de vida del binario en el shell del usuario:
// `install`/`uninstall` editan el bloque rc (marcadores >>> ccp shell init >>>),
// `upgrade` re-ejecuta install.sh desde la fuente registrada y re-sincroniza
// perfiles. Espejan cmd_install / cmd_uninstall / cmd_upgrade del oráculo bash.

const (
	rcComment    = "# ccp — router de perfiles de Claude Code"
	rcMarkerOpen = "# >>> ccp shell init >>>"
	rcMarkerEnd  = "# <<< ccp shell init <<<"
)

// rcPath resuelve el archivo rc a tocar: el override explícito (arg posicional)
// gana, luego CCP_RC (tests), luego ~/.zshrc o ~/.bashrc según $SHELL.
func rcPath(override string) string {
	if override != "" {
		return override
	}
	if rc := os.Getenv("CCP_RC"); rc != "" {
		return rc
	}
	home, _ := os.UserHomeDir()
	shell := filepath.Base(os.Getenv("SHELL"))
	if shell == "zsh" {
		return filepath.Join(home, ".zshrc")
	}
	return filepath.Join(home, ".bashrc")
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), needle)
}

// cmdInstall añade el bloque shell-init al rc (idempotente). Espeja cmd_install.
func cmdInstall(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := os.MkdirAll(home, 0o755); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	var override string
	if len(args) > 0 {
		override = args[0]
	}
	rc := rcPath(override)

	if fileContains(rc, "dsctl shell init") {
		fmt.Fprintln(stderr, warnLine(stderr, "Detecté el init viejo de dsctl en "+rc+"."))
		fmt.Fprintln(stderr, "  Quítalo con:  ccp uninstall   (o edita el bloque '# >>> dsctl shell init >>>')")
	}
	if fileContains(rc, "ccp shell init") {
		fmt.Fprintln(stdout, okLine(stdout, "El init de ccp ya está en "+rc))
		fmt.Fprintln(stdout, hrLine)
		fmt.Fprintln(stdout, "Recarga con:  source "+rc)
		fmt.Fprintln(stdout, hrLine)
		return 0
	}

	f, err := os.OpenFile(rc, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "[error] no se pudo abrir %s: %v\n", rc, err)
		return 1
	}
	fmt.Fprintf(f, "\n%s\n", rcComment)
	if _, err := core.WriteShellInit(f); err != nil {
		_ = f.Close()
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if err := f.Close(); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, "Init añadido a "+rc))
	fmt.Fprintln(stdout, hrLine)
	fmt.Fprintln(stdout, "Recarga con:  source "+rc)
	fmt.Fprintln(stdout, hrLine)
	return 0
}

// cmdUninstall remueve el bloque shell-init del rc. Espeja cmd_uninstall: borra
// desde la línea de comentario o el marcador de apertura hasta el de cierre.
func cmdUninstall(args []string, stdout, stderr io.Writer) int {
	var override string
	if len(args) > 0 {
		override = args[0]
	}
	rc := rcPath(override)
	if !fileContains(rc, "ccp shell init") {
		fmt.Fprintln(stderr, warnLine(stderr, "No encontré el init en "+rc))
		return 0
	}
	in, err := os.Open(rc)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	var out []string
	skip := false
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		line := sc.Text()
		if line == rcComment || line == rcMarkerOpen {
			skip = true
		}
		if !skip {
			out = append(out, line)
		}
		if line == rcMarkerEnd {
			skip = false
		}
	}
	_ = in.Close()
	if err := sc.Err(); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	content := strings.Join(out, "\n")
	if len(out) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(rc, []byte(content), 0o644); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, "Init de ccp removido de "+rc))
	return 0
}

// cmdUpgrade re-ejecuta install.sh desde la fuente registrada (install-source),
// luego re-sincroniza perfiles con el binario recién instalado y avisa si el
// bloque rc quedó desfasado. Espeja cmd_upgrade.
//
//	ccp upgrade [--pull] [--no-sync]
func cmdUpgrade(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	doPull, doSync := false, true
	for _, a := range args {
		switch a {
		case "--pull":
			doPull = true
		case "--no-sync":
			doSync = false
		default:
			fmt.Fprintln(stderr, "Uso: ccp upgrade [--pull] [--no-sync]")
			return 1
		}
	}

	srcFile := filepath.Join(home, "install-source")
	srcBytes, err := os.ReadFile(srcFile)
	if err != nil {
		fmt.Fprintln(stderr, "[error] No hay fuente registrada. Corre 'bash install.sh' desde el repo una vez.")
		return 1
	}
	src := strings.TrimSpace(string(srcBytes))
	installSh := filepath.Join(src, "install.sh")
	if fi, err := os.Stat(installSh); err != nil || fi.IsDir() {
		fmt.Fprintf(stderr, "[error] Repo registrado inválido: %s (re-corre install.sh).\n", src)
		return 1
	}

	if doPull {
		if _, err := exec.LookPath("git"); err != nil {
			fmt.Fprintln(stderr, "[error] git no está instalado (--pull no disponible).")
			return 1
		}
		fmt.Fprintln(stdout, "git pull en "+src+"...")
		pull := exec.Command("git", "-C", src, "pull")
		pull.Stdout, pull.Stderr = stdout, stderr
		if err := pull.Run(); err != nil {
			fmt.Fprintln(stderr, "[error] git pull falló.")
			return 1
		}
	}

	fmt.Fprintln(stdout, "Reinstalando desde "+src+"...")
	inst := exec.Command("bash", installSh)
	inst.Stdout, inst.Stderr = stdout, stderr
	if err := inst.Run(); err != nil {
		fmt.Fprintln(stderr, "[error] install.sh falló.")
		return 1
	}

	binDir := os.Getenv("CCP_BIN_DIR")
	if binDir == "" {
		uh, _ := os.UserHomeDir()
		binDir = filepath.Join(uh, ".local", "bin")
	}
	newbin := filepath.Join(binDir, "ccp")

	warnIfStaleRC(newbin, stdout, stderr)

	if doSync {
		fmt.Fprintln(stdout, "Sincronizando perfiles (migración overlay + regen)...")
		sync := exec.Command(newbin, "profile", "sync")
		sync.Stdout, sync.Stderr = stdout, stderr
		if err := sync.Run(); err != nil {
			fmt.Fprintln(stderr, warnLine(stderr, "profile sync tuvo problemas; revisa con 'ccp doctor'."))
		}
	}

	fmt.Fprintln(stdout, hrLine)
	fmt.Fprintln(stdout, okLine(stdout, "ccp actualizado."))
	fmt.Fprintln(stdout, hrLine)
	fmt.Fprintln(stdout, "Completions nuevos: abre una terminal nueva o 'source' tu rc.")
	return 0
}

// warnIfStaleRC compara el bloque shell-init del rc con el que emite el binario
// nuevo; avisa (no edita) si difieren. Espeja _upgrade_check_rc.
func warnIfStaleRC(newbin string, stdout, stderr io.Writer) {
	rc := rcPath("")
	if !fileContains(rc, rcMarkerOpen) {
		return
	}
	data, err := os.ReadFile(rc)
	if err != nil {
		return
	}
	current := extractBlock(string(data))
	out, err := exec.Command(newbin, "completion-shellinit").Output()
	if err != nil {
		return
	}
	if current == string(out) {
		return
	}
	fmt.Fprintln(stderr, warnLine(stderr, "El shell-init de ccp cambió en esta versión (tu rc tiene el viejo)."))
	fmt.Fprintln(stderr, "  Actualízalo:  ccp uninstall && ccp install && source "+rc)
}

// extractBlock devuelve el bloque entre los marcadores (inclusive), con \n
// final, igual que el awk de _upgrade_check_rc.
func extractBlock(content string) string {
	var b strings.Builder
	in := false
	for _, line := range strings.Split(content, "\n") {
		if line == rcMarkerOpen {
			in = true
		}
		if in {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if line == rcMarkerEnd {
			break
		}
	}
	return b.String()
}
