package cli

import (
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// env.go cablea la frontera binario↔shell: los comandos cuya salida el shell
// hace `eval` (_env, _hook) y el `resolve` scriptable. Son el contrato
// congelado — su salida debe coincidir byte-a-byte con el oráculo bash.

// safeDefaultDelta es el delta de entorno que emitimos cuando no se puede
// cargar la config (migración/lectura fallida) en un comando eval-able: limpia
// las managed vars y marca default, de modo que el shell nunca quede roto.
func safeDefaultDelta() string {
	return "unset " + core.CCPManagedVars + "\nexport CCP_PROFILE=default\n"
}

// cmdResolve imprime el perfil que aplica al path (o al cwd) y fija el exit
// code: 0 = una regla no-default ganó, 1 = default. Espeja cmd_resolve.
func cmdResolve(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	query := currentDir()
	if len(args) > 0 && args[0] != "" {
		query = args[0]
	}
	prof := core.Resolve(query, cfg.Rules)
	fmt.Fprintln(stdout, prof)
	if prof == "default" {
		return 1
	}
	return 0
}

// cmdEnv emite el delta de entorno (eval-able) de un perfil nombrado. Espeja
// cmd_env: `_env [perfil]`, default "default".
func cmdEnv(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	profile := "default"
	if len(args) > 0 && args[0] != "" {
		profile = args[0]
	}
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[warn] ccp: no se pudo cargar la config (%v); usando default\n", err)
		io.WriteString(stdout, safeDefaultDelta())
		return 0
	}
	io.WriteString(stdout, core.EnvDelta(home, profile, cfg))
	return 0
}

// cmdHook resuelve el perfil del path (o cwd) y emite su delta en un solo fork.
// Espeja cmd_hook: lo llama el hook _ccp_autocheck en cada cambio de PWD.
func cmdHook(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[warn] ccp: no se pudo cargar la config (%v); usando default\n", err)
		io.WriteString(stdout, safeDefaultDelta())
		return 0
	}
	query := currentDir()
	if len(args) > 0 && args[0] != "" {
		query = args[0]
	}
	prof := core.Resolve(query, cfg.Rules)
	io.WriteString(stdout, core.EnvDelta(home, prof, cfg))
	return 0
}
