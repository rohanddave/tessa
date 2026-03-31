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
	// TODO: update the topic for this consumer - lifecycle or events topic?
	consumer := kafka.NewDummyConsumer(cfg.Kafka.EventsTopic)

	logger.Printf(
		"starting %s consumer with kafka=%s topic=%s",
		cfg.ServiceName,
		cfg.Kafka.Brokers,
		consumer.Topic(),
	)
	logger.Printf("consumer is a placeholder process for decoupled background work")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}
