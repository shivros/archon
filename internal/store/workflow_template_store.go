package store

import (
	"context"
	"errors"
	"fmt"
	osPkg "os"
	"sort"
	"strings"
	"sync"

	"control/internal/guidedworkflows"
)

var ErrWorkflowTemplateNotFound = errors.New("workflow template not found")

const workflowTemplateSchemaVersion = 1

type WorkflowTemplateStore interface {
	ListWorkflowTemplates(ctx context.Context) ([]guidedworkflows.WorkflowTemplate, error)
	GetWorkflowTemplate(ctx context.Context, templateID string) (*guidedworkflows.WorkflowTemplate, bool, error)
	UpsertWorkflowTemplate(ctx context.Context, template guidedworkflows.WorkflowTemplate) (*guidedworkflows.WorkflowTemplate, error)
	DeleteWorkflowTemplate(ctx context.Context, templateID string) error
}

type FileWorkflowTemplateStore struct {
	path string
	mu   sync.Mutex
}

type workflowTemplateFile struct {
	Version   int                                `json:"version"`
	Templates []guidedworkflows.WorkflowTemplate `json:"templates"`
}

func NewFileWorkflowTemplateStore(path string) *FileWorkflowTemplateStore {
	return &FileWorkflowTemplateStore{path: path}
}

func (s *FileWorkflowTemplateStore) ListWorkflowTemplates(ctx context.Context) ([]guidedworkflows.WorkflowTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, osPkg.ErrNotExist) {
			return []guidedworkflows.WorkflowTemplate{}, nil
		}
		return nil, err
	}
	out := make([]guidedworkflows.WorkflowTemplate, 0, len(file.Templates))
	for _, template := range file.Templates {
		out = append(out, guidedworkflows.CloneWorkflowTemplate(template))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *FileWorkflowTemplateStore) GetWorkflowTemplate(ctx context.Context, templateID string) (*guidedworkflows.WorkflowTemplate, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(templateID)
	if id == "" {
		return nil, false, nil
	}
	file, err := s.load()
	if err != nil {
		if errors.Is(err, osPkg.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	for _, template := range file.Templates {
		if strings.TrimSpace(template.ID) != id {
			continue
		}
		copy := guidedworkflows.CloneWorkflowTemplate(template)
		return &copy, true, nil
	}
	return nil, false, nil
}

func (s *FileWorkflowTemplateStore) UpsertWorkflowTemplate(ctx context.Context, template guidedworkflows.WorkflowTemplate) (*guidedworkflows.WorkflowTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized, err := normalizeWorkflowTemplate(template)
	if err != nil {
		return nil, err
	}
	file, err := s.load()
	if err != nil {
		if !errors.Is(err, osPkg.ErrNotExist) {
			return nil, err
		}
		file = &workflowTemplateFile{
			Version: workflowTemplateSchemaVersion,
		}
	}
	replaced := false
	for idx := range file.Templates {
		if strings.TrimSpace(file.Templates[idx].ID) != normalized.ID {
			continue
		}
		file.Templates[idx] = normalized
		replaced = true
		break
	}
	if !replaced {
		file.Templates = append(file.Templates, normalized)
	}
	if err := s.save(file); err != nil {
		return nil, err
	}
	out := guidedworkflows.CloneWorkflowTemplate(normalized)
	return &out, nil
}

func (s *FileWorkflowTemplateStore) DeleteWorkflowTemplate(ctx context.Context, templateID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(templateID)
	if id == "" {
		return ErrWorkflowTemplateNotFound
	}
	file, err := s.load()
	if err != nil {
		if errors.Is(err, osPkg.ErrNotExist) {
			return ErrWorkflowTemplateNotFound
		}
		return err
	}
	filtered := file.Templates[:0]
	found := false
	for _, template := range file.Templates {
		if strings.TrimSpace(template.ID) == id {
			found = true
			continue
		}
		filtered = append(filtered, template)
	}
	if !found {
		return ErrWorkflowTemplateNotFound
	}
	file.Templates = filtered
	return s.save(file)
}

func (s *FileWorkflowTemplateStore) load() (*workflowTemplateFile, error) {
	raw, err := osPkg.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	parsed, err := guidedworkflows.DecodeWorkflowTemplateCatalogJSON(raw)
	if err != nil {
		return nil, err
	}
	file := &workflowTemplateFile{
		Version:   parsed.Version,
		Templates: make([]guidedworkflows.WorkflowTemplate, 0, len(parsed.Templates)),
	}
	if file.Version == 0 {
		file.Version = workflowTemplateSchemaVersion
	}
	for _, template := range parsed.Templates {
		normalized, normalizeErr := normalizeWorkflowTemplate(template)
		if normalizeErr != nil {
			return nil, fmt.Errorf("invalid workflow template %q: %w", strings.TrimSpace(template.ID), normalizeErr)
		}
		file.Templates = append(file.Templates, normalized)
	}
	return file, nil
}

func (s *FileWorkflowTemplateStore) save(file *workflowTemplateFile) error {
	if file == nil {
		return errors.New("workflow template file is required")
	}
	file.Version = workflowTemplateSchemaVersion
	if file.Templates == nil {
		file.Templates = []guidedworkflows.WorkflowTemplate{}
	}
	return writeJSONAtomic(s.path, file)
}

func normalizeWorkflowTemplate(template guidedworkflows.WorkflowTemplate) (guidedworkflows.WorkflowTemplate, error) {
	return guidedworkflows.NormalizeWorkflowTemplate(template)
}
