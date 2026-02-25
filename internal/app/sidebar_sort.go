package app

import (
	"sort"
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

type sidebarSortKey string

const (
	sidebarSortKeyActivity sidebarSortKey = "activity"
	sidebarSortKeyCreated  sidebarSortKey = "created"
	sidebarSortKeyName     sidebarSortKey = "name"
)

type sidebarSortState struct {
	Key     sidebarSortKey
	Reverse bool
}

type sidebarSortWorkspaceContext struct {
	ActivityByWorkspaceID map[string]time.Time
}

type SidebarSortKeySpec struct {
	Key   sidebarSortKey
	Label string
	Less  func(ctx sidebarSortWorkspaceContext, left, right *types.Workspace) bool
}

var (
	sidebarSortRegistryMu sync.RWMutex
	sidebarSortSpecs      = map[sidebarSortKey]SidebarSortKeySpec{}
	sidebarSortOrder      = []sidebarSortKey{}
)

func init() {
	mustRegisterDefaultSidebarSortSpecs()
}

func mustRegisterDefaultSidebarSortSpecs() {
	RegisterSidebarSortKey(SidebarSortKeySpec{
		Key:   sidebarSortKeyActivity,
		Label: "Activity",
		Less: func(ctx sidebarSortWorkspaceContext, left, right *types.Workspace) bool {
			if left == nil || right == nil {
				return left != nil
			}
			leftActivity := ctx.ActivityByWorkspaceID[strings.TrimSpace(left.ID)]
			rightActivity := ctx.ActivityByWorkspaceID[strings.TrimSpace(right.ID)]
			if leftActivity.Equal(rightActivity) {
				return sidebarWorkspaceNameLess(left, right)
			}
			return leftActivity.After(rightActivity)
		},
	})
	RegisterSidebarSortKey(SidebarSortKeySpec{
		Key:   sidebarSortKeyCreated,
		Label: "Created",
		Less: func(_ sidebarSortWorkspaceContext, left, right *types.Workspace) bool {
			if left == nil || right == nil {
				return left != nil
			}
			if left.CreatedAt.Equal(right.CreatedAt) {
				return sidebarWorkspaceNameLess(left, right)
			}
			return left.CreatedAt.Before(right.CreatedAt)
		},
	})
	RegisterSidebarSortKey(SidebarSortKeySpec{
		Key:   sidebarSortKeyName,
		Label: "Name",
		Less: func(_ sidebarSortWorkspaceContext, left, right *types.Workspace) bool {
			if left == nil || right == nil {
				return left != nil
			}
			leftName := sidebarWorkspaceName(left)
			rightName := sidebarWorkspaceName(right)
			if leftName == rightName {
				if left.CreatedAt.Equal(right.CreatedAt) {
					return strings.TrimSpace(left.ID) < strings.TrimSpace(right.ID)
				}
				return left.CreatedAt.Before(right.CreatedAt)
			}
			return leftName < rightName
		},
	})
}

func RegisterSidebarSortKey(spec SidebarSortKeySpec) {
	key := sidebarSortKey(strings.ToLower(strings.TrimSpace(string(spec.Key))))
	if key == "" {
		return
	}
	spec.Key = key
	label := strings.TrimSpace(spec.Label)
	if label == "" {
		label = strings.ToUpper(string(key[:1])) + string(key[1:])
	}
	spec.Label = label
	if spec.Less == nil {
		spec.Less = func(_ sidebarSortWorkspaceContext, left, right *types.Workspace) bool {
			if left == nil || right == nil {
				return left != nil
			}
			return sidebarWorkspaceNameLess(left, right)
		}
	}

	sidebarSortRegistryMu.Lock()
	defer sidebarSortRegistryMu.Unlock()
	if _, exists := sidebarSortSpecs[key]; !exists {
		sidebarSortOrder = append(sidebarSortOrder, key)
	}
	sidebarSortSpecs[key] = spec
}

func defaultSidebarSortState() sidebarSortState {
	return sidebarSortState{Key: sidebarSortKeyCreated}
}

func parseSidebarSortKey(raw string) sidebarSortKey {
	key := sidebarSortKey(strings.ToLower(strings.TrimSpace(raw)))
	sidebarSortRegistryMu.RLock()
	defer sidebarSortRegistryMu.RUnlock()
	if _, ok := sidebarSortSpecs[key]; ok {
		return key
	}
	return sidebarSortKeyCreated
}

func sidebarSortLabel(key sidebarSortKey) string {
	key = parseSidebarSortKey(string(key))
	sidebarSortRegistryMu.RLock()
	defer sidebarSortRegistryMu.RUnlock()
	if spec, ok := sidebarSortSpecs[key]; ok {
		return spec.Label
	}
	return "Created"
}

func cycleSidebarSortKey(current sidebarSortKey, step int) sidebarSortKey {
	sidebarSortRegistryMu.RLock()
	order := append([]sidebarSortKey(nil), sidebarSortOrder...)
	sidebarSortRegistryMu.RUnlock()
	if len(order) == 0 {
		return sidebarSortKeyCreated
	}
	current = parseSidebarSortKey(string(current))
	idx := 0
	for i, key := range order {
		if key == current {
			idx = i
			break
		}
	}
	if step == 0 {
		return order[idx]
	}
	next := (idx + step) % len(order)
	if next < 0 {
		next += len(order)
	}
	return order[next]
}

func sidebarSortLess(key sidebarSortKey, ctx sidebarSortWorkspaceContext, left, right *types.Workspace) bool {
	key = parseSidebarSortKey(string(key))
	sidebarSortRegistryMu.RLock()
	spec, ok := sidebarSortSpecs[key]
	sidebarSortRegistryMu.RUnlock()
	if !ok || spec.Less == nil {
		return sidebarWorkspaceNameLess(left, right)
	}
	return spec.Less(ctx, left, right)
}

func sidebarWorkspaceNameLess(left, right *types.Workspace) bool {
	leftName := sidebarWorkspaceName(left)
	rightName := sidebarWorkspaceName(right)
	if leftName == rightName {
		return strings.TrimSpace(left.ID) < strings.TrimSpace(right.ID)
	}
	return leftName < rightName
}

func sidebarWorkspaceName(workspace *types.Workspace) string {
	if workspace == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(workspace.Name))
}

