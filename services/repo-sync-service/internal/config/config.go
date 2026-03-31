package config

import "os"

type Config struct {
	ServiceName string
	Port        string
	LogLevel    string
	Kafka       KafkaConfig
	Storage     StorageConfig
}

type KafkaConfig struct {
	Brokers string
	EventsTopic   string
	LifeCycleTopic string
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
		ServiceName: envOrDefault("SERVICE_NAME", "repo-sync-service"),
		Port:        envOrDefault("PORT", "8081"),
		LogLevel:    envOrDefault("LOG_LEVEL", "info"),
		Kafka: KafkaConfig{
			Brokers: envOrDefault("KAFKA_BROKERS", "localhost:19092"),
			EventsTopic:   envOrDefault("KAFKA_EVENTS_TOPIC", "repo-sync.repo-events"),
			LifeCycleTopic: envOrDefault("KAFKA_LIFE_CYCLE_TOPIC", "repo-sync.repo-lifecycle"),
		},
		Storage: StorageConfig{
			Endpoint:        envOrDefault("S3_ENDPOINT", "localhost:9000"),
			Region:          envOrDefault("S3_REGION", "us-east-1"),
			Bucket:          envOrDefault("S3_BUCKET", "repo-sync"),
			AccessKeyID:     envOrDefault("S3_ACCESS_KEY_ID", "minioadmin"),
			SecretAccessKey: envOrDefault("S3_SECRET_ACCESS_KEY", "minioadmin"),
			UseSSL:          envOrDefault("S3_USE_SSL", "false"),
		},
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
