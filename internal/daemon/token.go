package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

func LoadOrCreateToken(tokenPath string) (string, error) {
	if token, err := readToken(tokenPath); err == nil && token != "" {
		_ = os.Chmod(tokenPath, 0o600)
		return token, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	token, err := generateToken()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	_ = os.Chmod(tokenPath, 0o600)
	return token, nil
}

func readToken(tokenPath string) (string, error) {
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
