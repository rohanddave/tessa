package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/service"
	"github.com/rohandave/tessa-rag/services/chunking-service/internal/config"
	treesitter "github.com/rohandave/tessa-rag/services/chunking-service/internal/tree-sitter"
	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
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

	consumer := service.NewConsumer(logger, blobStoreRepo, codeParser)
	fmt.Printf("consumer: %v\n", consumer)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Printf("shutting down %s consumer", cfg.ServiceName)
}
