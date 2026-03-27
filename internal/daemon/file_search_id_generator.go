package daemon

import (
	"crypto/rand"
	"encoding/hex"
)

type FileSearchIDGenerator interface {
	NewID() (string, error)
}

type randomFileSearchIDGenerator struct{}

func NewRandomFileSearchIDGenerator() FileSearchIDGenerator {
	return randomFileSearchIDGenerator{}
}

func (randomFileSearchIDGenerator) NewID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
