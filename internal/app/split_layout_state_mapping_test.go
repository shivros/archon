package app

import (
	"testing"

	"control/internal/types"
)

func TestSplitPreferenceMappingRoundTrip(t *testing.T) {
	in := &types.AppStateSplitPreference{Columns: 29, Ratio: 0.42}
	mapped := fromAppStateSplit(in)
	out := toAppStateSplit(mapped)
	if out == nil {
		t.Fatalf("expected mapped output")
	}
	if out.Columns != in.Columns || out.Ratio != in.Ratio {
		t.Fatalf("expected round trip to preserve values, in=%#v out=%#v", in, out)
	}
}

func TestToAppStateSplitNilReturnsNil(t *testing.T) {
	if got := toAppStateSplit(nil); got != nil {
		t.Fatalf("expected nil output for nil input, got %#v", got)
	}
}
