package kafka

import (
	"fmt"
	"time"

	cfkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type ProducedMessage struct {
	Topic string
	Key   []byte
	Value []byte
}

type Producer interface {
	Produce(topic string, key []byte, value []byte) error
	Close()
}

type producer struct {
	client *cfkafka.Producer
}

func NewProducer() (Producer, error) {
	config := LoadConfig()

	client, err := cfkafka.NewProducer(&cfkafka.ConfigMap{
		"bootstrap.servers": config.Brokers,
	})
	if err != nil {
		return nil, err
	}

	return &producer{client: client}, nil
}

func (p *producer) Produce(topic string, key []byte, value []byte) error {
	deliveryChan := make(chan cfkafka.Event, 1)
	defer close(deliveryChan)

	err := p.client.Produce(&cfkafka.Message{
		TopicPartition: cfkafka.TopicPartition{Topic: &topic, Partition: cfkafka.PartitionAny},
		Key:            key,
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

func (p *producer) Close() {
	p.client.Close()
}

type ConsumerConfig struct {
	GroupID           string
	MaxPollIntervalMs int
}

type Message struct {
	Key   []byte
	Value []byte
	raw   *cfkafka.Message
}

type Consumer interface {
	SubscribeTopics(topics []string) error
	ReadMessage(timeout time.Duration) (*Message, error)
	CommitMessage(message *Message) error
	Close()
}

type consumer struct {
	client *cfkafka.Consumer
}

func NewConsumer(config *ConsumerConfig) (Consumer, error) {
	sharedConfig := LoadConfig()

	configMap := &cfkafka.ConfigMap{
		"bootstrap.servers":  sharedConfig.Brokers,
		"group.id":           config.GroupID,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
	}
	if config.MaxPollIntervalMs > 0 {
		_ = configMap.SetKey("max.poll.interval.ms", config.MaxPollIntervalMs)
	}

	client, err := cfkafka.NewConsumer(configMap)
	if err != nil {
		return nil, err
	}

	return &consumer{client: client}, nil
}

func (c *consumer) SubscribeTopics(topics []string) error {
	return c.client.SubscribeTopics(topics, nil)
}

func (c *consumer) ReadMessage(timeout time.Duration) (*Message, error) {
	msg, err := c.client.ReadMessage(timeout)
	if err != nil {
		if kafkaErr, ok := err.(cfkafka.Error); ok && kafkaErr.Code() == cfkafka.ErrTimedOut {
			return nil, nil
		}
		return nil, err
	}

	return &Message{
		Key:   msg.Key,
		Value: msg.Value,
		raw:   msg,
	}, nil
}

func (c *consumer) CommitMessage(message *Message) error {
	if message == nil || message.raw == nil {
		return fmt.Errorf("message cannot be nil")
	}

	_, err := c.client.CommitMessage(message.raw)
	return err
}

func (c *consumer) Close() {
	c.client.Close()
}
