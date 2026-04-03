package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/github"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/kafka"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/minio"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/postgres"
	reposync "github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/service"
)

type consumerHandlerDeps struct {
	stateGateRepo     ports.StateGateRepo
	snapshotStoreRepo ports.SnapshotStoreRepo
	blobStoreRepo     ports.BlobStoreRepo
	dataSourceRepo    ports.DataSourceRepo
}

func main() {
	cfg := config.Load()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	ctx := context.Background()

	stateGateRepo, err := postgres.NewStateGateRepo(ctx, cfg.Database)
	if err != nil {
		logger.Fatalf("failed to create state gate repo: %v", err)
	}

	snapshotStoreRepo, err := postgres.NewSnapshotStoreRepo(ctx, cfg.Database)
	if err != nil {
		logger.Fatalf("failed to create snapshot store repo: %v", err)
	}

	blobStoreRepo, err := minio.NewBlobStoreRepo(cfg.Storage)
	if err != nil {
		logger.Fatalf("failed to create blob store repo: %v", err)
	}

	dataSourceRepo := github.NewDataSourceRepo(cfg.GitHub.Token)
	deps := consumerHandlerDeps{
		stateGateRepo:     stateGateRepo,
		snapshotStoreRepo: snapshotStoreRepo,
		blobStoreRepo:     blobStoreRepo,
		dataSourceRepo:    dataSourceRepo,
	}

	numberOfLifeCycleConsumers := 5
	numberOfEventConsumers := 5

	lifecycleConsumerConfig := &kafka.KafkaConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		GroupId: "repo-sync-lifecycle-consumer-group",
	}

	createAndRunNKafkaConsumers(numberOfLifeCycleConsumers, lifecycleConsumerConfig, cfg.Kafka.LifeCycleTopic, logger, deps)

	eventConsumerConfig := &kafka.KafkaConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		GroupId: "repo-sync-event-consumer-group",
	}

	createAndRunNKafkaConsumers(numberOfEventConsumers, eventConsumerConfig, cfg.Kafka.EventsTopic, logger, deps)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}

func createAndRunNKafkaConsumers(number int, consumerConfig *kafka.KafkaConsumerConfig, topic string, logger *log.Logger, deps consumerHandlerDeps) {
	for i := 0; i < number; i++ {
		consumer := kafka.NewKafkaConsumer(consumerConfig)
		err := consumer.SubscribeTopics([]string{topic})
		if err != nil {
			logger.Fatalf("failed to subscribe consumer to topic %s: %v", topic, err)
		}

		go func(consumer kafka.Consumer, workerID int) {
			defer consumer.Close()

			for {
				message, err := consumer.ReadMessage(5 * time.Second)
				if err != nil {
					logger.Printf("consumer %d read error on topic %s: %v", workerID, topic, err)
					continue
				}

				if message == nil || message.Event() == nil {
					continue
				}

				event := message.Event()
				if err := handleMessage(event, logger, deps); err != nil {
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

func handleMessage(msg *reposync.RepoEvent, logger *log.Logger, deps consumerHandlerDeps) error {
	logger.Printf("Received message for repo: %s, event type: %s", msg.RepoURL, msg.EventType)

	switch msg.EventType {
	case "repo.created":
		logger.Printf("Handling repo registration for repo: %s", msg.RepoURL)

		registerRepoService := service.NewRegisterRepoService(service.RegisterRepoServiceInput{
			RepoURL:   msg.RepoURL,
			Branch:    msg.Branch,
			CommitSHA: msg.CommitSHA,
		}, deps.dataSourceRepo, deps.snapshotStoreRepo, deps.blobStoreRepo, deps.stateGateRepo)

		if err := registerRepoService.RegisterRepo(); err != nil {
			return err
		}

		logger.Printf("Completed repo registration for repo: %s", msg.RepoURL)

	case "repo.updated":
		logger.Printf("Handling repo update for repo: %s", msg.RepoURL)

	case "repo.deleted":
		logger.Printf("Handling repo deletion for repo: %s", msg.RepoURL)

		deleteRepoService := service.NewDeleteRepoService(service.DeleteRepoServiceInput{
			RepoURL:   msg.RepoURL,
			Branch:    msg.Branch,
			CommitSHA: msg.CommitSHA,
		}, deps.snapshotStoreRepo, deps.blobStoreRepo, deps.stateGateRepo)

		if err := deleteRepoService.DeleteRepo(); err != nil {
			return err
		}

		logger.Printf("Completed repo deletion for repo: %s", msg.RepoURL)

	default:
		logger.Printf("Unknown event type: %s for repo: %s", msg.EventType, msg.RepoURL)
	}

	return nil
}
