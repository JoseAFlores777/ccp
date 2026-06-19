package core

import "testing"

func TestIsProviderType(t *testing.T) {
	for _, id := range []string{"deepseek", "kimi", "glm"} {
		if !IsProviderType(id) {
			t.Errorf("IsProviderType(%q) = false, quiero true", id)
		}
	}
	for _, id := range []string{"official", "default", "", "kimmi", "GLM"} {
		if IsProviderType(id) {
			t.Errorf("IsProviderType(%q) = true, quiero false", id)
		}
	}
}

func TestPresetDefaults(t *testing.T) {
	cases := map[string]Defaults{
		"deepseek": {BaseURL: "https://api.deepseek.com/anthropic", ModelPro: "deepseek-chat", ModelFlash: "deepseek-chat", Effort: "high"},
		"kimi":     {BaseURL: "https://api.moonshot.ai/anthropic", ModelPro: "kimi-k2.7-code", ModelFlash: "kimi-k2.7-code", Effort: "high"},
		"glm":      {BaseURL: "https://api.z.ai/api/anthropic", ModelPro: "glm-5.2[1m]", ModelFlash: "glm-4.7", Effort: "high"},
	}
	for id, want := range cases {
		got := PresetDefaults(id)
		if got != want {
			t.Errorf("PresetDefaults(%q) = %+v, quiero %+v", id, got, want)
		}
		// Editor nunca lo siembra un preset de proveedor.
		if got.Editor != "" {
			t.Errorf("PresetDefaults(%q).Editor = %q, quiero vacío", id, got.Editor)
		}
	}
}

// Las vars Extra de cada proveedor deben estar TODAS en CCPManagedVars para que
// el unset las limpie al cambiar de perfil.
func TestProviderExtraVarsAreManaged(t *testing.T) {
	for _, id := range ProviderTypes() {
		preset, ok := GetProviderPreset(id)
		if !ok {
			t.Fatalf("GetProviderPreset(%q) faltante", id)
		}
		for _, ev := range preset.Extra {
			if !contractContains(CCPManagedVars, ev.Name) {
				t.Errorf("var Extra %q de %q no está en CCPManagedVars", ev.Name, id)
			}
		}
	}
}

func contractContains(list, name string) bool {
	for _, w := range splitFields(list) {
		if w == name {
			return true
		}
	}
	return false
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
