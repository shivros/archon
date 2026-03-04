package app

import (
	"context"
	"errors"
	"time"
)

const fileLinkOpenTimeout = 4 * time.Second

var (
	errFileLinkEmptyTarget       = errors.New("link target is empty")
	errFileLinkUnsupportedTarget = errors.New("unsupported link target")
)

type ResolvedFileLink struct {
	RawTarget string
	Path      string
	Line      int
	Column    int
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
