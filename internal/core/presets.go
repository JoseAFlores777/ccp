package core

// presets.go — registro de proveedores compatibles con la API Anthropic.
//
// Un perfil "provider" (no-official) lleva como Type el id del proveedor:
// "deepseek", "kimi" o "glm". Cada id mapea a un ProviderPreset que aporta el
// base_url y los modelos por defecto al SEMBRAR un perfil nuevo, más las
// variables de entorno fijas y específicas del proveedor que env.go emite.
//
// Los tres comparten la misma forma de 2 modelos (pro/flash) que DeepSeek ya
// usaba; env.go los abanica a ANTHROPIC_MODEL/OPUS/SONNET (pro) y HAIKU/SUBAGENT
// (flash). El único delta real entre proveedores es el base_url, los modelos
// por defecto y el campo Extra (vars de tuning recomendadas por cada doc).
//
// Añadir un proveedor #4 = una fila en providerPresets (+ su id en
// providerOrder) y, si trae vars Extra nuevas, añadir esos nombres a
// CCPManagedVars en env.go (y al oráculo bash).

// EnvVar es un par nombre/valor de entorno. Se usa una lista ordenada (no un
// map) para que la emisión de las vars Extra sea determinista y el golden
// estable.
type EnvVar struct {
	Name  string
	Value string
}

// ProviderPreset describe un proveedor compatible: base_url y modelos por
// defecto (para sembrar perfiles nuevos) más las vars de entorno fijas que el
// proveedor recomienda. Effort por defecto al sembrar.
type ProviderPreset struct {
	BaseURL    string
	ModelPro   string
	ModelFlash string
	Effort     string
	// Extra son las vars específicas del proveedor; env.go las emite tras las
	// vars de modelo/effort y antes de CCP_PROFILE, en ESTE orden.
	Extra []EnvVar
}

// providerPresets es el registro de proveedores built-in, indexado por su
// id/Type. Los valores espejan la doc oficial de cada proveedor.
var providerPresets = map[string]ProviderPreset{
	"deepseek": {
		BaseURL:    "https://api.deepseek.com/anthropic",
		ModelPro:   "deepseek-chat",
		ModelFlash: "deepseek-chat",
		Effort:     "high",
	},
	"kimi": {
		BaseURL:    "https://api.moonshot.ai/anthropic",
		ModelPro:   "kimi-k2.7-code",
		ModelFlash: "kimi-k2.7-code",
		Effort:     "high",
		Extra: []EnvVar{
			{Name: "ENABLE_TOOL_SEARCH", Value: "false"},
			{Name: "CLAUDE_CODE_AUTO_COMPACT_WINDOW", Value: "262144"},
		},
	},
	"glm": {
		BaseURL:    "https://api.z.ai/api/anthropic",
		ModelPro:   "glm-5.2[1m]",
		ModelFlash: "glm-4.7",
		Effort:     "high",
		Extra: []EnvVar{
			{Name: "API_TIMEOUT_MS", Value: "3000000"},
			{Name: "CLAUDE_CODE_AUTO_COMPACT_WINDOW", Value: "1000000"},
		},
	},
}

// providerOrder es el orden estable de iteración/visualización de proveedores.
var providerOrder = []string{"deepseek", "kimi", "glm"}

// GetProviderPreset devuelve el preset de un proveedor y si existe.
func GetProviderPreset(id string) (ProviderPreset, bool) {
	p, ok := providerPresets[id]
	return p, ok
}

// IsProviderType reporta si t es un tipo de proveedor compatible conocido
// (deepseek/kimi/glm). Los demás Type ("official") no lo son.
func IsProviderType(t string) bool {
	_, ok := providerPresets[t]
	return ok
}

// ProviderTypes devuelve los ids de proveedor en orden estable.
func ProviderTypes() []string {
	out := make([]string, len(providerOrder))
	copy(out, providerOrder)
	return out
}

// PresetDefaults convierte un preset de proveedor en un Defaults-semilla
// (Editor queda vacío: el caller conserva el suyo). Se usa para sembrar un
// perfil nuevo de ese tipo cuando no hay defaults configurables propios.
func PresetDefaults(id string) Defaults {
	p := providerPresets[id]
	return Defaults{
		BaseURL:    p.BaseURL,
		ModelPro:   p.ModelPro,
		ModelFlash: p.ModelFlash,
		Effort:     p.Effort,
	}
}
