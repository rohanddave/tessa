package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/kafka"
	reposync "github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync"
)

func main() {
	cfg := config.Load()

	logger := log.New(os.Stdout, "", log.LstdFlags)

	numberOfLifeCycleConsumers := 5
	numberOfEventConsumers := 5

	lifecycleConsumerConfig := &kafka.KafkaConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		GroupId: "repo-sync-lifecycle-consumer-group",
	}

	createAndRunNKafkaConsumers(numberOfLifeCycleConsumers, lifecycleConsumerConfig, cfg.Kafka.LifeCycleTopic, logger)

	eventConsumerConfig := &kafka.KafkaConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		GroupId: "repo-sync-event-consumer-group",
	}

	createAndRunNKafkaConsumers(numberOfEventConsumers, eventConsumerConfig, cfg.Kafka.EventsTopic, logger)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}

func createAndRunNKafkaConsumers(number int, config *kafka.KafkaConsumerConfig, topic string, logger *log.Logger) {
	for i := 0; i < number; i++ {
		c := kafka.NewKafkaConsumer(config)
		err := c.SubscribeTopics([]string{topic})

		if err != nil {
			logger.Fatalf("failed to subscribe lifecycle consumer to topic: %v", err)
		}

		go func(consumer kafka.Consumer, workerId int) {
			defer consumer.Close()

			for {
				msg, err := consumer.ReadMessage(5 * time.Second)
				if err != nil {
					logger.Printf("lifecycle consumer %d read error: %v", workerId, err)
					continue
				}

				if msg == nil {
					continue
				}

				handleMessage(msg, logger)
				logger.Printf("lifecycle consumer %d received message for repo: %s", workerId, msg.RepoURL)
			}
		}(c, i)
	}
}

func handleMessage(msg *reposync.RepoEvent, logger *log.Logger) {
	logger.Printf("Received message for repo: %s, event type: %s", msg.RepoURL, msg.EventType)
	switch msg.EventType {
	case "repo.created":
		// handle repo registration event
		logger.Printf("Handling repo registration for repo: %s", msg.RepoURL)
		logger.Printf("Repo registration service wiring is still pending for repo: %s", msg.RepoURL)

	case "repo.updated":
		// handle repo update event
		logger.Printf("Handling repo update for repo: %s", msg.RepoURL)
	case "repo.deleted":
		// handle repo deletion event
		logger.Printf("Handling repo deletion for repo: %s", msg.RepoURL)
	default:
		logger.Printf("Unknown event type: %s for repo: %s", msg.EventType, msg.RepoURL)
	}
}
