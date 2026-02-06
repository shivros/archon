package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"control/internal/types"
)

func (c *Client) TailStream(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error) {
	if stream == "" {
		stream = "combined"
	}
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("%s/v1/sessions/%s/tail?follow=1&stream=%s", c.baseURL, id, stream)
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
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan types.LogEvent, 256)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

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

func (c *Client) EventStream(ctx context.Context, id string) (<-chan types.CodexEvent, func(), error) {
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("%s/v1/sessions/%s/events?follow=1", c.baseURL, id)
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
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan types.CodexEvent, 256)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

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

func (c *Client) ItemsStream(ctx context.Context, id string) (<-chan map[string]any, func(), error) {
	if err := c.ensureToken(); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("%s/v1/sessions/%s/items?follow=1", c.baseURL, id)
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
		return nil, nil, decodeAPIError(resp)
	}

	ch := make(chan map[string]any, 256)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

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
