package service

import (
	"log"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
)

type Consumer struct {
	logger        *log.Logger
	blobStoreRepo *sharedblobstore.Repo
	codeParser    ports.CodeParser
}

func NewConsumer(logger *log.Logger, blobStoreRepo *sharedblobstore.Repo, codeParser ports.CodeParser) *Consumer {
	return &Consumer{logger: logger, blobStoreRepo: blobStoreRepo, codeParser: codeParser}
}

func (c *Consumer) Start() {
	c.logger.Printf("Starting consumer with code parser: %s", c.codeParser.DescribeParser())
}
