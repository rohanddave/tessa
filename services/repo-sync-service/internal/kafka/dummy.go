package kafka

import (
	"fmt"
	"sync"

	reposync "github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync"
)

type DummyProducer struct {
	mu     sync.Mutex
	events []reposync.RepoEvent
}

func NewDummyProducer() Producer {
	return &DummyProducer{
		events: make([]reposync.RepoEvent, 0),
	}
}

func (d *DummyProducer) Produce(event *reposync.RepoEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.events = append(d.events, *event)
	return nil
}

func (d *DummyProducer) Close() {
	// no-op
}

type DummyConsumer struct{}

func NewDummyConsumer() Consumer {
	return &DummyConsumer{}
}
