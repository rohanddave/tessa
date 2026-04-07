package domain

type FileJob struct {
	Path    string
	Size    int64
	Content []byte
}
