package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"strings"

	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
)

type BlobStoreRepo struct {
	client   *miniosdk.Client
	endpoint string
	bucket   string
	useSSL   bool
}

func NewBlobStoreRepo(cfg config.StorageConfig) (ports.BlobStoreRepo, error) {
	useSSL := strings.EqualFold(cfg.UseSSL, "true")

	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: useSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &BlobStoreRepo{
		client:   client,
		endpoint: cfg.Endpoint,
		bucket:   cfg.Bucket,
		useSSL:   useSSL,
	}, nil
}

func (r *BlobStoreRepo) InsertFile(filePath string, content []byte) (string, error) {
	objectKey := normalizeObjectKey(filePath)
	contentType := detectContentType(objectKey)

	_, err := r.client.PutObject(
		context.Background(),
		r.bucket,
		objectKey,
		bytes.NewReader(content),
		int64(len(content)),
		miniosdk.PutObjectOptions{
			ContentType: contentType,
		},
	)
	if err != nil {
		return "", fmt.Errorf("put object %q: %w", objectKey, err)
	}

	return r.objectURL(objectKey), nil
}

func (r *BlobStoreRepo) GetFile(fileURL string) ([]byte, error) {
	objectKey, err := r.resolveObjectKey(fileURL)
	if err != nil {
		return nil, err
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

	return content, nil
}

func (r *BlobStoreRepo) RemoveFile(fileURL string) error {
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

func (r *BlobStoreRepo) resolveObjectKey(fileURL string) (string, error) {
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

func (r *BlobStoreRepo) objectURL(objectKey string) string {
	scheme := "http"
	if r.useSSL {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s/%s/%s", scheme, r.endpoint, r.bucket, objectKey)
}

func normalizeObjectKey(objectKey string) string {
	return strings.TrimPrefix(path.Clean("/"+objectKey), "/")
}

func detectContentType(objectKey string) string {
	contentType := mime.TypeByExtension(path.Ext(objectKey))
	if contentType == "" {
		return "application/octet-stream"
	}

	return contentType
}
