package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/kafka"
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

	for i := 0; i < numberOfLifeCycleConsumers; i++ {
		c := kafka.NewKafkaConsumer(lifecycleConsumerConfig)
		err := c.SubscribeTopics([]string{cfg.Kafka.LifeCycleTopic})
		if err != nil {
			logger.Fatalf("failed to subscribe lifecycle consumer to topic: %v", err)
		}

		go func(consumer kafka.Consumer, workerId int) {
			defer consumer.Close()

			for {
				msg, err := consumer.ReadMessage(5)
				if err != nil {
					logger.Printf("lifecycle consumer %d read error: %v", workerId, err)
					continue
				}
				logger.Printf("lifecycle consumer %d received message: %s", workerId, string(msg.RepoURL))
			}
		}(c, i)
	}

	eventConsumerConfig := &kafka.KafkaConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		GroupId: "repo-sync-event-consumer-group",
	}

	for i := 0; i < numberOfEventConsumers; i++ {
		c := kafka.NewKafkaConsumer(eventConsumerConfig)
		err := c.SubscribeTopics([]string{cfg.Kafka.EventsTopic})
		if err != nil {
			logger.Fatalf("failed to subscribe event consumer to topic: %v", err)
		}

		go func(consumer kafka.Consumer, workerId int) {
			defer consumer.Close()

			for {
				msg, err := consumer.ReadMessage(5)
				if err != nil {
					logger.Printf("events consumer %d read error: %v", workerId, err)
					continue
				}
				logger.Printf("events consumer %d received message: %s", workerId, string(msg.RepoURL))
			}
		}(c, i)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}
