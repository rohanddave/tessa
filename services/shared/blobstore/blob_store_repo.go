package blobstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"strings"
	"sync"

	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
)

type Repo struct {
	client   *miniosdk.Client
	endpoint string
	bucket   string
	useSSL   bool
}

const extensionMetadataKey = "extension"

func NewRepo() (*Repo, error) {
	cfg := LoadConfig()
	useSSL := strings.EqualFold(cfg.UseSSL, "true")

	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: useSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &Repo{
		client:   client,
		endpoint: cfg.Endpoint,
		bucket:   cfg.Bucket,
		useSSL:   useSSL,
	}, nil
}

func (r *Repo) InsertFile(filePath string, content []byte, extension string) (string, error) {
	objectKey := normalizeObjectKey(filePath)
	normalizedExtension := normalizeExtension(extension)
	if normalizedExtension == "" {
		normalizedExtension = normalizeExtension(path.Ext(objectKey))
	}
	contentType := detectContentType(normalizedExtension, objectKey)

	_, err := r.client.PutObject(
		context.Background(),
		r.bucket,
		objectKey,
		bytes.NewReader(content),
		int64(len(content)),
		miniosdk.PutObjectOptions{
			ContentType: contentType,
			UserMetadata: map[string]string{
				extensionMetadataKey: normalizedExtension,
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("put object %q: %w", objectKey, err)
	}

	return r.objectURL(objectKey), nil
}

func (r *Repo) GetFile(fileURL string) (*shareddomain.FileJob, error) {
	objectKey, err := r.resolveObjectKey(fileURL)
	if err != nil {
		return nil, err
	}

	objectInfo, err := r.client.StatObject(context.Background(), r.bucket, objectKey, miniosdk.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("stat object %q: %w", objectKey, err)
	}

	object, err := r.client.GetObject(context.Background(), r.bucket, objectKey, miniosdk.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", objectKey, err)
	}
	defer object.Close()

	content, err := io.ReadAll(object)
	if err != nil {
		return nil, fmt.Errorf("read object %q: %w", objectKey, err)
	}

	return &shareddomain.FileJob{
		Path:      objectKey,
		Size:      objectInfo.Size,
		Content:   content,
		Extension: extractValueFromMetadataForKey(objectInfo.UserMetadata, extensionMetadataKey, objectKey),
	}, nil
}

func (r *Repo) GetFiles(fileURLs []string, jobs chan<- *shareddomain.FileJob, workerCount int) error {
	if workerCount <= 0 {
		workerCount = 1
	}

	if len(fileURLs) == 0 {
		return nil
	}

	work := make(chan string, len(fileURLs))
	errCh := make(chan error, workerCount)

	var wg sync.WaitGroup

	for _, fileURL := range fileURLs {
		work <- fileURL
	}
	close(work)

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for fileURL := range work {
				fileJob, err := r.GetFile(fileURL)
				if err != nil {
					errCh <- fmt.Errorf("get file for url %q: %w", fileURL, err)
					return
				}
				jobs <- fileJob
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Repo) RemoveFile(fileURL string) error {
	objectKey, err := r.resolveObjectKey(fileURL)
	if err != nil {
		return err
	}

	err = r.client.RemoveObject(context.Background(), r.bucket, objectKey, miniosdk.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("remove object %q: %w", objectKey, err)
	}

	return nil
}

func (r *Repo) RemoveDirectory(directory string) error {
	prefix, err := r.resolveObjectKey(directory)
	if err != nil {
		return err
	}

	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	objectsCh := make(chan miniosdk.ObjectInfo)
	go func() {
		defer close(objectsCh)

		for object := range r.client.ListObjects(context.Background(), r.bucket, miniosdk.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		}) {
			objectsCh <- object
		}
	}()

	for removeErr := range r.client.RemoveObjects(context.Background(), r.bucket, objectsCh, miniosdk.RemoveObjectsOptions{}) {
		if removeErr.Err != nil {
			return fmt.Errorf("remove object %q under prefix %q: %w", removeErr.ObjectName, prefix, removeErr.Err)
		}
	}

	return nil
}

func (r *Repo) resolveObjectKey(fileURL string) (string, error) {
	if fileURL == "" {
		return "", fmt.Errorf("file url cannot be empty")
	}

	if !strings.Contains(fileURL, "://") {
		return normalizeObjectKey(fileURL), nil
	}

	parsed, err := url.Parse(fileURL)
	if err != nil {
		return "", fmt.Errorf("parse file url %q: %w", fileURL, err)
	}

	prefix := "/" + r.bucket + "/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		return "", fmt.Errorf("file url %q does not belong to bucket %q", fileURL, r.bucket)
	}

	objectKey := strings.TrimPrefix(parsed.Path, prefix)
	return normalizeObjectKey(objectKey), nil
}

func (r *Repo) objectURL(objectKey string) string {
	scheme := "http"
	if r.useSSL {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s/%s/%s", scheme, r.endpoint, r.bucket, objectKey)
}

func normalizeObjectKey(objectKey string) string {
	return strings.TrimPrefix(path.Clean("/"+objectKey), "/")
}

func detectContentType(extension string, objectKey string) string {
	contentType := mime.TypeByExtension("." + normalizeExtension(extension))
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(objectKey))
	}
	if contentType == "" {
		return "application/octet-stream"
	}

	return contentType
}

func normalizeExtension(extension string) string {
	return strings.TrimPrefix(strings.TrimSpace(extension), ".")
}

func extractValueFromMetadataForKey(metadata map[string]string, metadataKey string, objectKey string) string {
	for key, value := range metadata {
		if strings.EqualFold(key, metadataKey) || strings.EqualFold(key, "X-Amz-Meta-"+metadataKey) {
			return normalizeExtension(value)
		}
	}

	return normalizeExtension(path.Ext(objectKey))
}
