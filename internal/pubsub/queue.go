package pubsub

import amqp "github.com/rabbitmq/amqp091-go"

type QueueType int

const (
	QueueTypeTransient QueueType = iota
	QueueTypeDurable
)

func DeclareAndBind(conn *amqp.Connection, exchange, queueName, key string, queueType QueueType) (*amqp.Channel, amqp.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	queue, err := ch.QueueDeclare(queueName, queueType == QueueTypeDurable, queueType != QueueTypeDurable, queueType != QueueTypeDurable, false, nil)
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	err = ch.QueueBind(queue.Name, key, exchange, false, nil)
	if err != nil {
		return nil, amqp.Queue{}, err
	}
	return ch, queue, nil
}
