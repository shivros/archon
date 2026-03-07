package daemon

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOpenCodeDoEventStreamRequestValidation(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/event", nil)
	if _, err := openCodeDoEventStreamRequest(nil, req, context.Background(), time.Second); err == nil {
		t.Fatalf("expected error for nil http client")
	}
	client := &http.Client{}
	if _, err := openCodeDoEventStreamRequest(client, nil, context.Background(), time.Second); err == nil {
		t.Fatalf("expected error for nil request")
	}
}

func TestOpenCodeDoEventStreamRequestSuccess(t *testing.T) {
	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}),
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/event", nil)

	resp, err := openCodeDoEventStreamRequest(client, req, context.Background(), time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	_ = resp.Body.Close()
}

func TestOpenCodeDoEventStreamRequestContextCancellation(t *testing.T) {
	unblock := make(chan struct{})
	defer close(unblock)

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			<-unblock
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/event", nil)

	resp, err := openCodeDoEventStreamRequest(client, req, ctx, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
	if resp != nil {
		t.Fatalf("expected nil response on context cancellation, got %#v", resp)
	}
}

func TestOpenCodeDoEventStreamRequestTimeout(t *testing.T) {
	unblock := make(chan struct{})
	defer close(unblock)

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			<-unblock
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}),
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/event", nil)

	resp, err := openCodeDoEventStreamRequest(client, req, context.Background(), 20*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if resp != nil {
		t.Fatalf("expected nil response on timeout, got %#v", resp)
	}
}