func sortedSidebarSortKeys() []sidebarSortKey {
	sidebarSortRegistryMu.RLock()
	defer sidebarSortRegistryMu.RUnlock()
	out := append([]sidebarSortKey(nil), sidebarSortOrder...)
	if len(out) == 0 {
		return []sidebarSortKey{sidebarSortKeyCreated}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func formatKeyBadge(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "+")
	for i, part := range parts {
		token := strings.TrimSpace(part)
		lower := strings.ToLower(token)
		switch lower {
		case "ctrl":
			parts[i] = "Ctrl"
		case "alt":
			parts[i] = "Alt"
		case "shift":
			parts[i] = "Shift"
		case "cmd":
			parts[i] = "Cmd"
		case "left":
			parts[i] = "Left"
		case "right":
			parts[i] = "Right"
		case "up":
			parts[i] = "Up"
		case "down":
			parts[i] = "Down"
		case "space":
			parts[i] = "Space"
		case "enter":
			parts[i] = "Enter"
		case "backspace":
			parts[i] = "Backspace"
		case "delete":
			parts[i] = "Delete"
		case "tab":
			parts[i] = "Tab"
		case "esc":
			parts[i] = "Esc"
		default:
			if len(token) == 1 {
				parts[i] = strings.ToUpper(token)
			} else {
				parts[i] = strings.ToUpper(token[:1]) + token[1:]
			}
		}
	}
	return "[" + strings.Join(parts, "+") + "]"
}
