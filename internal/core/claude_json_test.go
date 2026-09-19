package core

import (
	"encoding/json"
	"reflect"
	"testing"
)

const liveClaudeJSON = `{
  "machineID": "m-123",
  "numStartups": 42,
  "oauthAccount": {"emailAddress": "x@y.z"},
  "mcpServers": {"github": {"command": "npx", "args": ["-y", "gh"], "env": {"TOKEN": "t"}}},
  "projects": {
    "/repo/a": {"allowedTools": ["Bash(ls)"], "mcpServers": {}, "history": [1, 2], "hasTrustDialogAccepted": true},
    "/repo/b": {"mcpServers": {"local": {"command": "node"}}, "enabledMcpjsonServers": ["x"]},
    "/repo/c": {"history": []}
  }
}`

func decode(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, data)
	}
	return v
}

func TestClaudeJSONConfigKeepsOnlyConfig(t *testing.T) {
	sub, err := ClaudeJSONConfig([]byte(liveClaudeJSON))
	if err != nil {
		t.Fatal(err)
	}
	got := decode(t, sub)
	want := decode(t, []byte(`{
	  "mcpServers": {"github": {"command": "npx", "args": ["-y", "gh"], "env": {"TOKEN": "t"}}},
	  "projects": {
	    "/repo/a": {"allowedTools": ["Bash(ls)"]},
	    "/repo/b": {"mcpServers": {"local": {"command": "node"}}, "enabledMcpjsonServers": ["x"]}
	  }
	}`))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subconjunto = %v\nquiero %v", got, want)
	}
}

func TestClaudeJSONConfigNothingToKeep(t *testing.T) {
	sub, err := ClaudeJSONConfig([]byte(`{"machineID":"x","mcpServers":{},"projects":{"/r":{"history":[]}}}`))
	if err != nil || sub != nil {
		t.Fatalf("sub = %s, %v; quiero nil", sub, err)
	}
	if _, err := ClaudeJSONConfig([]byte(`[1]`)); err == nil {
		t.Fatal("aceptó un .claude.json que no es un objeto")
	}
}

// Restaurar deja la configuración exactamente como en el snapshot y no toca
// nada más: el estado de Claude Code (machineID, contadores, historial) sigue
// siendo el vivo.
func TestClaudeJSONApplyConfig(t *testing.T) {
	sub, _ := ClaudeJSONConfig([]byte(liveClaudeJSON))
	live := []byte(`{
	  "machineID": "m-999",
	  "numStartups": 50,
	  "mcpServers": {"otro": {"command": "x"}},
	  "projects": {
	    "/repo/a": {"allowedTools": ["Bash(rm)"], "history": [1, 2, 3]},
	    "/repo/d": {"mcpServers": {"nuevo": {"command": "y"}}, "history": [9]}
	  }
	}`)
	out, err := ClaudeJSONApplyConfig(live, sub)
	if err != nil {
		t.Fatal(err)
	}
	got := decode(t, out)
	want := decode(t, []byte(`{
	  "machineID": "m-999",
	  "numStartups": 50,
	  "mcpServers": {"github": {"command": "npx", "args": ["-y", "gh"], "env": {"TOKEN": "t"}}},
	  "projects": {
	    "/repo/a": {"allowedTools": ["Bash(ls)"], "history": [1, 2, 3]},
	    "/repo/b": {"mcpServers": {"local": {"command": "node"}}, "enabledMcpjsonServers": ["x"]},
	    "/repo/d": {"history": [9]}
	  }
	}`))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fusión = %v\nquiero %v", got, want)
	}
}

func TestClaudeJSONApplyConfigToMissingFile(t *testing.T) {
	sub, _ := ClaudeJSONConfig([]byte(liveClaudeJSON))
	out, err := ClaudeJSONApplyConfig(nil, sub)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decode(t, out), decode(t, sub)) {
		t.Fatalf("sobre un archivo que no existe, el resultado debe ser la configuración tal cual:\n%s", out)
	}
	if _, err := ClaudeJSONApplyConfig([]byte(`[1]`), sub); err == nil {
		t.Fatal("fusionó sobre algo que no es un objeto")
	}
}
