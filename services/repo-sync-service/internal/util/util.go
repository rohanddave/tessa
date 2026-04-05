package util

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

func EnvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func HashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func HashString(s string) string {
	return HashContent([]byte(s))
}
