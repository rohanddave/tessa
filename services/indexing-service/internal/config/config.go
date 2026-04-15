package config

import (
	"os"
	"strconv"

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
	OpenAI        *OpenAIConfig
	Neo4j         *Neo4jConfig
}

type ElasticsearchConfig struct {
	URL   string
	Index string
}

type PineconeConfig struct {
	Host       string
	APIKey     string
	APIVersion string
	Index      string
	Namespace  string
}

type OpenAIConfig struct {
	APIKey              string
	BaseURL             string
	EmbeddingModel      string
	EmbeddingDimensions int
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
			URL:   sharedutil.EnvOrDefault("ELASTICSEARCH_URL", "http://localhost:9200"),
			Index: sharedutil.EnvOrDefault("ELASTICSEARCH_INDEX", "tessa-chunks"),
		},
		Pinecone: &PineconeConfig{
			Host:       sharedutil.EnvOrDefault("PINECONE_HOST", "http://localhost:5081"),
			APIKey:     sharedutil.EnvOrDefault("PINECONE_API_KEY", "pclocal"),
			APIVersion: sharedutil.EnvOrDefault("PINECONE_API_VERSION", "2025-01"),
			Index:      sharedutil.EnvOrDefault("PINECONE_INDEX", "tessa-chunks"),
			Namespace:  sharedutil.EnvOrDefault("PINECONE_NAMESPACE", "__default__"),
		},
		OpenAI: &OpenAIConfig{
			APIKey:              os.Getenv("OPENAI_API_KEY"),
			BaseURL:             sharedutil.EnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			EmbeddingModel:      sharedutil.EnvOrDefault("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
			EmbeddingDimensions: envIntOrDefault("OPENAI_EMBEDDING_DIMENSIONS", 0),
		},
		Neo4j: &Neo4jConfig{
			URI:      sharedutil.EnvOrDefault("NEO4J_URI", "bolt://localhost:7687"),
			User:     sharedutil.EnvOrDefault("NEO4J_USER", "neo4j"),
			Password: sharedutil.EnvOrDefault("NEO4J_PASSWORD", "password"),
		},
	}
}

func envIntOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
