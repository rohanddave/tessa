package service

import (
	"log"

	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
	sitter "github.com/smacker/go-tree-sitter"
)

type Consumer struct {
	logger        *log.Logger
	blobStoreRepo *sharedblobstore.Repo
}

func NewConsumer(logger *log.Logger) *Consumer {
	return &Consumer{logger: logger}
}

func (c *Consumer) SetBlobStoreRepo(blobStoreRepo *sharedblobstore.Repo) {
	c.blobStoreRepo = blobStoreRepo
}

func (c *Consumer) DescribeParser() {
	parser := sitter.NewParser()
	defer parser.Close()

	c.logger.Printf("tree-sitter parser initialized: %t", parser != nil)
	c.logger.Printf("blob store repo initialized: %t", c.blobStoreRepo != nil)
}
