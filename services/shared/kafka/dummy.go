package kafka

import (
	"fmt"
	"sync"
	"time"
)

type DummyProducer struct {
	mu       sync.Mutex
	messages []ProducedMessage
}

func NewDummyProducer() *DummyProducer {
	return &DummyProducer{
		messages: make([]ProducedMessage, 0),
	}
}

func (d *DummyProducer) Produce(topic string, key []byte, value []byte) error {
	if topic == "" {
		return fmt.Errorf("topic cannot be empty")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.messages = append(d.messages, ProducedMessage{
		Topic: topic,
		Key:   append([]byte(nil), key...),
		Value: append([]byte(nil), value...),
	})
	return nil
}

func (d *DummyProducer) Close() {}

func (d *DummyProducer) Messages() []ProducedMessage {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]ProducedMessage, len(d.messages))
	copy(out, d.messages)
	return out
}

type DummyConsumer struct{}

func NewDummyConsumer() *DummyConsumer {
	return &DummyConsumer{}
}

func (d *DummyConsumer) SubscribeTopics(topics []string) error {
	panic("unimplemented")
}

func (d *DummyConsumer) ReadMessage(timeout time.Duration) (*Message, error) {
	panic("unimplemented")
}

func (d *DummyConsumer) CommitMessage(message *Message) error {
	panic("unimplemented")
}

func (d *DummyConsumer) Close() {}
