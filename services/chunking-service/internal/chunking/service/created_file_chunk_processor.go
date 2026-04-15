package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
)

type CreatedFileChunkProcessor struct {
	snapshot   *shareddomain.Snapshot
	hasStarted bool

	logger *log.Logger

	blobStoreRepo        *sharedblobstore.Repo
	fetchFileChannel     chan *shareddomain.ChangeLogFile
	fetchFileWG          sync.WaitGroup
	fileFetchWorkerCount int

	normalizationService     *NormalizationService
	normalizationChannel     chan *shareddomain.FileJob
	normalizationWG          sync.WaitGroup
	normalizationWorkerCount int

	extractionChannel     chan *shareddomain.FileJob
	extractionService     *ExtractionService
	extractionWG          sync.WaitGroup
	extractionWorkerCount int

	chunkBuilder   *ChunkBuilder
	persistChannel chan *shareddomain.Chunk

	chunkRepo          *ports.ChunkRepo
	persistWG          sync.WaitGroup
	persistWorkerCount int

	kafkaProducer      sharedkafka.Producer
	kafkaIndexingTopic string
}

type CreatedFileChunkProcessorInput struct {
	snapshot             *shareddomain.Snapshot
	logger               *log.Logger
	blobStoreRepo        *sharedblobstore.Repo
	normalizationService *NormalizationService
	extractionService    *ExtractionService
	chunkRepo            *ports.ChunkRepo
	kafkaProducer        sharedkafka.Producer
	kafkaIndexingTopic   string
}

func NewCreatedFileChunkProcessor(input *CreatedFileChunkProcessorInput) *CreatedFileChunkProcessor {
	return &CreatedFileChunkProcessor{
		snapshot:             input.snapshot,
		hasStarted:           false,
		logger:               input.logger,
		fetchFileChannel:     make(chan *shareddomain.ChangeLogFile),
		fileFetchWorkerCount: 5,

		blobStoreRepo:        input.blobStoreRepo,
		normalizationChannel: make(chan *shareddomain.FileJob),

		normalizationService:     input.normalizationService,
		extractionChannel:        make(chan *shareddomain.FileJob),
		normalizationWorkerCount: 5,

		extractionService:     input.extractionService,
		extractionWorkerCount: 5,

		chunkBuilder:   NewChunkBuilder(),
		persistChannel: make(chan *shareddomain.Chunk),

		chunkRepo:          input.chunkRepo,
		persistWorkerCount: 5,

		kafkaProducer:      input.kafkaProducer,
		kafkaIndexingTopic: input.kafkaIndexingTopic,
	}
}

func (s *CreatedFileChunkProcessor) Run(changeLogFiles []shareddomain.ChangeLogFile) error {
	if s.hasStarted {
		return fmt.Errorf("Already started")
	}

	if len(changeLogFiles) == 0 {
		s.logger.Printf("no created or updated files to fetch snapshot=%s; deletion-only change log handled", s.snapshot.Id)
		return nil
	}

	s.hasStarted = true

	// start file fetch workers
	for i := 0; i < s.fileFetchWorkerCount; i++ {
		s.fetchFileWG.Add(1)
		go s.fetch()
	}

	// when fetching is fully done, close fetchFileChannel
	go func() {
		s.fetchFileWG.Wait()
		close(s.fetchFileChannel)
	}()

	// start normalization workers
	for i := 0; i < s.normalizationWorkerCount; i++ {
		s.normalizationWG.Add(1)
		go s.normalize()
	}

	// when normalization is fully done, close normalizationChannel
	go func() {
		s.normalizationWG.Wait()
		close(s.normalizationChannel)
	}()

	// start extraction workers
	for i := 0; i < s.extractionWorkerCount; i++ {
		s.extractionWG.Add(1)
		go s.extract()
	}

	// when extraction is fully done, close extractionChannel
	go func() {
		s.extractionWG.Wait()
		close(s.extractionChannel)
	}()

	// start final persist stage
	for i := 0; i < s.persistWorkerCount; i++ {
		s.persistWG.Add(1)
		go s.persist()
	}

	for i := range changeLogFiles {
		s.fetchFileChannel <- &changeLogFiles[i]
	}

	// wait for final step to complete
	s.persistWG.Wait()
	close(s.persistChannel)
	return nil
}
func (s *CreatedFileChunkProcessor) fetch() {
	defer s.fetchFileWG.Done()

	for changeLogFile := range s.fetchFileChannel {
		blobDirectoryURL, err := sharedutil.DeriveRawStorageURL(s.snapshot.ChangeLogURL)
		if err != nil {
			s.logger.Printf("failed to derive blob directory from change log url for snapshot=%s: %v", s.snapshot.Id, err)
			continue
		}

		fileJob, err := s.blobStoreRepo.GetFile(blobDirectoryURL + "/" + changeLogFile.FileHash)

		s.logger.Printf("fetch raw file at url=%s for file in repo at path=%s output_bytes=%d", fileJob.Path, changeLogFile.Path, fileJob.Size)

		// override the path returned from blob storage since the chunk needs the repo file path not the raw storage file url
		fileJob.Path = changeLogFile.Path
		s.normalizationChannel <- fileJob
	}

}

