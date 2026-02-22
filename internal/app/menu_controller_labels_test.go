package app

import "testing"

func TestMenuWorkspaceSubmenuUsesEditWorkspaceLabel(t *testing.T) {
	menu := NewMenuController()
	menu.submenuKind = submenuWorkspaces
	labels := menu.submenuLabels()
	foundEdit := false
	foundRename := false
	for _, label := range labels {
		if label == "Edit Workspace" {
			foundEdit = true
		}
		if label == "Rename Workspace" {
			foundRename = true
		}
	}
	if !foundEdit {
		t.Fatalf("expected workspace submenu to include Edit Workspace label")
	}
	if foundRename {
		t.Fatalf("did not expect legacy Rename Workspace label in submenu")
	}
}
