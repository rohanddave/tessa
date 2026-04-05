package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/service"
	"github.com/rohandave/tessa-rag/services/chunking-service/internal/config"
)

func main() {
	cfg := config.Load()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("starting %s consumer", cfg.ServiceName)

	blobStoreRepo, err := sharedblobstore.NewRepo(sharedblobstore.Config{
		Endpoint:        cfg.Storage.Endpoint,
		Region:          cfg.Storage.Region,
		Bucket:          cfg.Storage.Bucket,
		AccessKeyID:     cfg.Storage.AccessKeyID,
		SecretAccessKey: cfg.Storage.SecretAccessKey,
		UseSSL:          cfg.Storage.UseSSL,
	})
	if err != nil {
		logger.Fatalf("failed to create blob store repo: %v", err)
	}

	consumer := service.NewConsumer(logger)
	consumer.SetBlobStoreRepo(blobStoreRepo)
	consumer.DescribeParser()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}
