package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"control/internal/config"
	"control/internal/daemon/transcriptdomain"
	"control/internal/types"

	"log"
)

func streamDebugEnabled() bool {
	streamDebugOnce.Do(func() {
		coreCfg, err := config.LoadCoreConfig()
		if err != nil {
			streamDebug = false
			return
		}
		streamDebug = coreCfg.StreamDebugEnabled()
	})
	return streamDebug
}

var (
	streamLogger     *log.Logger
	streamLoggerOnce sync.Once
	streamDebug      bool
	streamDebugOnce  sync.Once
)

func streamDebugLogger() *log.Logger {
	if !streamDebugEnabled() {
		return nil
	}
	streamLoggerOnce.Do(func() {
		path := ""
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = filepath.Join(home, ".archon", "ui-stream.log")
		}
		if path == "" {
			path = filepath.Join(os.TempDir(), "archon-ui-stream.log")
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
		defer func() { _ = resp.Body.Close() }()
		cancel()
		if streamDebugEnabled() {
			streamLogf("stream tail error id=%s status=%d", id, resp.StatusCode)
		}
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan types.LogEvent, 256)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		start := time.Now()
		count := 0
		reason := "eof"
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
		if err := scanner.Err(); err != nil {
			reason = "scan_error"
			if streamDebugEnabled() {
				streamLogf("stream tail scan error id=%s stream=%s err=%v", id, stream, err)
			}
		}
		if streamDebugEnabled() {
			streamLogf("stream tail close id=%s stream=%s reason=%s count=%d dur=%s", id, stream, reason, count, time.Since(start))
		}
	}()

	return ch, cancel, nil
}

func (c *Client) TranscriptStream(ctx context.Context, id string, afterRevision string) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}
	query := "follow=1"
	if strings.TrimSpace(afterRevision) != "" {
		query += "&after_revision=" + url.QueryEscape(strings.TrimSpace(afterRevision))
	}

	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("%s/v1/sessions/%s/transcript/stream?%s", c.baseURL, id, query)
	if streamDebugEnabled() {
		streamLogf("stream transcript open id=%s url=%s", id, url)
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
		defer func() { _ = resp.Body.Close() }()
		cancel()
		if streamDebugEnabled() {
			streamLogf("stream transcript error id=%s status=%d", id, resp.StatusCode)
		}
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan transcriptdomain.TranscriptEvent, 256)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		start := time.Now()
		count := 0
		reason := "eof"
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
				var event transcriptdomain.TranscriptEvent
				if err := json.Unmarshal([]byte(payload), &event); err == nil {
					select {
					case ch <- event:
					default:
					}
					count++
					if count == 1 && streamDebugEnabled() {
						streamLogf("stream transcript first id=%s kind=%s revision=%s", id, event.Kind, event.Revision.String())
					}
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
			}
		}
		if err := scanner.Err(); err != nil {
			reason = "scan_error"
			if streamDebugEnabled() {
				streamLogf("stream transcript scan error id=%s err=%v", id, err)
			}
		}
		if streamDebugEnabled() {
			streamLogf("stream transcript close id=%s reason=%s count=%d dur=%s", id, reason, count, time.Since(start))
		}
	}()

	return ch, cancel, nil
}

func (c *Client) FileSearchEvents(ctx context.Context, id string) (<-chan types.FileSearchEvent, func(), error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil, fmt.Errorf("file search id is required")
	}
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("%s/v1/file-searches/%s/events?follow=1", c.baseURL, id)
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
		defer func() { _ = resp.Body.Close() }()
		cancel()
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan types.FileSearchEvent, 256)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

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
				var event types.FileSearchEvent
				if err := json.Unmarshal([]byte(payload), &event); err == nil {
					select {
					case ch <- event:
					default:
					}
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
			}
		}
	}()

	return ch, cancel, nil
}

func (c *Client) DebugStream(ctx context.Context, id string) (<-chan types.DebugEvent, func(), error) {
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("%s/v1/sessions/%s/debug?follow=1&lines=200", c.baseURL, id)
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
		defer func() { _ = resp.Body.Close() }()
		cancel()
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan types.DebugEvent, 256)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

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
				var event types.DebugEvent
				if err := json.Unmarshal([]byte(payload), &event); err == nil {
					select {
					case ch <- event:
					default:
					}
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
			}
		}
	}()

	return ch, cancel, nil
}

func (c *Client) MetadataStream(ctx context.Context, afterRevision string) (<-chan types.MetadataEvent, func(), error) {
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}
	query := "follow=1"
	if strings.TrimSpace(afterRevision) != "" {
		query += "&after_revision=" + url.QueryEscape(strings.TrimSpace(afterRevision))
	}
	ctx, cancel := context.WithCancel(ctx)
	streamURL := fmt.Sprintf("%s/v1/metadata/stream?%s", c.baseURL, query)
	if streamDebugEnabled() {
		streamLogf("stream metadata open after=%s url=%s", strings.TrimSpace(afterRevision), streamURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(afterRevision) != "" {
		req.Header.Set("Last-Event-ID", strings.TrimSpace(afterRevision))
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		cancel()
		if streamDebugEnabled() {
			streamLogf("stream metadata error status=%d", resp.StatusCode)
		}
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan types.MetadataEvent, 256)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		start := time.Now()
		count := 0
		reason := "eof"
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var dataLines []string
		lastID := ""

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if len(dataLines) == 0 {
					lastID = ""
					continue
				}
				payload := strings.Join(dataLines, "\n")
				dataLines = dataLines[:0]
				var event types.MetadataEvent
				if err := json.Unmarshal([]byte(payload), &event); err == nil {
					if strings.TrimSpace(event.Revision) == "" && strings.TrimSpace(lastID) != "" {
						event.Revision = strings.TrimSpace(lastID)
					}
					select {
					case ch <- event:
					default:
					}
					count++
				}
				lastID = ""
				continue
			}
			if strings.HasPrefix(line, "id:") {
				lastID = strings.TrimSpace(line[len("id:"):])
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
			}
		}
		if err := scanner.Err(); err != nil {
			reason = "scan_error"
			if streamDebugEnabled() {
				streamLogf("stream metadata scan error err=%v", err)
			}
		}
		if streamDebugEnabled() {
			streamLogf("stream metadata close reason=%s count=%d dur=%s", reason, count, time.Since(start))
		}
	}()
	return ch, cancel, nil
}
