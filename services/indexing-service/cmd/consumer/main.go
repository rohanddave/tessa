package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/config"
	"github.com/rohandave/tessa-rag/services/indexing-service/internal/indexing/ports"
	"github.com/rohandave/tessa-rag/services/indexing-service/internal/indexing/service"
	"github.com/rohandave/tessa-rag/services/shared/domain"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("starting %s consumer", cfg.ServiceName)

	chunkRepo, err := ports.NewChunkRepo(context.Background(), cfg.Database)
	if err != nil {
		logger.Fatalf("failed to create chunk repo: %v", err)
	}
	defer chunkRepo.Close()

	indexingService := service.NewIndexingService(
		logger,
		chunkRepo,
		service.NewElasticsearchIndexer(logger, cfg.Elasticsearch),
		service.NewPineconeIndexer(logger, cfg.Pinecone, service.NewOpenAIEmbeddingProvider(logger, cfg.OpenAI)),
		service.NewNeo4jIndexer(logger, cfg.Neo4j),
	)

	createAndRunNKafkaConsumers(20, cfg.Kafka.IndexingTopic, logger, indexingService)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}

func createAndRunNKafkaConsumers(number int, topic string, logger *log.Logger, indexingService *service.IndexingService) {
	for i := 0; i < number; i++ {
		consumer, err := sharedkafka.NewConsumer(&sharedkafka.ConsumerConfig{
			GroupID: "indexing-service-consumer-group",
		})
		if err != nil {
			logger.Fatalf("failed to create consumer for topic %s: %v", topic, err)
		}
		if err := consumer.SubscribeTopics([]string{topic}); err != nil {
			logger.Fatalf("failed to subscribe consumer to topic %s: %v", topic, err)
		}

		go func(consumer sharedkafka.Consumer, workerID int) {
			defer consumer.Close()

			for {
				message, err := consumer.ReadMessage(1 * time.Second)
				if err != nil {
					logger.Printf("consumer %d read error on topic %s: %v", workerID, topic, err)
					continue
				}
				if message == nil || len(message.Value) == 0 {
					continue
				}

				var event domain.ChunkIndexingEvent
				if err := json.Unmarshal(message.Value, &event); err != nil {
					logger.Printf("consumer %d failed to decode chunk indexing event: %v", workerID, err)
					continue
				}

				if err := indexingService.HandleChunkEvent(context.Background(), event); err != nil {
					logger.Printf("consumer %d failed to process chunk event type=%s chunk_id=%s: %v", workerID, event.EventType, event.ChunkID, err)
					continue
				}

				if err := consumer.CommitMessage(message); err != nil {
					logger.Printf("consumer %d failed to commit chunk event chunk_id=%s: %v", workerID, event.ChunkID, err)
					continue
				}

				logger.Printf("consumer %d processed and committed chunk event type=%s chunk_id=%s", workerID, event.EventType, event.ChunkID)
			}
		}(consumer, i)
	}
}
