package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	httpapi "github.com/rohandave/tessa-rag/services/repo-sync-service/internal/http"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/kafka"
)

func main() {
	cfg := config.Load()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf(
		"starting %s api on :%s with kafka=%s eventstopic=%s lifecycletopic=%s bucket=%s s3=%s",
		cfg.ServiceName,
		cfg.Port,
		cfg.Kafka.Brokers,
		cfg.Kafka.EventsTopic,
		cfg.Kafka.LifeCycleTopic,
		cfg.Storage.Bucket,
		cfg.Storage.Endpoint,
	)

	eventsproducer := kafka.NewDummyProducer(cfg.Kafka.EventsTopic)
	lifecycleproducer := kafka.NewDummyProducer(cfg.Kafka.LifeCycleTopic)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(cfg, eventsproducer, lifecycleproducer),
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
