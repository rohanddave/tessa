package config

import (
	sharedstorage "github.com/rohandave/tessa-rag/services/shared/blobstore"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
)

type Config struct {
	ServiceName string
	Port        string
	LogLevel    string
	Kafka       sharedkafka.Config
	GitHub      GitHubConfig
	Database    DatabaseConfig
	Storage     sharedstorage.Config
}

type GitHubConfig struct {
	Token string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

func Load() Config {
	return Config{
		ServiceName: sharedutil.EnvOrDefault("SERVICE_NAME", "repo-sync-service"),
		Port:        sharedutil.EnvOrDefault("PORT", "8081"),
		LogLevel:    sharedutil.EnvOrDefault("LOG_LEVEL", "info"),
		Kafka:       sharedkafka.LoadConfig(),
		GitHub: GitHubConfig{
			Token: sharedutil.EnvOrDefault("GITHUB_TOKEN", ""),
		},
		Database: DatabaseConfig{
			Host:     sharedutil.EnvOrDefault("SNAPSHOT_STORE_HOST", "localhost"),
			Port:     sharedutil.EnvOrDefault("SNAPSHOT_STORE_PORT", "5432"),
			Name:     sharedutil.EnvOrDefault("SNAPSHOT_STORE_DB", "snapshot_store"),
			User:     sharedutil.EnvOrDefault("SNAPSHOT_STORE_USER", "postgres"),
			Password: sharedutil.EnvOrDefault("SNAPSHOT_STORE_PASSWORD", "postgres"),
			SSLMode:  sharedutil.EnvOrDefault("SNAPSHOT_STORE_SSLMODE", "disable"),
		},
		Storage: sharedstorage.LoadConfig(),
	}
}
