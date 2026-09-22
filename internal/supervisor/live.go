package supervisor

// live.go — publica el estado de la corrida en core.LiveSession para que la app
// (y `ccp auto live`) puedan enseñar dónde está la conversación, a dónde pasó y
// cuándo vuelve, mientras ocurre. Solo LEE la cadena y el resultado: nunca
// decide nada, y un fallo al escribir se ignora — una caché de presentación no
// puede tumbar la sesión que describe.

import (
	"os"
	"sort"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

type liveRecorder struct {
	home  string
	now   func() time.Time
	chain *Chain
	s     core.LiveSession
}

func newLiveRecorder(o Options, chain *Chain, session string, start time.Time) *liveRecorder {
	core.PruneLiveSessions(o.Home, start)
	pid := os.Getpid()
	return &liveRecorder{
		home:  o.Home,
		now:   o.Now,
		chain: chain,
		s: core.LiveSession{
			ID:          core.NewLiveID(start, pid),
			PID:         pid,
			State:       core.LiveRunning,
			StartedAt:   start,
			Cwd:         o.Cwd,
			Origin:      o.Origin,
			Session:     session,
			Primary:     chain.Primary(),
			Chain:       append([]string{chain.Primary()}, chain.Fallback()...),
			Policy:      chain.policy.Name,
			ReturnCheck: int(chain.policy.ReturnCheck / time.Second),
			ReturnIdle:  int(chain.policy.ReturnIdle / time.Second),
			NoReturn:    o.NoReturn,
			Headless:    o.Headless,
		},
	}
}

// sync vuelca el estado actual de la cadena y los saltos hechos, y publica.
func (l *liveRecorder) sync(session string, hops []Hop) {
	l.syncAt(session, hops, "")
}

// syncAt es sync con la cuenta actual explícita: al terminar, la conversación
// puede haber vuelto a casa (el cierre del préstamo) sin que la cadena avance.
func (l *liveRecorder) syncAt(session string, hops []Hop, current string) {
	l.s.Session = session
	if n := len(l.s.Sessions); n == 0 || l.s.Sessions[n-1] != session {
		l.s.Sessions = append(l.s.Sessions, session)
	}
	l.s.Current = l.chain.Current()
	if current != "" {
		l.s.Current = current
	}
	l.s.Since = l.chain.Since()
	l.s.LoansUsed = l.chain.Hops()
	l.s.MaxHops = l.chain.MaxHops()

	l.s.Hops = l.s.Hops[:0]
	for _, h := range hops {
		l.s.Hops = append(l.s.Hops, core.LiveHop{
			From: h.From, To: h.To, At: h.At, Reason: h.Reason, Session: h.Session,
			Home: h.To == l.chain.Primary(),
		})
	}
	now := l.now()
	l.s.Cooldowns = l.s.Cooldowns[:0]
	for p, until := range l.chain.Cooldowns() {
		if until.After(now) {
			l.s.Cooldowns = append(l.s.Cooldowns, core.LiveCooldown{Profile: p, Until: until})
		}
	}
	sort.Slice(l.s.Cooldowns, func(i, j int) bool { return l.s.Cooldowns[i].Until.Before(l.s.Cooldowns[j].Until) })
	l.s.UpdatedAt = now
	_ = core.WriteLiveSession(l.home, l.s)
}

// finish publica el desenlace.
func (l *liveRecorder) finish(res Result, err error) {
	switch {
	case err != nil:
		l.s.State = core.LiveFailed
		l.s.Error = err.Error()
	case res.Parked:
		l.s.State = core.LiveParked
	default:
		l.s.State = core.LiveDone
	}
	l.s.ExitCode = res.ExitCode
	if res.Session == "" {
		res.Session = l.s.Session
	}
	l.syncAt(res.Session, res.Hops, res.Profile)
}
