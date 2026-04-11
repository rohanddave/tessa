package config

import (
	sharedstorage "github.com/rohandave/tessa-rag/services/shared/blobstore"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"

	sharedpostgres "github.com/rohandave/tessa-rag/services/shared/postgres"
)

type Config struct {
	ServiceName string
	Port        string
	LogLevel    string
	Kafka       *sharedkafka.KafkaConfig
	GitHub      *GitHubConfig
	Database    *sharedpostgres.DatabaseConfig
	Storage     *sharedstorage.BlobStorageConfig
}

type GitHubConfig struct {
	Token string
}

func Load() Config {
	return Config{
		ServiceName: sharedutil.EnvOrDefault("SERVICE_NAME", "repo-sync-service"),
		Port:        sharedutil.EnvOrDefault("PORT", "8081"),
		LogLevel:    sharedutil.EnvOrDefault("LOG_LEVEL", "info"),
		Kafka:       sharedkafka.LoadConfig(),
		GitHub: &GitHubConfig{
			Token: sharedutil.EnvOrDefault("GITHUB_TOKEN", ""),
		},
		Database: sharedpostgres.LoadConfig(),
		Storage:  sharedstorage.LoadConfig(),
	}
}
