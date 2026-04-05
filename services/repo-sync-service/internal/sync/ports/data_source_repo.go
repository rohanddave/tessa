package ports

import "io"

type DataSourceRepo interface {
	OpenRepoArchive(input OpenRepoFileStreamInput) (RepoFileStream, error)
	ComputeDiff(oldSHA string, newSHA string) (bool, error)
}

type OpenRepoFileStreamInput struct {
	RepoURL   string
	Branch    string
	CommitSHA string
}

type RepoFileStream interface {
	Next() (*RepoFile, error)
	Close() error
	CommitSHA() string
}

type RepoFile struct {
	Path   string
	Size   int64
	Reader io.ReadCloser
}
