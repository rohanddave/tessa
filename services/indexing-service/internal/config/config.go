package config

import (
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
	sharedpostgres "github.com/rohandave/tessa-rag/services/shared/postgres"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
)

type Config struct {
	ServiceName   string
	Kafka         *sharedkafka.KafkaConfig
	Database      *sharedpostgres.DatabaseConfig
	Elasticsearch *ElasticsearchConfig
	Pinecone      *PineconeConfig
	Neo4j         *Neo4jConfig
}

type ElasticsearchConfig struct {
	URL string
}

type PineconeConfig struct {
	Host   string
	APIKey string
	Index  string
}

type Neo4jConfig struct {
	URI      string
	User     string
	Password string
}

func Load() *Config {
	return &Config{
		ServiceName: "indexing-service",
		Kafka:       sharedkafka.LoadConfig(),
		Database:    sharedpostgres.LoadConfig(),
		Elasticsearch: &ElasticsearchConfig{
			URL: sharedutil.EnvOrDefault("ELASTICSEARCH_URL", "http://localhost:9200"),
		},
		Pinecone: &PineconeConfig{
			Host:   sharedutil.EnvOrDefault("PINECONE_HOST", "http://localhost:5080"),
			APIKey: sharedutil.EnvOrDefault("PINECONE_API_KEY", "pclocal"),
			Index:  sharedutil.EnvOrDefault("PINECONE_INDEX", "tessa-chunks"),
		},
		Neo4j: &Neo4jConfig{
			URI:      sharedutil.EnvOrDefault("NEO4J_URI", "bolt://localhost:7687"),
			User:     sharedutil.EnvOrDefault("NEO4J_USER", "neo4j"),
			Password: sharedutil.EnvOrDefault("NEO4J_PASSWORD", "password"),
		},
	}
}
