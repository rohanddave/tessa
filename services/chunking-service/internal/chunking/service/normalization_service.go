package service

import (
	"fmt"
	"strings"

	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
)

type NormalizationServiceInput struct {
}

type NormalizationService struct {
}

func NewNormalizationService(input *NormalizationServiceInput) *NormalizationService {
	return &NormalizationService{}
}

func (s *NormalizationService) NormalizeFileContent(fileJob *shareddomain.FileJob) (*shareddomain.FileJob, error) {
	if fileJob == nil {
		return nil, fmt.Errorf("file job cannot be nil")
	}

	normalizedContent := strings.TrimSpace(string(fileJob.Content))
	fileJob.Content = []byte(normalizedContent)
	fileJob.Size = int64(len(fileJob.Content))
	strings.TrimSpace(string(fileJob.Extension))

	return fileJob, nil
}
