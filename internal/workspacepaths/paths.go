package workspacepaths

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type DirChecker interface {
	Stat(name string) (os.FileInfo, error)
}

type osDirChecker struct{}

func (osDirChecker) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func OSDirChecker() DirChecker {
	return osDirChecker{}
}

func NormalizeSubpath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return "", nil
	}
	if filepath.IsAbs(cleaned) {
		return "", errors.New("workspace session subpath must be relative")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("workspace session subpath must not escape repo root")
	}
	return cleaned, nil
}

func ResolveSessionPath(rootPath, subpath string) (string, error) {
	root := strings.TrimSpace(rootPath)
	if root == "" {
		return "", errors.New("workspace path is required")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	normalized, err := NormalizeSubpath(subpath)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return filepath.Clean(rootAbs), nil
	}
	return filepath.Clean(filepath.Join(rootAbs, normalized)), nil
}

func ValidateDirectory(path string, checker DirChecker) error {
	if checker == nil {
		checker = OSDirChecker()
	}
	info, err := checker.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	return nil
}

func ValidateRootAndSessionPath(rootPath, subpath string, checker DirChecker) error {
	root := strings.TrimSpace(rootPath)
	if root == "" {
		return errors.New("workspace path is required")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if err := ValidateDirectory(rootAbs, checker); err != nil {
		return err
	}
	sessionPath, err := ResolveSessionPath(rootAbs, subpath)
	if err != nil {
		return err
	}
	if err := ValidateDirectory(sessionPath, checker); err != nil {
		return err
	}
	return nil
}
