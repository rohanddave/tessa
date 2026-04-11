package postgres

import sharedutil "github.com/rohandave/tessa-rag/services/shared/util"

type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

func LoadConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Host:     sharedutil.EnvOrDefault("SNAPSHOT_STORE_HOST", "localhost"),
		Port:     sharedutil.EnvOrDefault("SNAPSHOT_STORE_PORT", "5432"),
		Name:     sharedutil.EnvOrDefault("SNAPSHOT_STORE_DB", "snapshot_store"),
		User:     sharedutil.EnvOrDefault("SNAPSHOT_STORE_USER", "postgres"),
		Password: sharedutil.EnvOrDefault("SNAPSHOT_STORE_PASSWORD", "postgres"),
		SSLMode:  sharedutil.EnvOrDefault("SNAPSHOT_STORE_SSLMODE", "disable"),
	}
}
