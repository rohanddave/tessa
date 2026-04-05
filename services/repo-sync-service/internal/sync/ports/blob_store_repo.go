package ports

type BlobStoreRepo interface {
	InsertFile(filePath string, content []byte) (string, error) // returns the url of the inserted file

	GetFile(fileUrl string) ([]byte, error)

	RemoveFile(fileUrl string) error

	RemoveDirectory(directory string) error
}
