package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	ports "github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	"github.com/rohandave/tessa-rag/services/chunking-service/internal/config"
	treesitter "github.com/rohandave/tessa-rag/services/chunking-service/internal/tree-sitter"
	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
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

	createAndRunNKafkaConsumers(5, blobStoreRepo, &codeParser, cfg.Kafka.SnapshotsTopic, logger)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}

func createAndRunNKafkaConsumers(number int, blobStoreRepo *sharedblobstore.Repo, codeParser *ports.CodeParser, topic string, logger *log.Logger) {
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

				logger.Printf("consumer %d processed and message: %s", workerID, message)
			}
		}(consumer, i)
	}
}
