package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/github"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/postgres"
	reposync "github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/service"
	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
)

type consumerHandlerDeps struct {
	repoRegistryRepo  ports.RepoRegistryRepo
	snapshotStoreRepo ports.SnapshotStoreRepo
	blobStoreRepo     ports.BlobStoreRepo
	dataSourceRepo    ports.DataSourceRepo
	snapshotProducer  sharedkafka.Producer
}

func main() {
	cfg := config.Load()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	ctx := context.Background()

	snapshotProducer, err := sharedkafka.NewProducer()
	if err != nil {
		logger.Fatalf("failed to create kafka producer: %v", err)
	}
	defer snapshotProducer.Close()

	repoRegistryRepo, err := postgres.NewRepoRegistryRepo(ctx, cfg.Database)
	if err != nil {
		logger.Fatalf("failed to create repo registry repo: %v", err)
	}

	snapshotStoreRepo, err := postgres.NewSnapshotStoreRepo(ctx, cfg.Database)
	if err != nil {
		logger.Fatalf("failed to create snapshot store repo: %v", err)
	}

	blobStoreRepo, err := sharedblobstore.NewRepo()
	if err != nil {
		logger.Fatalf("failed to create blob store repo: %v", err)
	}

	dataSourceRepo := github.NewDataSourceRepo(cfg.GitHub.Token)
	deps := &consumerHandlerDeps{
		repoRegistryRepo:  repoRegistryRepo,
		snapshotStoreRepo: snapshotStoreRepo,
		blobStoreRepo:     blobStoreRepo,
		dataSourceRepo:    dataSourceRepo,
		snapshotProducer:  snapshotProducer,
	}

	numberOfConsumers := 5

	eventConsumerConfig := &sharedkafka.ConsumerConfig{
		GroupID: "repo-sync-event-consumer-group",
	}

	createAndRunNKafkaConsumers(numberOfConsumers, eventConsumerConfig, cfg.Kafka.EventsTopic, logger, deps)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}

func createAndRunNKafkaConsumers(number int, consumerConfig *sharedkafka.ConsumerConfig, topic string, logger *log.Logger, deps *consumerHandlerDeps) {
	for i := 0; i < number; i++ {
		consumer, err := sharedkafka.NewConsumer(consumerConfig)
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

				var event reposync.RepoEvent
				if err := json.Unmarshal(message.Value, &event); err != nil {
					logger.Printf("consumer %d failed to decode message: %v", workerID, err)
					continue
				}

				if err := handleMessage(&event, logger, deps); err != nil {
					logger.Printf("consumer %d failed to process message for repo %s: %v", workerID, event.RepoURL, err)
					continue
				}

				if err := consumer.CommitMessage(message); err != nil {
					logger.Printf("consumer %d failed to commit message for repo %s: %v", workerID, event.RepoURL, err)
					continue
				}

				logger.Printf("consumer %d processed and committed message for repo: %s", workerID, event.RepoURL)
			}
		}(consumer, i)
	}
}

func handleMessage(msg *reposync.RepoEvent, logger *log.Logger, deps *consumerHandlerDeps) error {
	logger.Printf("Received message for repo: %s, event type: %s", msg.RepoURL, msg.EventType)

	switch msg.EventType {
	case "repo.created":
		logger.Printf("Handling repo registration for repo: %s", msg.RepoURL)

		registerRepoService := service.NewRegisterRepoService(&service.RegisterRepoServiceInput{
			RepoURL:   msg.RepoURL,
			Branch:    msg.Branch,
			CommitSHA: msg.CommitSHA,
		}, deps.dataSourceRepo, deps.snapshotStoreRepo, deps.blobStoreRepo, deps.repoRegistryRepo)

		snapshot, err := registerRepoService.RegisterRepo()
		if err != nil {
			return err
		}

		logger.Printf("Completed repo registration for repo: %s, snapshot ID: %s", msg.RepoURL, snapshot.Id)

		// publish event to kafka for repo registration completion with snapshot details
		kafkaMessage, err := json.Marshal(snapshot)
		if err != nil {
			logger.Printf("Failed to marshal snapshot for repo: %s", msg.RepoURL)
			return err
		}
		err = deps.snapshotProducer.Produce("snapshot.created", []byte(msg.RepoURL), kafkaMessage)
		if err != nil {
			logger.Printf("Failed to produce snapshot created event for repo: %s", msg.RepoURL)
			return err
		}

	case "repo.updated":
		logger.Printf("Handling repo update for repo: %s", msg.RepoURL)
		updateRepoService := service.NewRepoUpdateService(&service.RepoUpdateServiceInput{
			RepoURL:   msg.RepoURL,
			Branch:    msg.Branch,
			CommitSHA: msg.CommitSHA,
		}, deps.repoRegistryRepo, deps.dataSourceRepo, deps.blobStoreRepo, deps.snapshotStoreRepo)

		snapshot, err := updateRepoService.UpdateRepo()
		if err != nil {
			return err
		}

		if snapshot == nil {
			logger.Printf("No update needed for repo: %s, already at commit SHA: %s", msg.RepoURL, msg.CommitSHA)
			return nil
		}

		logger.Printf("Completed repo update for repo: %s, new snapshot ID: %s", msg.RepoURL, snapshot.Id)

		// publish event to kafka for repo update completion with snapshot details
		kafkaMessage, err := json.Marshal(snapshot)
		if err != nil {
			logger.Printf("Failed to marshal snapshot for repo: %s", msg.RepoURL)
			return err
		}
		err = deps.snapshotProducer.Produce("snapshot.created", []byte(msg.RepoURL), kafkaMessage)
		if err != nil {
			logger.Printf("Failed to produce snapshot created event for repo: %s", msg.RepoURL)
			return err
		}

	case "repo.deleted":
		logger.Printf("Handling repo deletion for repo: %s", msg.RepoURL)

		deleteRepoService := service.NewDeleteRepoService(&service.DeleteRepoServiceInput{
			RepoURL:   msg.RepoURL,
			Branch:    msg.Branch,
			CommitSHA: msg.CommitSHA,
		}, deps.snapshotStoreRepo, deps.blobStoreRepo, deps.repoRegistryRepo)

		if err := deleteRepoService.DeleteRepo(); err != nil {
			return err
		}

		logger.Printf("Completed repo deletion for repo: %s", msg.RepoURL)

	default:
		logger.Printf("Unknown event type: %s for repo: %s", msg.EventType, msg.RepoURL)
	}

	return nil
}
