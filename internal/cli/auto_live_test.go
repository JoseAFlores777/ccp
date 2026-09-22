package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// `ccp auto live --json` es siempre un array, y una sesión que dice «running»
// con su proceso muerto se lee «lost»: nunca se da por viva una que no lo está.
func TestAutoLiveJSONYPerdida(t *testing.T) {
	home := serveEnv(t)
	now := time.Now()
	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"live", "--json"}, &out, &errb); code != 0 || strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("sin sesiones: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	mine := core.LiveSession{ID: core.NewLiveID(now, os.Getpid()), PID: os.Getpid(), State: core.LiveRunning,
		StartedAt: now, UpdatedAt: now, Primary: "trabajo", Current: "personal", Session: "u-2", Sessions: []string{"u-1", "u-2"}, Origin: "u-0"}
	dead := core.LiveSession{ID: "1-999999", PID: 999999, State: core.LiveRunning, StartedAt: now.Add(-time.Hour), UpdatedAt: now}
	for _, s := range []core.LiveSession{mine, dead} {
		if err := core.WriteLiveSession(home, s); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	if code := dispatchAuto([]string{"live", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	var got []core.LiveSession
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got) != 2 {
		t.Fatalf("json: %v %q", err, out.String())
	}
	if got[0].State != core.LiveRunning || got[1].State != core.LiveLost {
		t.Fatalf("estados = %s, %s; quería running (vivo) y lost (proceso muerto)", got[0].State, got[1].State)
	}

	// El método de la app filtra por conversación: la original, cualquier uuid
	// usado o la actual la encuentran; otra conversación no.
	for uuid, want := range map[string]int{"u-0": 1, "u-1": 1, "u-2": 1, "otra": 0} {
		_, rep := serveRun(t, req(1, "auto.live", map[string]any{"session": uuid}))
		var l []core.LiveSession
		mustResult(t, rep["1"], &l)
		if len(l) != want {
			t.Fatalf("auto.live(%s) = %d sesiones, quería %d", uuid, len(l), want)
		}
	}
}
