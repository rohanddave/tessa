package config

import (
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/util"
)

type Config struct {
	ServiceName string
	Port        string
	LogLevel    string
	Kafka       KafkaConfig
	GitHub      GitHubConfig
	Database    DatabaseConfig
	Storage     StorageConfig
}

type KafkaConfig struct {
	Brokers        string
	EventsTopic    string
	LifeCycleTopic string
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

type StorageConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          string
}

func Load() Config {
	return Config{
		ServiceName: util.EnvOrDefault("SERVICE_NAME", "repo-sync-service"),
		Port:        util.EnvOrDefault("PORT", "8081"),
		LogLevel:    util.EnvOrDefault("LOG_LEVEL", "info"),
		Kafka: KafkaConfig{
			Brokers:        util.EnvOrDefault("KAFKA_BROKERS", "localhost:19092"),
			EventsTopic:    util.EnvOrDefault("KAFKA_EVENTS_TOPIC", "repo-sync.repo-events"),
			LifeCycleTopic: util.EnvOrDefault("KAFKA_LIFE_CYCLE_TOPIC", "repo-sync.repo-lifecycle"),
		},
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
		Storage: StorageConfig{
			Endpoint:        util.EnvOrDefault("S3_ENDPOINT", "localhost:9000"),
			Region:          util.EnvOrDefault("S3_REGION", "us-east-1"),
			Bucket:          util.EnvOrDefault("S3_BUCKET", "repo-sync"),
			AccessKeyID:     util.EnvOrDefault("S3_ACCESS_KEY_ID", "minioadmin"),
			SecretAccessKey: util.EnvOrDefault("S3_SECRET_ACCESS_KEY", "minioadmin"),
			UseSSL:          util.EnvOrDefault("S3_USE_SSL", "false"),
		},
	}
}
