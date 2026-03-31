package kafka

import (
	"context"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync"
)

type Producer interface {
	PublishRepoUpdated(context.Context, sync.RepoEvent) error

	PublishRepoCreated(context.Context, sync.RepoEvent) error

	PublishRepoDeleted(context.Context, sync.RepoEvent) error
}

type DummyProducer struct {
	topic string
}

func NewDummyProducer(topic string) DummyProducer {
	return DummyProducer{topic: topic}
}

func (p DummyProducer) PublishRepoCreated(_ context.Context, _ sync.RepoEvent) error {
	return nil
}

func (p DummyProducer) PublishRepoUpdated(_ context.Context, _ sync.RepoEvent) error {
	return nil
}

func (p DummyProducer) PublishRepoDeleted(_ context.Context, _ sync.RepoEvent) error {
	return nil
}

func (p DummyProducer) Topic() string {
	return p.topic
}

type DummyConsumer struct {
	topic string
}

func NewDummyConsumer(topic string) DummyConsumer {
	return DummyConsumer{topic: topic}
}

func (c DummyConsumer) Topic() string {
	return c.topic
}
