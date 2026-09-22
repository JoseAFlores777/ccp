package cli

// auto_live.go — `ccp auto live`: las sesiones supervisadas vistas desde fuera
// (core.LiveSession), lo mismo que la app pinta en el detalle de una
// conversación. No está en la completion: es contrato golden.

import (
	"fmt"
	"io"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

func autoLive(args []string, stdout, stderr io.Writer) int {
	asJSON, running := false, false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--running":
			running = true
		default:
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.auto.unknown_flag", a))
			return 1
		}
	}
	home := resolveHome()
	list, err := core.ReadLiveSessions(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if running {
		keep := list[:0]
		for _, s := range list {
			if s.State == core.LiveRunning {
				keep = append(keep, s)
			}
		}
		list = keep
	}
	if asJSON {
		return snapJSON(stdout, stderr, list)
	}
	lang := currentLang()
	if len(list) == 0 {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.auto.live_none"))
		return 0
	}
	now := time.Now()
	for i, s := range list {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintln(stdout, boldLine(stdout, fmt.Sprintf("%s · %s · %s", liveStateText(lang, s), tilde(s.Cwd), s.StartedAt.Local().Format("02/01 15:04"))))
		fmt.Fprintln(stdout, i18n.T(lang, "cli.auto.live_where", s.Current, s.Since.Local().Format("15:04"), s.LoansUsed, s.MaxHops, shortID(s.Session)))
		for _, h := range s.Hops {
			mark := ""
			if h.Home {
				mark = i18n.T(lang, "cli.auto.live_home_mark")
			}
			fmt.Fprintln(stdout, i18n.T(lang, "cli.auto.live_hop", h.At.Local().Format("15:04"), h.From, h.To, mark))
		}
		for _, c := range s.Cooldowns {
			if c.Until.After(now) {
				fmt.Fprintln(stdout, i18n.T(lang, "cli.auto.live_cooldown", c.Profile, c.Until.Local().Format("15:04")))
			}
		}
		if s.State == core.LiveRunning && s.Current != s.Primary && !s.NoReturn {
			free := "—"
			for _, c := range s.Cooldowns {
				if c.Profile == s.Primary {
					free = c.Until.Local().Format("15:04")
				}
			}
			fmt.Fprintln(stdout, i18n.T(lang, "cli.auto.live_return", s.Primary, free, s.ReturnIdle, s.ReturnCheck))
		}
	}
	return 0
}

func liveStateText(lang i18n.Lang, s core.LiveSession) string {
	switch s.State {
	case core.LiveRunning:
		return i18n.T(lang, "cli.auto.live_state_running")
	case core.LiveParked:
		return i18n.T(lang, "cli.auto.live_state_parked")
	case core.LiveDone:
		return i18n.T(lang, "cli.auto.live_state_done", s.ExitCode)
	case core.LiveFailed:
		return i18n.T(lang, "cli.auto.live_state_failed", s.Error)
	default:
		return i18n.T(lang, "cli.auto.live_state_lost")
	}
}
