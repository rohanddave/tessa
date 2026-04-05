package config

import "os"

type Config struct {
	ServiceName string
	Port        string
	Storage     StorageConfig
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
		ServiceName: getEnv("SERVICE_NAME", "chunking-service"),
		Port:        getEnv("PORT", "8082"),
		Storage: StorageConfig{
			Endpoint:        getEnv("S3_ENDPOINT", "localhost:9000"),
			Region:          getEnv("S3_REGION", "us-east-1"),
			Bucket:          getEnv("S3_BUCKET", "repo-sync"),
			AccessKeyID:     getEnv("S3_ACCESS_KEY_ID", "minioadmin"),
			SecretAccessKey: getEnv("S3_SECRET_ACCESS_KEY", "minioadmin"),
			UseSSL:          getEnv("S3_USE_SSL", "false"),
		},
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
