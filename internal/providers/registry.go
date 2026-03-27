package providers

import "strings"

type Capabilities struct {
	SupportsGuidedWorkflowDispatch bool
	UsesItems                      bool
	SupportsEvents                 bool
	SupportsApprovals              bool
	SupportsInterrupt              bool
	SupportsFileSearch             bool
	NoProcess                      bool
}

type Runtime string

const (
	RuntimeCodex          Runtime = "codex"
	RuntimeClaude         Runtime = "claude"
	RuntimeExec           Runtime = "exec"
	RuntimeOpenCodeServer Runtime = "opencode_server"
	RuntimeCustom         Runtime = "custom"
)

type Definition struct {
	Name              string
	Label             string
	Runtime           Runtime
	CommandCandidates []string
	Capabilities      Capabilities
	Bootstrap         BootstrapProfile
}

var registry = []Definition{
	{
		Name:              "codex",
		Label:             "codex",
		Runtime:           RuntimeCodex,
		CommandCandidates: []string{"codex"},
		Capabilities: Capabilities{
			SupportsGuidedWorkflowDispatch: true,
			SupportsEvents:                 true,
			SupportsApprovals:              true,
			SupportsInterrupt:              true,
			SupportsFileSearch:             true,
		},
		Bootstrap: BootstrapProfile{
			HistoryConsistency:     HistoryConsistencyEventuallyConsistent,
			SessionStartTranscript: TranscriptBootstrapModeDeferSnapshot,
		},
	},
	{
		Name:              "claude",
		Label:             "claude",
		Runtime:           RuntimeClaude,
		CommandCandidates: []string{"claude"},
		Capabilities: Capabilities{
			SupportsGuidedWorkflowDispatch: true,
			UsesItems:                      true,
			SupportsApprovals:              true,
			SupportsInterrupt:              true,
			NoProcess:                      true,
		},
		Bootstrap: defaultBootstrapProfile(),
	},
	{
		Name:    "opencode",
		Label:   "opencode",
		Runtime: RuntimeOpenCodeServer,
		Capabilities: Capabilities{
			SupportsGuidedWorkflowDispatch: true,
			UsesItems:                      true,
			SupportsEvents:                 true,
			SupportsApprovals:              true,
			SupportsInterrupt:              true,
			SupportsFileSearch:             true,
			NoProcess:                      true,
		},
		Bootstrap: defaultBootstrapProfile(),
	},
	{
		Name:    "kilocode",
		Label:   "kilocode",
		Runtime: RuntimeOpenCodeServer,
		Capabilities: Capabilities{
			SupportsGuidedWorkflowDispatch: true,
			UsesItems:                      true,
			SupportsEvents:                 true,
			SupportsApprovals:              true,
			SupportsInterrupt:              true,
			SupportsFileSearch:             true,
			NoProcess:                      true,
		},
		Bootstrap: defaultBootstrapProfile(),
	},
	{
		Name:              "gemini",
		Label:             "gemini",
		Runtime:           RuntimeExec,
		CommandCandidates: []string{"gemini"},
		Bootstrap:         defaultBootstrapProfile(),
	},
	{
		Name:      "custom",
		Label:     "custom",
		Runtime:   RuntimeCustom,
		Bootstrap: defaultBootstrapProfile(),
	},
}

var registryByName = buildByName(registry)

func Normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func All() []Definition {
	out := make([]Definition, 0, len(registry))
	for _, def := range registry {
		out = append(out, cloneDefinition(def))
	}
	return out
}

func Lookup(name string) (Definition, bool) {
	key := Normalize(name)
	def, ok := registryByName[key]
	if !ok {
		return Definition{}, false
	}
	return cloneDefinition(def), true
}

func CapabilitiesFor(name string) Capabilities {
	def, ok := Lookup(name)
	if !ok {
		return Capabilities{}
	}
	return def.Capabilities
}

func BootstrapProfileFor(name string) BootstrapProfile {
	def, ok := Lookup(name)
	if !ok {
		return defaultBootstrapProfile()
	}
	profile := def.Bootstrap
	if profile.HistoryConsistency == "" {
		profile.HistoryConsistency = HistoryConsistencyImmediate
	}
	if profile.SessionStartTranscript == "" {
		profile.SessionStartTranscript = TranscriptBootstrapModeSnapshotFirst
	}
	return profile
}

func buildByName(defs []Definition) map[string]Definition {
	out := make(map[string]Definition, len(defs))
	for _, def := range defs {
		name := Normalize(def.Name)
		if name == "" {
			continue
		}
		out[name] = cloneDefinition(def)
	}
	return out
}

func cloneDefinition(def Definition) Definition {
	copy := def
	if def.CommandCandidates != nil {
		copy.CommandCandidates = append([]string{}, def.CommandCandidates...)
	}
	return copy
}
