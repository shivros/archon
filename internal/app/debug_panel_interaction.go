package app

import "strings"

type defaultDebugPanelInteractionService struct{}

func NewDefaultDebugPanelInteractionService() debugPanelInteractionService {
	return defaultDebugPanelInteractionService{}
}

func (defaultDebugPanelInteractionService) HitTest(spans []renderedBlockSpan, yOffset int, col, line int) (debugPanelControlHit, bool) {
	if len(spans) == 0 || col < 0 || line < 0 {
		return debugPanelControlHit{}, false
	}
	absolute := yOffset + line
	for _, span := range spans {
		if absolute < span.StartLine || absolute > span.EndLine {
			continue
		}
		blockID := strings.TrimSpace(span.ID)
		if blockID == "" {
			continue
		}
		for _, control := range span.MetaControls {
			if control.Line != absolute || control.Start < 0 || control.End < control.Start {
				continue
			}
			if col < control.Start || col > control.End {
				continue
			}
			controlID := control.ID
			if strings.TrimSpace(string(controlID)) == "" {
				controlID = debugControlFromLabel(control.Label)
			}
			if strings.TrimSpace(string(controlID)) == "" {
				return debugPanelControlHit{}, false
			}
			return debugPanelControlHit{BlockID: blockID, ControlID: controlID}, true
		}
		return debugPanelControlHit{}, false
	}
	return debugPanelControlHit{}, false
}

func debugControlFromLabel(label string) ChatMetaControlID {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "[copy]":
		return debugMetaControlCopy
	case "[expand]", "[collapse]":
		return debugMetaControlToggle
	default:
		return ""
	}
}
