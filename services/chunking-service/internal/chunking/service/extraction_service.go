package service

import (
	"fmt"
	"log"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
)

type ExtractionServiceInput struct {
	CodeParser ports.CodeParser
}

type ExtractionService struct {
	codeParser ports.CodeParser
}

func NewExtractionService(input *ExtractionServiceInput) *ExtractionService {
	if input == nil {
		log.Printf("extraction service initialized with nil input")
		return &ExtractionService{}
	}

	if input.CodeParser == nil {
		log.Printf("extraction service initialized without a code parser")
	}

	return &ExtractionService{
		codeParser: input.CodeParser,
	}
}

func (s *ExtractionService) ExtractFileContent(fileJob *shareddomain.FileJob) (*ports.CodeParseResult, error) {
	if s.codeParser == nil {
		err := fmt.Errorf("code parser is not configured")
		log.Printf("extraction failed: %v", err)
		return nil, err
	}

	parsedData, err := s.codeParser.ExtractFileMetadata(string(fileJob.Content), fileJob.Extension)
	if err != nil {
		log.Printf("code extraction failed language=%s: %v", "ts", err)
		return nil, err
	}

	if parsedData == nil {
		err := fmt.Errorf("code parser returned nil parse result")
		log.Printf("code extraction failed language=%s: %v", "ts", err)
		return nil, err
	}

	log.Printf(
		"completed code extraction symbols=%d containers=%d imports=%d doc_comments=%d classes=%d functions=%d methods=%d module_level_declarations=%d",
		len(parsedData.Symbols),
		len(parsedData.Containers),
		len(parsedData.Imports),
		len(parsedData.DocComments),
		len(parsedData.Classes),
		len(parsedData.Functions),
		len(parsedData.Methods),
		len(parsedData.ModuleLevelDeclarations),
	)

	return parsedData, nil
}
