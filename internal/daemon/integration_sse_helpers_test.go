package daemon

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

const integrationSSEPollInterval = 50 * time.Millisecond

func openSSE(t *testing.T, server *httptest.Server, path string) (<-chan string, func()) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open sse: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("open sse status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "data: ") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
				if payload != "" {
					ch <- payload
				}
			}
		}
	}()
	closeFn := func() {
		_ = resp.Body.Close()
	}
	return ch, closeFn
}

func waitForSSEData(ch <-chan string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return "", false
			}
			return data, true
		case <-time.After(integrationSSEPollInterval):
		}
	}
	return "", false
}

func collectEvents(ch <-chan string, timeout time.Duration) []types.CodexEvent {
	deadline := time.Now().Add(timeout)
	out := make([]types.CodexEvent, 0)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return out
			}
			if event, ok := codexEventFromSSEPayload(data); ok {
				out = append(out, event)
			}
		case <-time.After(integrationSSEPollInterval):
		}
	}
	return out
}

func waitForEvent(ch <-chan string, method string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return false
			}
			event, parsed := codexEventFromSSEPayload(data)
			if !parsed {
				continue
			}
			if event.Method == method {
				return true
			}
		case <-time.After(integrationSSEPollInterval):
		}
	}
	return false
}

func codexEventFromSSEPayload(data string) (types.CodexEvent, bool) {
	var event types.CodexEvent
	if err := json.Unmarshal([]byte(data), &event); err == nil && strings.TrimSpace(event.Method) != "" {
		return event, true
	}

	var transcriptEvent transcriptdomain.TranscriptEvent
	if err := json.Unmarshal([]byte(data), &transcriptEvent); err != nil {
		return types.CodexEvent{}, false
	}
	synth := types.CodexEvent{}
	switch transcriptEvent.Kind {
	case transcriptdomain.TranscriptEventTurnStarted:
		synth.Method = "turn/started"
		synth.Params = mustJSONMarshal(map[string]any{
			"turn": map[string]any{
				"id":     transcriptTurnID(transcriptEvent),
				"status": "started",
			},
		})
	case transcriptdomain.TranscriptEventTurnCompleted:
		synth.Method = "turn/completed"
		synth.Params = mustJSONMarshal(map[string]any{
			"turn": map[string]any{
				"id":     transcriptTurnID(transcriptEvent),
				"status": "completed",
			},
		})
	case transcriptdomain.TranscriptEventTurnFailed:
		status := "interrupted"
		if turnError := strings.ToLower(strings.TrimSpace(transcriptTurnError(transcriptEvent))); strings.Contains(turnError, "interrupt") {
			status = "interrupted"
		}
		synth.Method = "turn/completed"
		synth.Params = mustJSONMarshal(map[string]any{
			"turn": map[string]any{
				"id":     transcriptTurnID(transcriptEvent),
				"status": status,
				"error":  transcriptTurnError(transcriptEvent),
			},
		})
	case transcriptdomain.TranscriptEventApprovalPending, transcriptdomain.TranscriptEventApprovalResolved:
		synth.Method = "approval"
		if transcriptEvent.Approval != nil {
			if method := strings.TrimSpace(transcriptEvent.Approval.Method); method != "" {
				synth.Method = method
			}
			if transcriptEvent.Approval.RequestID != 0 {
				id := transcriptEvent.Approval.RequestID
				synth.ID = &id
			}
		}
	default:
		return types.CodexEvent{}, false
	}
	return synth, true
}

func transcriptTurnID(event transcriptdomain.TranscriptEvent) string {
	if event.Turn == nil {
		return ""
	}
	return strings.TrimSpace(event.Turn.TurnID)
}

func transcriptTurnError(event transcriptdomain.TranscriptEvent) string {
	if event.Turn == nil {
		return ""
	}
	return strings.TrimSpace(event.Turn.Error)
}

func mustJSONMarshal(payload map[string]any) json.RawMessage {
	data, _ := json.Marshal(payload)
	return data
}
