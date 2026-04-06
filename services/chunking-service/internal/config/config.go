package config

import (
	"os"

	sharedstorage "github.com/rohandave/tessa-rag/services/shared/blobstore"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
)

type Config struct {
	ServiceName string
	Port        string
	Storage     sharedstorage.Config
	Kafka       sharedkafka.Config
}

func Load() Config {
	return Config{
		ServiceName: getEnv("SERVICE_NAME", "chunking-service"),
		Port:        getEnv("PORT", "8082"),
		Storage:     sharedstorage.LoadConfig(),
		Kafka:       sharedkafka.LoadConfig(),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
