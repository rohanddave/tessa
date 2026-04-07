package service

import "github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"

type ExtractionServiceInput struct {
	CodeParser ports.CodeParser
}

type ExtractionService struct {
	codeParser ports.CodeParser
}

func NewExtractionService(input *ExtractionServiceInput) *ExtractionService {
	return &ExtractionService{
		codeParser: input.CodeParser,
	}
}

func (s *ExtractionService) ExtractFileContent(fileContent string) (map[string]string, error) {
	// Implement your Extraction logic here using s.codeParser
	// This is a placeholder for your actual Extraction logic
	// You can use s.codeParser to parse the file content and extract relevant information

	parsedData := make(map[string]string)

	// Example of using the code parser to parse the file content
	// parsedData = s.codeParser.Parse(fileContent, filePath)

	return parsedData, nil
}