func (s *CreatedFileChunkProcessor) normalize() {
	defer s.normalizationWG.Done()

	for fileJob := range s.normalizationChannel {
		if fileJob == nil {
			s.logger.Printf("skipping nil file job in normalization step")
			continue
		}
		normalizedFileJob, err := s.normalizationService.NormalizeFileContent(fileJob)
		if err != nil {
			s.logger.Printf("normalization failed for file path=%s: %v", fileJob.Path, err)
			continue
		}

		s.logger.Printf("normalized file path=%s output_bytes=%d", normalizedFileJob.Path, len(normalizedFileJob.Content))
		s.extractionChannel <- normalizedFileJob
	}
}

func (s *CreatedFileChunkProcessor) extract() {
	defer s.extractionWG.Done()

	for fileJob := range s.extractionChannel {
		if fileJob == nil {
			s.logger.Printf("skipping nil file job in extraction step")
			continue
		}

		s.logger.Printf("extracting normalized content path=%s bytes=%d extension=%s", fileJob.Path, len(fileJob.Content), fileJob.Extension)
		extractedData, err := s.extractionService.ExtractFileContent(fileJob)
		if err != nil {
			s.logger.Printf("extraction failed path=%s: %v", fileJob.Path, err)
			continue
		}

		s.logger.Printf("extracted data path=%s symbols=%d containers=%d imports=%d doc_comments=%d classes=%d functions=%d methods=%d module_level_declarations=%d", fileJob.Path, len(extractedData.Symbols), len(extractedData.Containers), len(extractedData.Imports), len(extractedData.DocComments), len(extractedData.Classes), len(extractedData.Functions), len(extractedData.Methods), len(extractedData.ModuleLevelDeclarations))
		chunks, err := s.chunkBuilder.Build(s.snapshot, fileJob, extractedData)
		if err != nil {
			s.logger.Printf("chunk build failed path=%s: %v", fileJob.Path, err)
			continue
		}

		s.logger.Printf("built chunks path=%s chunks=%d", fileJob.Path, len(chunks))
		for _, chunk := range chunks {
			s.persistChannel <- chunk
		}
	}
}

func (s *CreatedFileChunkProcessor) persist() {
	defer s.persistWG.Done()

	for chunk := range s.persistChannel {
		err := s.chunkRepo.CreateChunk(context.Background(), chunk)
		if err != nil {
			s.logger.Printf("failed to write chunk to database snapshot=%s chunk_id=%s symbol=%s type=%s: %v", s.snapshot.Id, chunk.ChunkID, chunk.SymbolName, chunk.SymbolType, err)
			continue
		}
		event := shareddomain.ChunkIndexingEvent{
			EventType:  shareddomain.ChunkIndexRequestedEvent,
			ChunkID:    chunk.ChunkID,
			RepoURL:    chunk.RepoURL,
			Branch:     chunk.Branch,
			SnapshotID: chunk.SnapshotID,
			FileName:   chunk.FileName,
			FilePath:   chunk.FilePath,
		}
		value, err := json.Marshal(event)
		if err != nil {
			s.logger.Printf("failed to marshal chunk indexing event chunk_id=%s: %v", chunk.ChunkID, err)
			continue
		}

		if err := s.kafkaProducer.Produce(s.kafkaIndexingTopic, []byte(event.ChunkID), value); err != nil {
			s.logger.Printf("failed to produce indexing event chunk_id=%s: %v", chunk.ChunkID, err)
			continue
		}
	}
}
