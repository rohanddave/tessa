package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	cfkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/util"
)

type KafkaProducerConfig struct {
	Brokers string
}

type Producer interface {
	Produce(*sync.RepoEvent) error

	Close()
}

type KafkaProducerAdapter struct {
	config   *KafkaProducerConfig
	producer *cfkafka.Producer
}

func NewKafkaProducer(config *KafkaProducerConfig) Producer {
	p, err := cfkafka.NewProducer(&cfkafka.ConfigMap{
		"bootstrap.servers": config.Brokers,
	})
	if err != nil {
		panic(err)
	}
	return &KafkaProducerAdapter{config: config, producer: p}
}

func (p *KafkaProducerAdapter) getTopic(event *sync.RepoEvent) (string, error) {
	switch event.EventType {
	case "repo.created", "repo.deleted":
		return util.EnvOrDefault("KAFKA_LIFE_CYCLE_TOPIC", "repo-sync.repo-lifecycle"), nil
	case "repo.updated":
		return util.EnvOrDefault("KAFKA_EVENTS_TOPIC", "repo-sync.repo-events"), nil
	default:
		return "", fmt.Errorf("unsupported event type: %s", event.EventType)
	}
}

func (p *KafkaProducerAdapter) getPartitionKey(event *sync.RepoEvent) (string, error) {
	// Use repo URL as partition key to ensure events for the same repo go to the same partition
	switch event.EventType {
	case "repo.created", "repo.deleted":
		return event.RepoURL, nil
	case "repo.updated":
		return event.RepoURL + ":" + event.Branch, nil
	default:
		return "", fmt.Errorf("unsupported event type: %s", event.EventType)
	}
}

func (p *KafkaProducerAdapter) Produce(event *sync.RepoEvent) error {
	topic, err := p.getTopic(event)
	if err != nil {
		return err
	}

	partitionKey, err := p.getPartitionKey(event)
	if err != nil {
		return err
	}

	value, err := json.Marshal(event)
	if err != nil {
		return err
	}

	deliveryChan := make(chan cfkafka.Event, 1)
	defer close(deliveryChan)

	err = p.producer.Produce(&cfkafka.Message{
		TopicPartition: cfkafka.TopicPartition{Topic: &topic, Partition: cfkafka.PartitionAny},
		Key:            []byte(partitionKey),
		Value:          value,
	}, deliveryChan)
	if err != nil {
		return err
	}

	deliveryEvent := <-deliveryChan
	deliveredMessage, ok := deliveryEvent.(*cfkafka.Message)
	if !ok {
		return fmt.Errorf("unexpected kafka delivery event type %T", deliveryEvent)
	}

	if deliveredMessage.TopicPartition.Error != nil {
		return deliveredMessage.TopicPartition.Error
	}

	return nil
}

func (p *KafkaProducerAdapter) Close() {
	p.producer.Close()
}

type KafkaConsumerConfig struct {
	Brokers string
	GroupId string
}

type Consumer interface {
	SubscribeTopics(topics []string) error

	ReadMessage(timeout time.Duration) (*sync.RepoEvent, error)

	Close()
}

type KafkaConsumerAdapter struct {
	config   *KafkaConsumerConfig
	consumer *cfkafka.Consumer
}

func NewKafkaConsumer(config *KafkaConsumerConfig) Consumer {
	c, err := cfkafka.NewConsumer(&cfkafka.ConfigMap{
		"bootstrap.servers": config.Brokers,
		"group.id":          config.GroupId,
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		panic(err)
	}
	return &KafkaConsumerAdapter{config: config, consumer: c}
}

func (c *KafkaConsumerAdapter) SubscribeTopics(topics []string) error {
	return c.consumer.SubscribeTopics(topics, nil)
}

func (c *KafkaConsumerAdapter) ReadMessage(timeout time.Duration) (*sync.RepoEvent, error) {
	msg, err := c.consumer.ReadMessage(timeout)
	if err != nil {
		if kafkaErr, ok := err.(cfkafka.Error); ok && kafkaErr.Code() == cfkafka.ErrTimedOut {
			return nil, nil // No message received within timeout, return nil without error
		}
		return nil, err
	}

	var event sync.RepoEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return nil, err
	}

	return &event, nil
}

func (c *KafkaConsumerAdapter) Close() {
	c.consumer.Close()
}
