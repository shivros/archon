package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"control/internal/types"

	"log"
)

func streamDebugEnabled() bool {
	return strings.TrimSpace(os.Getenv("CONTROL_STREAM_DEBUG")) == "1"
}

var (
	streamLogger     *log.Logger
	streamLoggerOnce sync.Once
)

func streamDebugLogger() *log.Logger {
	if !streamDebugEnabled() {
		return nil
	}
	streamLoggerOnce.Do(func() {
		path := ""
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = filepath.Join(home, ".control", "ui-stream.log")
		}
		if path == "" {
			path = filepath.Join(os.TempDir(), "control-ui-stream.log")
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			streamLogger = log.New(os.Stderr, "ui-stream ", log.LstdFlags)
			return
		}
		streamLogger = log.New(file, "ui-stream ", log.LstdFlags)
	})
	return streamLogger
}

func streamLogf(format string, args ...any) {
	logger := streamDebugLogger()
	if logger == nil {
		return
	}
	logger.Printf(format, args...)
}

func (c *Client) TailStream(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error) {
	if stream == "" {
		stream = "combined"
	}
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("%s/v1/sessions/%s/tail?follow=1&stream=%s", c.baseURL, id, stream)
	if streamDebugEnabled() {
		streamLogf("stream tail open id=%s stream=%s url=%s", id, stream, url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "text/event-stream")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		cancel()
		if streamDebugEnabled() {
			streamLogf("stream tail error id=%s status=%d", id, resp.StatusCode)
		}
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan types.LogEvent, 256)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		start := time.Now()
		count := 0
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var dataLines []string

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if len(dataLines) == 0 {
					continue
				}
				payload := strings.Join(dataLines, "\n")
				dataLines = dataLines[:0]
				var event types.LogEvent
				if err := json.Unmarshal([]byte(payload), &event); err == nil {
					select {
					case ch <- event:
					default:
					}
					count++
					if count == 1 && streamDebugEnabled() {
						streamLogf("stream tail first id=%s stream=%s", id, stream)
					}
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
			}
		}
		if err := scanner.Err(); err != nil && streamDebugEnabled() {
			streamLogf("stream tail scan error id=%s stream=%s err=%v", id, stream, err)
		}
		if streamDebugEnabled() {
			streamLogf("stream tail close id=%s stream=%s count=%d dur=%s", id, stream, count, time.Since(start))
		}
	}()

	return ch, cancel, nil
}

func (c *Client) EventStream(ctx context.Context, id string) (<-chan types.CodexEvent, func(), error) {
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("%s/v1/sessions/%s/events?follow=1", c.baseURL, id)
	if streamDebugEnabled() {
		streamLogf("stream events open id=%s url=%s", id, url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "text/event-stream")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		cancel()
		if streamDebugEnabled() {
			streamLogf("stream events error id=%s status=%d", id, resp.StatusCode)
		}
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan types.CodexEvent, 256)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		start := time.Now()
		count := 0
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var dataLines []string

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if len(dataLines) == 0 {
					continue
				}
				payload := strings.Join(dataLines, "\n")
				dataLines = dataLines[:0]
				var event types.CodexEvent
				if err := json.Unmarshal([]byte(payload), &event); err == nil {
					select {
					case ch <- event:
					default:
					}
					count++
					if count == 1 && streamDebugEnabled() {
						streamLogf("stream events first id=%s method=%s", id, event.Method)
					}
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
			}
		}
		if err := scanner.Err(); err != nil && streamDebugEnabled() {
			streamLogf("stream events scan error id=%s err=%v", id, err)
		}
		if streamDebugEnabled() {
			streamLogf("stream events close id=%s count=%d dur=%s", id, count, time.Since(start))
		}
	}()

	return ch, cancel, nil
}

func (c *Client) ItemsStream(ctx context.Context, id string) (<-chan map[string]any, func(), error) {
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("%s/v1/sessions/%s/items?follow=1", c.baseURL, id)
	if streamDebugEnabled() {
		streamLogf("stream items open id=%s url=%s", id, url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "text/event-stream")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		cancel()
		if streamDebugEnabled() {
			streamLogf("stream items error id=%s status=%d", id, resp.StatusCode)
		}
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan map[string]any, 256)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		start := time.Now()
		count := 0
		firstType := ""
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var dataLines []string

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if len(dataLines) == 0 {
					continue
				}
				payload := strings.Join(dataLines, "\n")
				dataLines = dataLines[:0]
				var item map[string]any
				if err := json.Unmarshal([]byte(payload), &item); err == nil {
					select {
					case ch <- item:
					default:
					}
					count++
					if count == 1 {
						if typ, _ := item["type"].(string); typ != "" {
							firstType = typ
						}
						if streamDebugEnabled() {
							streamLogf("stream items first id=%s type=%s", id, firstType)
						}
					}
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
			}
		}
		if err := scanner.Err(); err != nil && streamDebugEnabled() {
			streamLogf("stream items scan error id=%s err=%v", id, err)
		}
		if streamDebugEnabled() {
			streamLogf("stream items close id=%s count=%d first_type=%s dur=%s", id, count, firstType, time.Since(start))
		}
	}()

	return ch, cancel, nil
}
