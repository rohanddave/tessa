package kafka

import "os"

type Config struct {
	Brokers        string
	EventsTopic    string
	SnapshotsTopic string
}

func LoadConfig() Config {
	return Config{
		Brokers:        envOrDefault("KAFKA_BROKERS", "localhost:19092"),
		EventsTopic:    envOrDefault("KAFKA_EVENTS_TOPIC", "repo-sync.repo-events"),
		SnapshotsTopic: envOrDefault("KAFKA_SNAPSHOTS_TOPIC", "snapshot.created"),
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
