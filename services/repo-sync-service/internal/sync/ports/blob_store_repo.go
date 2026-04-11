package ports

import shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"

type BlobStoreRepo interface {
	InsertFile(filePath string, content []byte, extension string) (string, error) // returns the url of the inserted file

	GetFile(fileUrl string) (*shareddomain.FileJob, error)
	GetFiles(fileUrls []string, jobs chan<- *shareddomain.FileJob, workerCount int) error

	RemoveFile(fileUrl string) error

	RemoveDirectory(directory string) error
}
