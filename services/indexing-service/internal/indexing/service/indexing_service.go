package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/indexing/ports"
	"github.com/rohandave/tessa-rag/services/shared/domain"
)

type IndexingService struct {
	logger      *log.Logger
	chunkRepo   *ports.ChunkRepo
	textSearch  ports.TextSearchIndexer
	vectorStore ports.VectorIndexer
	graphStore  ports.GraphIndexer
}

func NewIndexingService(logger *log.Logger, chunkRepo *ports.ChunkRepo, textSearch ports.TextSearchIndexer, vectorStore ports.VectorIndexer, graphStore ports.GraphIndexer) *IndexingService {
	return &IndexingService{
		logger:      logger,
		chunkRepo:   chunkRepo,
		textSearch:  textSearch,
		vectorStore: vectorStore,
		graphStore:  graphStore,
	}
}

func (s *IndexingService) HandleChunkEvent(ctx context.Context, event domain.ChunkIndexingEvent) error {
	s.logger.Printf("handling chunk indexing event type=%s chunk_id=%s snapshot=%s", event.EventType, event.ChunkID, event.SnapshotID)

	switch event.EventType {
	case domain.ChunkIndexRequestedEvent:
		return s.indexChunk(ctx, event.ChunkID)
	case domain.ChunkDeleteRequestedEvent:
		return s.deleteChunk(ctx, event.ChunkID)
	default:
		return fmt.Errorf("unsupported chunk indexing event type %q", event.EventType)
	}
}

func (s *IndexingService) indexChunk(ctx context.Context, chunkID string) error {
	chunk, err := s.chunkRepo.GetChunkByID(ctx, chunkID)
	if err != nil {
		return err
	}

	err = s.runIndexerWorkers(chunkID, "index", []indexerWorker{
		{
			name: "elasticsearch",
			run: func() error {
				if err := s.textSearch.IndexChunk(ctx, chunk); err != nil {
					return err
				}
				return s.chunkRepo.MarkTextSearchIndexed(ctx, chunkID)
			},
		},
		{
			name: "pinecone",
			run: func() error {
				if err := s.vectorStore.IndexChunk(ctx, chunk); err != nil {
					return err
				}
				return s.chunkRepo.MarkVectorIndexed(ctx, chunkID)
			},
		},
		{
			name: "neo4j",
			run: func() error {
				if err := s.graphStore.IndexChunk(ctx, chunk); err != nil {
					return err
				}
				return s.chunkRepo.MarkGraphIndexed(ctx, chunkID)
			},
		},
	})
	if err != nil {
		return err
	}

	if err := s.chunkRepo.MarkChunkIndexed(ctx, chunkID); err != nil {
		return err
	}

	s.logger.Printf("indexed chunk chunk_id=%s file=%s symbol=%s type=%s", chunkID, chunk.FilePath, chunk.SymbolName, chunk.SymbolType)
	return nil
}

func (s *IndexingService) deleteChunk(ctx context.Context, chunkID string) error {
	err := s.runIndexerWorkers(chunkID, "delete", []indexerWorker{
		{
			name: "elasticsearch",
			run: func() error {
				if err := s.textSearch.DeleteChunk(ctx, chunkID); err != nil {
					return err
				}
				return s.chunkRepo.MarkTextSearchDeleted(ctx, chunkID)
			},
		},
		{
			name: "pinecone",
			run: func() error {
				if err := s.vectorStore.DeleteChunk(ctx, chunkID); err != nil {
					return err
				}
				return s.chunkRepo.MarkVectorDeleted(ctx, chunkID)
			},
		},
		{
			name: "neo4j",
			run: func() error {
				if err := s.graphStore.DeleteChunk(ctx, chunkID); err != nil {
					return err
				}
				return s.chunkRepo.MarkGraphDeleted(ctx, chunkID)
			},
		},
	})
	if err != nil {
		return err
	}

	if err := s.chunkRepo.MarkChunkDeleted(ctx, chunkID); err != nil {
		return err
	}

	s.logger.Printf("deleted chunk from indexes chunk_id=%s", chunkID)
	return nil
}

type indexerWorker struct {
	name string
	run  func() error
}

func (s *IndexingService) runIndexerWorkers(chunkID string, operation string, workers []indexerWorker) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(workers))

	for _, worker := range workers {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := worker.run(); err != nil {
				errs <- fmt.Errorf("%s %s chunk %s: %w", operation, worker.name, chunkID, err)
			}
		}()
	}

	wg.Wait()
	close(errs)

	var joined error
	for err := range errs {
		joined = errors.Join(joined, err)
	}

	return joined
}
