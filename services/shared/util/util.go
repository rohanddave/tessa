package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"net/url"
	"path"

	"github.com/google/uuid"
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

func GenerateUUID() string {
	return uuid.New().String()
}


func DeriveRawStorageURL(fileURL string) (string, error) {
	parsedURL, err := url.Parse(fileURL)
	if err != nil {
		return "", fmt.Errorf("parse file url %q: %w", fileURL, err)
	}

	dirPath := path.Dir(parsedURL.Path)
	if dirPath == "." || dirPath == "/" {
		return "", fmt.Errorf("file url %q does not contain a directory path", fileURL)
	}

	parsedURL.Path = dirPath
	return parsedURL.String(), nil
}
