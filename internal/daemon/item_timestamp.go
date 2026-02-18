package daemon

import (
	"strconv"
	"strings"
	"time"
)

type itemTimestampClassification struct {
	HasProviderTimestamp bool
	UsedDaemonTimestamp  bool
}

func prepareItemForPersistence(item map[string]any, now time.Time) map[string]any {
	prepared, _ := prepareItemForPersistenceWithClassification(item, now)
	return prepared
}

func prepareItemForPersistenceWithClassification(item map[string]any, now time.Time) (map[string]any, itemTimestampClassification) {
	if item == nil {
		return nil, itemTimestampClassification{}
	}
	classification := itemTimestampClassification{
		HasProviderTimestamp: !parsePersistedTimestamp(item["provider_created_at"]).IsZero(),
	}
	prepared := cloneItemMap(item)
	createdAt := resolveItemCreatedAt(prepared)
	if createdAt.IsZero() {
		if now.IsZero() {
			now = time.Now().UTC()
		}
		createdAt = now.UTC()
		classification.UsedDaemonTimestamp = true
	} else {
		createdAt = createdAt.UTC()
	}
	prepared["created_at"] = createdAt.Format(time.RFC3339Nano)
	return prepared, classification
}

func cloneItemMap(item map[string]any) map[string]any {
	cloned := make(map[string]any, len(item))
	for k, v := range item {
		cloned[k] = v
	}
	return cloned
}

func resolveItemCreatedAt(item map[string]any) time.Time {
	if item == nil {
		return time.Time{}
	}
	for _, key := range []string{"provider_created_at", "created_at", "createdAt", "ts", "timestamp", "created"} {
		if when := parsePersistedTimestamp(item[key]); !when.IsZero() {
			return when
		}
	}
	if message, ok := item["message"].(map[string]any); ok && message != nil {
		for _, key := range []string{"created_at", "createdAt", "ts", "timestamp", "created"} {
			if when := parsePersistedTimestamp(message[key]); !when.IsZero() {
				return when
			}
		}
	}
	if info, ok := item["info"].(map[string]any); ok && info != nil {
		for _, key := range []string{"created_at", "createdAt", "ts", "timestamp", "created"} {
			if when := parsePersistedTimestamp(info[key]); !when.IsZero() {
				return when
			}
		}
	}
	if clock, ok := item["time"].(map[string]any); ok && clock != nil {
		for _, key := range []string{"created", "created_at", "createdAt", "ts", "timestamp"} {
			if when := parsePersistedTimestamp(clock[key]); !when.IsZero() {
				return when
			}
		}
	}
	return time.Time{}
}

func parsePersistedTimestamp(raw any) time.Time {
	parseUnix := func(value int64) time.Time {
		switch {
		case value >= 1_000_000_000_000_000_000:
			return time.Unix(0, value).UTC()
		case value >= 1_000_000_000_000_000:
			return time.UnixMicro(value).UTC()
		case value >= 1_000_000_000_000:
			return time.UnixMilli(value).UTC()
		case value > 0:
			return time.Unix(value, 0).UTC()
		default:
			return time.Time{}
		}
	}
	switch typed := raw.(type) {
	case time.Time:
		return typed.UTC()
	case string:
		value := strings.TrimSpace(typed)
		if value == "" {
			return time.Time{}
		}
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC()
		}
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parseUnix(n)
		}
	case int64:
		return parseUnix(typed)
	case int:
		return parseUnix(int64(typed))
	case float64:
		return parseUnix(int64(typed))
	case jsonNumberLike:
		if n, err := typed.Int64(); err == nil {
			return parseUnix(n)
		}
	}
	return time.Time{}
}

type jsonNumberLike interface {
	Int64() (int64, error)
}
