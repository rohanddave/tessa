package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	httpapi "github.com/rohandave/tessa-rag/services/repo-sync-service/internal/http"
)

func main() {
	cfg := config.Load()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf(
		"starting %s api on :%s with kafka=%s eventstopic=%s bucket=%s s3=%s",
		cfg.ServiceName,
		cfg.Port,
		cfg.Kafka.Brokers,
		cfg.Kafka.EventsTopic,
		cfg.Storage.Bucket,
		cfg.Storage.Endpoint,
	)

	producer, err := sharedkafka.NewProducer()
	if err != nil {
		logger.Fatalf("failed to create kafka producer: %v", err)
	}
	defer producer.Close()

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(cfg, producer),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Printf("shutting down %s api", cfg.ServiceName)
	if err := server.Shutdown(ctx); err != nil {
		logger.Fatalf("shutdown failed: %v", err)
	}
}
