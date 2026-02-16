package daemon

import "testing"

func TestFlattenCodexItemsWithLimit(t *testing.T) {
	thread := &codexThread{
		Turns: []codexTurn{
			{
				ID: "t1",
				Items: []map[string]any{
					{"id": "a"},
					{"id": "b"},
				},
			},
			{
				ID: "t2",
				Items: []map[string]any{
					{"id": "c"},
					{"id": "d"},
				},
			},
		},
	}

	items := flattenCodexItemsWithLimit(thread, 2)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if got, _ := items[0]["id"].(string); got != "c" {
		t.Fatalf("expected first limited item c, got %q", got)
	}
	if got, _ := items[1]["id"].(string); got != "d" {
		t.Fatalf("expected second limited item d, got %q", got)
	}
}

func TestFlattenCodexItemsWithLimitUnlimited(t *testing.T) {
	thread := &codexThread{
		Turns: []codexTurn{
			{
				ID: "t1",
				Items: []map[string]any{
					{"id": "a"},
					{"id": "b"},
				},
			},
		},
	}

	items := flattenCodexItemsWithLimit(thread, 0)
	if len(items) != 2 {
		t.Fatalf("expected all items when limit<=0, got %d", len(items))
	}
}
