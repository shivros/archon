package app

import "testing"

type testSidebarThreadClassificationPolicy struct {
	isThread bool
}

func (p testSidebarThreadClassificationPolicy) IsThreadTarget(*sidebarItem) bool {
	return p.isThread
}

func TestDefaultSidebarThreadClassificationPolicy(t *testing.T) {
	policy := defaultSidebarThreadClassificationPolicy{}
	if !policy.IsThreadTarget(&sidebarItem{kind: sidebarSession}) {
		t.Fatalf("expected session item to be classified as thread target")
	}
	if policy.IsThreadTarget(&sidebarItem{kind: sidebarWorkflow}) {
		t.Fatalf("expected workflow item not to be classified as thread target by default")
	}
	if policy.IsThreadTarget(nil) {
		t.Fatalf("expected nil item not to be classified as thread target")
	}
}

func TestWithSidebarThreadClassificationPolicyOption(t *testing.T) {
	custom := testSidebarThreadClassificationPolicy{isThread: true}
	m := NewModel(nil, WithSidebarThreadClassificationPolicy(custom))

	got, ok := m.sidebarThreadClassificationPolicy.(testSidebarThreadClassificationPolicy)
	if !ok {
		t.Fatalf("expected custom thread classification policy, got %T", m.sidebarThreadClassificationPolicy)
	}
	if !got.isThread {
		t.Fatalf("expected custom thread classification policy state preserved")
	}

	WithSidebarThreadClassificationPolicy(nil)(&m)
	if _, ok := m.sidebarThreadClassificationPolicy.(defaultSidebarThreadClassificationPolicy); !ok {
		t.Fatalf("expected nil option to restore default thread classification policy, got %T", m.sidebarThreadClassificationPolicy)
	}
}

func TestWithSidebarThreadClassificationPolicyNilModelNoop(t *testing.T) {
	WithSidebarThreadClassificationPolicy(testSidebarThreadClassificationPolicy{isThread: true})(nil)
}

func TestSidebarThreadClassificationPolicyOrDefaultNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.sidebarThreadClassificationPolicyOrDefault().(defaultSidebarThreadClassificationPolicy); !ok {
		t.Fatalf("expected nil model to return default thread classification policy")
	}
}
