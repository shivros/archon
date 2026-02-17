package store

import (
	"context"
	"errors"
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
		out = append(out, cloneWorkflowTemplate(template))
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
		copy := cloneWorkflowTemplate(template)
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
	out := cloneWorkflowTemplate(normalized)
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
	file := &workflowTemplateFile{}
	if err := readJSON(s.path, file); err != nil {
		return nil, err
	}
	if file.Version == 0 {
		file.Version = workflowTemplateSchemaVersion
	}
	if file.Templates == nil {
		file.Templates = []guidedworkflows.WorkflowTemplate{}
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
	template.ID = strings.TrimSpace(template.ID)
	template.Name = strings.TrimSpace(template.Name)
	template.Description = strings.TrimSpace(template.Description)
	if template.ID == "" {
		return guidedworkflows.WorkflowTemplate{}, errors.New("template id is required")
	}
	if template.Name == "" {
		return guidedworkflows.WorkflowTemplate{}, errors.New("template name is required")
	}
	if len(template.Phases) == 0 {
		return guidedworkflows.WorkflowTemplate{}, errors.New("template phases are required")
	}
	stepIDs := map[string]struct{}{}
	for pIdx := range template.Phases {
		phase := &template.Phases[pIdx]
		phase.ID = strings.TrimSpace(phase.ID)
		phase.Name = strings.TrimSpace(phase.Name)
		if phase.ID == "" {
			return guidedworkflows.WorkflowTemplate{}, errors.New("phase id is required")
		}
		if phase.Name == "" {
			return guidedworkflows.WorkflowTemplate{}, errors.New("phase name is required")
		}
		if len(phase.Steps) == 0 {
			return guidedworkflows.WorkflowTemplate{}, errors.New("phase steps are required")
		}
		for sIdx := range phase.Steps {
			step := &phase.Steps[sIdx]
			step.ID = strings.TrimSpace(step.ID)
			step.Name = strings.TrimSpace(step.Name)
			step.Prompt = strings.TrimSpace(step.Prompt)
			if step.ID == "" {
				return guidedworkflows.WorkflowTemplate{}, errors.New("step id is required")
			}
			if step.Name == "" {
				return guidedworkflows.WorkflowTemplate{}, errors.New("step name is required")
			}
			if step.Prompt == "" {
				return guidedworkflows.WorkflowTemplate{}, errors.New("step prompt is required")
			}
			if _, exists := stepIDs[step.ID]; exists {
				return guidedworkflows.WorkflowTemplate{}, errors.New("duplicate step id: " + step.ID)
			}
			stepIDs[step.ID] = struct{}{}
		}
	}
	return template, nil
}

func cloneWorkflowTemplate(template guidedworkflows.WorkflowTemplate) guidedworkflows.WorkflowTemplate {
	out := template
	out.Phases = make([]guidedworkflows.WorkflowTemplatePhase, len(template.Phases))
	for idx, phase := range template.Phases {
		outPhase := phase
		outPhase.Steps = append([]guidedworkflows.WorkflowTemplateStep{}, phase.Steps...)
		out.Phases[idx] = outPhase
	}
	return out
}
