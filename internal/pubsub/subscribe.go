package pubsub

import (
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type AckType int

const (
	Ack AckType = iota
	NackRequeue
	NackDiscard
)

func SubscribeJSON[T any](conn *amqp.Connection, exchange, queueName, key string, queueType QueueType, handler func(T) AckType) error {
	ch, queue, err := DeclareAndBind(conn, exchange, queueName, key, queueType)
	if err != nil {
		return err
	}

	err = ch.Qos(5, 0, false)
	if err != nil {
		return err
	}

	msgCh, err := ch.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	go func() {
		for m := range msgCh {
			var msg T
			err := json.Unmarshal(m.Body, &msg)
			if err != nil {
				log.Printf("error unmarshal msg body: %v\n", err)
				continue
			}
			ackType := handler(msg)
			switch ackType {
			case Ack:
				m.Ack(false)
			case NackRequeue:
				m.Nack(false, true)
			case NackDiscard:
				m.Nack(false, false)
			}
		}
	}()
	return nil
}
