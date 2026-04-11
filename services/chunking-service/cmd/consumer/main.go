package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	ports "github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/service"
	"github.com/rohandave/tessa-rag/services/chunking-service/internal/config"
	treesitter "github.com/rohandave/tessa-rag/services/chunking-service/internal/tree-sitter"
	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
)

func main() {
	cfg := config.Load()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("starting %s consumer", cfg.ServiceName)

	blobStoreRepo, err := sharedblobstore.NewRepo()
	if err != nil {
		logger.Fatalf("failed to create blob store repo: %v", err)
	}

	codeParser := treesitter.NewTreeSitterRepo()
	normalizationService := service.NewNormalizationService(&service.NormalizationServiceInput{})
	extractionService := service.NewExtractionService(&service.ExtractionServiceInput{
		CodeParser: codeParser,
	})

	chunkRepo, err := ports.NewChunkRepo(context.Background(), cfg.Database)
	if err != nil {
		logger.Fatalf("failed to chunk repo: %v", err)
	}

	kafkaProducer, err := sharedkafka.NewProducer()
	if err != nil {
		logger.Fatalf("failed to create kafka producer: %v", err)
	}
	defer kafkaProducer.Close()

	chunkingService := service.NewChunkingService(&service.ChunkingServiceInput{
		Logger:               logger,
		BlobStoreRepo:        blobStoreRepo,
		ChunkRepo:            chunkRepo,
		KafkaProducer:        kafkaProducer,
		IndexingTopic:        cfg.Kafka.IndexingTopic,
		NormalizationService: normalizationService,
		ExtractionService:    extractionService,
	})

	createAndRunNKafkaConsumers(5, cfg.Kafka.SnapshotsTopic, logger, chunkingService)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}

func createAndRunNKafkaConsumers(number int, topic string, logger *log.Logger, chunkingService *service.ChunkingService) {
	for i := 0; i < number; i++ {
		consumer, err := sharedkafka.NewConsumer(&sharedkafka.ConsumerConfig{
			GroupID: "chunking-service-consumer-group",
		})
		if err != nil {
			logger.Fatalf("failed to create consumer for topic %s: %v", topic, err)
		}
		err = consumer.SubscribeTopics([]string{topic})
		if err != nil {
			logger.Fatalf("failed to subscribe consumer to topic %s: %v", topic, err)
		}

		go func(consumer sharedkafka.Consumer, workerID int) {
			defer consumer.Close()

			for {
				message, err := consumer.ReadMessage(5 * time.Second)
				if err != nil {
					logger.Printf("consumer %d read error on topic %s: %v", workerID, topic, err)
					continue
				}

				if message == nil || len(message.Value) == 0 {
					continue
				}

				var snapshot shareddomain.Snapshot
				if err := json.Unmarshal(message.Value, &snapshot); err != nil {
					logger.Printf("consumer %d failed to decode snapshot message: %v", workerID, err)
					continue
				}

				logger.Printf("consumer %d received snapshot: %s to index", workerID, snapshot.Id)

				err = chunkingService.Start(snapshot)
				if err != nil {
					logger.Printf("consumer %d failed to process snapshot %s: %v", workerID, snapshot.Id, err)
					continue
				}

				if err := consumer.CommitMessage(message); err != nil {
					logger.Printf("consumer %d failed to commit snapshot %s: %v", workerID, snapshot.Id, err)
					continue
				}

				logger.Printf("consumer %d processed and committed snapshot: %s", workerID, snapshot.Id)
			}
		}(consumer, i)
	}
}
