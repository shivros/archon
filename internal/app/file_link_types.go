package app

import (
	"context"
	"errors"
	"strings"
	"time"
)

const fileLinkOpenTimeout = 4 * time.Second

var (
	errFileLinkEmptyTarget       = errors.New("link target is empty")
	errFileLinkUnsupportedTarget = errors.New("unsupported link target")
)

type ResolvedFileLink struct {
	RawTarget string
	Kind      FileLinkTargetKind
	FilePath  string
	URL       string
	Line      int
	Column    int
}

type FileLinkTargetKind string

const (
	FileLinkTargetKindUnknown FileLinkTargetKind = ""
	FileLinkTargetKindFile    FileLinkTargetKind = "file"
	FileLinkTargetKindURL     FileLinkTargetKind = "url"
)

func (t ResolvedFileLink) OpenTarget() string {
	switch t.Kind {
	case FileLinkTargetKindURL:
		return strings.TrimSpace(t.URL)
	default:
		return strings.TrimSpace(t.FilePath)
	}
}

type FileLinkResolver interface {
	Resolve(rawTarget string) (ResolvedFileLink, error)
}

type FileLinkOpener interface {
	Open(context.Context, ResolvedFileLink) error
}

type FileLinkOpenCommand struct {
	Name string
	Args []string
}

type FileLinkOpenPolicy interface {
	BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error)
}

type FileLinkCommandRunner interface {
	Run(ctx context.Context, command FileLinkOpenCommand) error
}
