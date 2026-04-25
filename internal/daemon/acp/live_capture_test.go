package acp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveCapture exercises the real hermes acp binary and captures protocol
// fixtures. It is skipped if hermes is not on PATH.
func TestLiveCapture(t *testing.T) {
	if _, err := exec.LookPath("hermes"); err != nil {
		t.Skip("hermes not on PATH, skipping live capture")
	}

	outDir := filepath.Join("testdata", "live")
	os.MkdirAll(outDir, 0o755)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client, err := Start(ctx, StartOptions{
		Command: "hermes",
		Args:    []string{"acp"},
		Env:     os.Environ(),
		Cwd:     "/tmp",
		ClientInfo: ImplementationInfo{
			Name:    "archon-capture",
			Version: "0.1.0",
		},
		ClientCapabilities: ClientCapabilities{},
		ProtocolVersion:    ProtocolVersion1,
		InitializeTimeout:  30 * time.Second,
		CloseTimeout:       10 * time.Second,
		Logger:             func(format string, args ...any) { t.Logf("[DBG] "+format, args...) },
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer client.Close(context.Background())

	// 1. Capture initialize result
	caps := client.AgentCapabilities()
	info := client.AgentInfo()
	t.Logf("Initialize OK: agent=%s v%s, loadSession=%v", info.Name, info.Version, caps.LoadSession)
	writeFixture(t, filepath.Join(outDir, "initialize_response.json"), map[string]any{
		"agentCapabilities": caps,
		"agentInfo":         info,
	})

	// 2. Create session
	var newResult NewSessionResult
	if err := client.Call(ctx, MethodSessionNew, NewSessionParams{Cwd: "/tmp", McpServers: []McpServer{}}, &newResult); err != nil {
		t.Fatalf("session/new failed: %v", err)
	}
	sessionID := newResult.SessionID
	t.Logf("Session created: %s", sessionID)

	// 3. Subscribe and send prompt
	sub := client.Subscribe()
	var notifications []json.RawMessage
	notifDone := make(chan struct{})
	go func() {
		for n := range sub {
			b, _ := json.Marshal(n)
			notifications = append(notifications, b)
			t.Logf("  notification: method=%s", n.Method)
		}
		close(notifDone)
	}()

	var promptResult PromptResult
	promptErr := client.Call(ctx, MethodSessionPrompt, PromptParams{
		SessionID: sessionID,
		Prompt:    []ContentBlock{{Type: "text", Text: "What is 2+2? Reply with just the number."}},
	}, &promptResult)

	if promptErr != nil {
		t.Errorf("session/prompt failed: %v", promptErr)
	} else {
		t.Logf("Prompt completed: stopReason=%s", promptResult.StopReason)
		writeFixture(t, filepath.Join(outDir, "prompt_response.json"), promptResult)
	}

	client.Unsubscribe(sub)
	<-notifDone

	// Classify notifications by sessionUpdate type
	var classified []map[string]any
	for _, raw := range notifications {
		var n struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		json.Unmarshal(raw, &n)
		if n.Method == MethodSessionUpdate {
			su, _ := DecodeSessionUpdate(n.Params)
			entry := map[string]any{
				"sessionUpdate": su.SessionUpdate,
			}
			classified = append(classified, entry)
		} else {
			classified = append(classified, map[string]any{"method": n.Method})
		}
	}
	writeFixture(t, filepath.Join(outDir, "notifications_summary.json"), classified)
	t.Logf("Captured %d notifications", len(notifications))

	// 4. Test session/load (task 1.2)
	if caps.LoadSession {
		t.Log("Agent supports loadSession")

		var loadResult LoadSessionResult
		loadErr := client.Call(ctx, MethodSessionLoad, LoadSessionParams{
			SessionID:  sessionID,
			Cwd:       "/tmp",
			McpServers: []McpServer{},
		}, &loadResult)
		if loadErr != nil {
			t.Logf("session/load (valid) error: %v (type %T)", loadErr, loadErr)
			errObj := map[string]any{"error": loadErr.Error()}
			if rpcErr, ok := loadErr.(*RPCError); ok {
				errObj["code"] = rpcErr.Code
				errObj["message"] = rpcErr.Message
			}
			writeFixture(t, filepath.Join(outDir, "load_valid_error.json"), errObj)
		} else {
			t.Logf("session/load (valid) OK: sessionId=%s", loadResult.SessionID)
			writeFixture(t, filepath.Join(outDir, "load_valid_response.json"), loadResult)
		}

		// Unknown session ID
		var badLoadResult LoadSessionResult
		badLoadErr := client.Call(ctx, MethodSessionLoad, LoadSessionParams{
			SessionID:  "nonexistent-session-12345",
			Cwd:       "/tmp",
			McpServers: []McpServer{},
		}, &badLoadResult)
		if badLoadErr != nil {
			t.Logf("session/load (unknown) error: %v (type: %T)", badLoadErr, badLoadErr)
			errObj := map[string]any{"error": badLoadErr.Error()}
			if rpcErr, ok := badLoadErr.(*RPCError); ok {
				errObj["code"] = rpcErr.Code
				errObj["message"] = rpcErr.Message
				errObj["data"] = string(rpcErr.Data)
			}
			writeFixture(t, filepath.Join(outDir, "load_unknown_error.json"), errObj)
		} else {
			t.Logf("session/load (unknown) unexpectedly succeeded: %s", badLoadResult.SessionID)
			writeFixture(t, filepath.Join(outDir, "load_unknown_response.json"), badLoadResult)
		}
	} else {
		t.Log("Agent does NOT support loadSession")
	}
}

func writeFixture(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal error for %s: %v", path, err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write error for %s: %v", path, err)
	}
}
