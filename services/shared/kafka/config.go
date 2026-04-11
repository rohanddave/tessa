package kafka

import sharedutil "github.com/rohandave/tessa-rag/services/shared/util"

type KafkaConfig struct {
	Brokers        string
	EventsTopic    string
	SnapshotsTopic string
	IndexingTopic  string
}

func LoadConfig() *KafkaConfig {
	return &KafkaConfig{
		Brokers:        sharedutil.EnvOrDefault("KAFKA_BROKERS", "localhost:19092"),
		EventsTopic:    sharedutil.EnvOrDefault("KAFKA_EVENTS_TOPIC", "repo-sync.repo-events"),
		SnapshotsTopic: sharedutil.EnvOrDefault("KAFKA_SNAPSHOTS_TOPIC", "snapshot.created"),
		IndexingTopic:  sharedutil.EnvOrDefault("KAFKA_INDEXING_TOPIC", "chunking.chunk-indexing-events"),
	}
}
