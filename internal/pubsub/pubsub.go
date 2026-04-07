// Package pubsub provides reusable code for interacting with rabbitmq
package pubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"

	"github.com/StephenCotterrell/peril/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func publish[T any](ch *amqp.Channel, exchange, key string, val T, contentType string, marshaller func(T) ([]byte, error)) error {
	body, err := marshaller(val)
	if err != nil {
		return err
	}

	err = ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: contentType,
		Body:        body,
	})
	if err != nil {
		return err
	}
	return nil
}

func EncodeJSON[T any](data T) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	return publish(ch, exchange, key, val, "application/json", EncodeJSON[T])
}

func EncodeGob[T any](data T) ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func PublishGob[T any](ch *amqp.Channel, exchange, key string, val T) error {
	return publish(ch, exchange, key, val, "application/gob", EncodeGob[T])
}

type SimpleQueueType string

const (
	Durable   SimpleQueueType = "durable"
	Transient SimpleQueueType = "transient"
)

type Acktype string

const (
	Ack         Acktype = "Ack"
	NackRequeue Acktype = "NackRequeue"
	NackDiscard Acktype = "NackDiscard"
)

func DeclareAndBind(conn *amqp.Connection, exchange, queueName, key string, queueType SimpleQueueType) (*amqp.Channel, amqp.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	var durable, autoDelete, exclusive, noWait bool
	switch queueType {
	case Durable:
		durable = true
		autoDelete = false
		exclusive = false
	case Transient:
		durable = false
		autoDelete = true
		exclusive = true
	}

	noWait = false

	queue, err := ch.QueueDeclare(queueName, durable, autoDelete, exclusive, noWait, amqp.Table{
		"x-dead-letter-exchange": routing.ExchangePerilDeadLetter,
	})
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	err = ch.QueueBind(queueName, key, exchange, noWait, nil)
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	return ch, queue, nil
}

func subscribe[T any](conn *amqp.Connection, exchange, queueName, key string, queueType SimpleQueueType, handler func(T) Acktype, unmarshaller func([]byte) (T, error)) error {
	ch, _, err := DeclareAndBind(conn, exchange, queueName, key, queueType)
	if err != nil {
		return err
	}

	ch.Qos(10, 0, true)
	deliveryChan, err := ch.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	go func() {
		for msg := range deliveryChan {
			payload, err := unmarshaller(msg.Body)
			if err != nil {
				fmt.Printf("Failed to unmarshal message: %v\n", err)
				// if nackErr := msg.Nack(false, false); nackErr != nil {
				// fmt.Printf("Failed to nack message: %v\n", nackErr)
				// }
				// continue
			}

			ack := handler(payload)
			switch ack {
			case Ack:
				if err := msg.Ack(false); err != nil {
					fmt.Printf("There was an error acknowledging the message: %v\n", err)
				} else {
					log.Println("Ack message sent")
				}
			case NackRequeue:
				if err := msg.Nack(false, true); err != nil {
					fmt.Printf("There was an error acknowledging the message: %v\n", err)
				} else {
					log.Println("NackRequeue message sent")
				}
			case NackDiscard:
				if err := msg.Nack(false, false); err != nil {
					fmt.Printf("There was an error acknowledging the message: %v\n", err)
				} else {
					log.Println("NackDiscard message sent")
				}
			}
		}
	}()
	return nil
}

func unmarshalJSON[T any](data []byte) (T, error) {
	var payload T
	err := json.Unmarshal(data, &payload)
	return payload, err
}

func unmarshalGob[T any](data []byte) (T, error) {
	var payload T
	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&payload)
	return payload, err
}

func SubscribeJSON[T any](conn *amqp.Connection, exchange, queueName, key string, queueType SimpleQueueType, handler func(T) Acktype) error {
	return subscribe(conn, exchange, queueName, key, queueType, handler, unmarshalJSON[T])
}

func SubscribeGob[T any](conn *amqp.Connection, exchange, queueName, key string, queueType SimpleQueueType, handler func(T) Acktype) error {
	return subscribe(conn, exchange, queueName, key, queueType, handler, unmarshalGob[T])
}
