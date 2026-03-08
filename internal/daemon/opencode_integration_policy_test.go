package daemon

const (
	opencodeIntegrationEnv          = "ARCHON_OPENCODE_INTEGRATION"
	kilocodeIntegrationEnv          = "ARCHON_KILOCODE_INTEGRATION"
	openCodeIntegrationDefaultModel = "openrouter/x-ai/grok-4.1-fast"
)

var openCodeIntegrationFallbackModels = map[string]string{
	"opencode": openCodeIntegrationDefaultModel,
	"kilocode": openCodeIntegrationDefaultModel,
}
