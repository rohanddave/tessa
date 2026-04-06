package config

import (
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/util"
	sharedstorage "github.com/rohandave/tessa-rag/services/shared/blobstore"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
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
		ServiceName: util.EnvOrDefault("SERVICE_NAME", "repo-sync-service"),
		Port:        util.EnvOrDefault("PORT", "8081"),
		LogLevel:    util.EnvOrDefault("LOG_LEVEL", "info"),
		Kafka:       sharedkafka.LoadConfig(),
		GitHub: GitHubConfig{
			Token: util.EnvOrDefault("GITHUB_TOKEN", ""),
		},
		Database: DatabaseConfig{
			Host:     util.EnvOrDefault("SNAPSHOT_STORE_HOST", "localhost"),
			Port:     util.EnvOrDefault("SNAPSHOT_STORE_PORT", "5432"),
			Name:     util.EnvOrDefault("SNAPSHOT_STORE_DB", "snapshot_store"),
			User:     util.EnvOrDefault("SNAPSHOT_STORE_USER", "postgres"),
			Password: util.EnvOrDefault("SNAPSHOT_STORE_PASSWORD", "postgres"),
			SSLMode:  util.EnvOrDefault("SNAPSHOT_STORE_SSLMODE", "disable"),
		},
		Storage: sharedstorage.LoadConfig(),
	}
}
