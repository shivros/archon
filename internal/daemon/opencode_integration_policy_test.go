package daemon

const (
	opencodeIntegrationEnv = "ARCHON_OPENCODE_INTEGRATION"
	kilocodeIntegrationEnv = "ARCHON_KILOCODE_INTEGRATION"
)

var openCodeIntegrationFallbackModels = map[string]string{
	"opencode": "openrouter/google/gemini-2.5-flash",
	"kilocode": "moonshotai/kimi-k2.5",
}
