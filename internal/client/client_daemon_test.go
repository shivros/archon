package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEnsureDaemonRestartsWhenConfigSignatureChanges(t *testing.T) {
	origStart := startBackgroundDaemonFn
	origSignature := currentConfigSignatureFn
	t.Cleanup(func() {
		startBackgroundDaemonFn = origStart
		currentConfigSignatureFn = origSignature
	})

	currentConfigSignatureFn = func() string { return "cfg-new" }

	var startCalls int
	startBackgroundDaemonFn = func() error {
		startCalls++
		return nil
	}

	var calls []string
	c := &Client{
		baseURL: "http://daemon.test",
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls = append(calls, req.Method+" "+req.URL.Path)
				switch len(calls) {
				case 1:
					return jsonResponse(http.StatusOK, `{"ok":true,"version":"v1","pid":123,"config_signature":"cfg-old"}`), nil
				case 2:
					if req.URL.Path != "/v1/shutdown" {
						t.Fatalf("expected shutdown request, got %s", req.URL.Path)
					}
					return jsonResponse(http.StatusOK, `{}`), nil
				case 3:
					return nil, errors.New("connection refused")
				case 4:
					return jsonResponse(http.StatusOK, `{"ok":true,"version":"v1","pid":456,"config_signature":"cfg-new"}`), nil
				default:
					t.Fatalf("unexpected request %d: %s %s", len(calls), req.Method, req.URL.Path)
					return nil, nil
				}
			}),
		},
	}

	if err := c.EnsureDaemon(context.Background()); err != nil {
		t.Fatalf("EnsureDaemon error: %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("expected daemon restart, got %d starts", startCalls)
	}
	if got := strings.Join(calls, ", "); got != "GET /health, POST /v1/shutdown, GET /health, GET /health" {
		t.Fatalf("unexpected request sequence: %s", got)
	}
}

func TestEnsureDaemonSkipsRestartWhenConfigSignatureMatches(t *testing.T) {
	origStart := startBackgroundDaemonFn
	origSignature := currentConfigSignatureFn
	t.Cleanup(func() {
		startBackgroundDaemonFn = origStart
		currentConfigSignatureFn = origSignature
	})

	currentConfigSignatureFn = func() string { return "cfg-same" }

	var startCalls int
	startBackgroundDaemonFn = func() error {
		startCalls++
		return nil
	}

	var calls []string
	c := &Client{
		baseURL: "http://daemon.test",
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls = append(calls, req.Method+" "+req.URL.Path)
				return jsonResponse(http.StatusOK, `{"ok":true,"version":"v1","pid":123,"config_signature":"cfg-same"}`), nil
			}),
		},
	}

	if err := c.EnsureDaemon(context.Background()); err != nil {
		t.Fatalf("EnsureDaemon error: %v", err)
	}
	if startCalls != 0 {
		t.Fatalf("expected no daemon restart, got %d starts", startCalls)
	}
	if got := strings.Join(calls, ", "); got != "GET /health" {
		t.Fatalf("unexpected request sequence: %s", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
