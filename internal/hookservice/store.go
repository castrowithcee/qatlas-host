package hookservice

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// NewSecret returns a fresh secret: 32 random bytes as base64url without padding, the form the receiver accepts.
func NewSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashSecret is the stored form of a secret, the SHA-256 of its text, as the receiver compares it.
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// NewID returns an unused hook id of the form h-<8 hex>.
func (c Config) NewID() (string, error) {
	for range 16 {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("random id: %w", err)
		}
		id := "h-" + hex.EncodeToString(b)
		if !c.hasID(id) {
			return id, nil
		}
	}
	return "", errors.New("no unused hook id found")
}

func (c Config) hasID(id string) bool {
	for _, h := range c.Hooks {
		if h.ID == id {
			return true
		}
	}
	return false
}

// Remove drops the hook with the id and reports whether there was one.
func (c *Config) Remove(id string) bool {
	for i, h := range c.Hooks {
		if h.ID == id {
			c.Hooks = append(c.Hooks[:i:i], c.Hooks[i+1:]...)
			return true
		}
	}
	return false
}
