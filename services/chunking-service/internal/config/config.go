package config

import (
	sharedstorage "github.com/rohandave/tessa-rag/services/shared/blobstore"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
	sharedpostgres "github.com/rohandave/tessa-rag/services/shared/postgres"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
)

type Config struct {
	ServiceName string
	Port        string
	Storage     *sharedstorage.BlobStorageConfig
	Kafka       *sharedkafka.KafkaConfig
	Database    *sharedpostgres.DatabaseConfig
}

func Load() *Config {
	return &Config{
		ServiceName: sharedutil.EnvOrDefault("SERVICE_NAME", "chunking-service"),
		Port:        sharedutil.EnvOrDefault("PORT", "8082"),
		Storage:     sharedstorage.LoadConfig(),
		Kafka:       sharedkafka.LoadConfig(),
		Database:    sharedpostgres.LoadConfig(),
	}
}
