package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGuidedWorkflowRunMutationsUseStateTransitionGateway(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("expected runtime caller path")
	}
	dir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read app directory: %v", err)
	}

	allowed := map[string]struct{}{
		"guided_workflow_state_transition.go": {},
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, ok := allowed[name]; ok {
			continue
		}
		path := filepath.Join(dir, name)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			method := strings.TrimSpace(sel.Sel.Name)
			if method != "SetRun" && method != "SetSnapshot" {
				return true
			}
			if !isGuidedWorkflowSelector(sel.X) {
				return true
			}
			t.Fatalf("expected %s to route %s mutations through GuidedWorkflowStateTransitionGateway", name, method)
			return false
		})
	}
}

func isGuidedWorkflowSelector(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return strings.TrimSpace(selector.Sel.Name) == "guidedWorkflow"
}
