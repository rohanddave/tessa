package blobstore

import sharedutil "github.com/rohandave/tessa-rag/services/shared/util"

type BlobStorageConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          string
}

func LoadConfig() *BlobStorageConfig {
	return &BlobStorageConfig{
		Endpoint:        sharedutil.EnvOrDefault("S3_ENDPOINT", "localhost:9000"),
		Region:          sharedutil.EnvOrDefault("S3_REGION", "us-east-1"),
		Bucket:          sharedutil.EnvOrDefault("S3_BUCKET", "repo-sync"),
		AccessKeyID:     sharedutil.EnvOrDefault("S3_ACCESS_KEY_ID", "minioadmin"),
		SecretAccessKey: sharedutil.EnvOrDefault("S3_SECRET_ACCESS_KEY", "minioadmin"),
		UseSSL:          sharedutil.EnvOrDefault("S3_USE_SSL", "false"),
	}
}
