package daemon

import (
	"encoding/json"
	"sort"
)

func detectStringsByKey(value any, key string) []string {
	if value == nil || key == "" {
		return nil
	}
	raw, err := toAny(value)
	if err != nil {
		return nil
	}
	set := make(map[string]struct{})
	collectStringsByKey(raw, key, set)
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for val := range set {
		out = append(out, val)
	}
	sort.Strings(out)
	return out
}

func toAny(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func collectStringsByKey(value any, key string, out map[string]struct{}) {
	switch v := value.(type) {
	case map[string]any:
		for k, val := range v {
			if k == key {
				if s, ok := val.(string); ok && s != "" {
					out[s] = struct{}{}
				}
			}
			collectStringsByKey(val, key, out)
		}
	case []any:
		for _, entry := range v {
			collectStringsByKey(entry, key, out)
		}
	}
}
