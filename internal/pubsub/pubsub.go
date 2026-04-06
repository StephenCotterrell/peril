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

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	bytes, err := json.Marshal(val)
	if err != nil {
		return err
	}

	err = ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        bytes,
	})
	if err != nil {
		return err
	}

	return nil
}

func PublishGob[T any](ch *amqp.Channel, exchange, key string, val T) error {
	var buf bytes.Buffer

	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(val); err != nil {
		return err
	}

	err := ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/gob",
		Body:        buf.Bytes(),
	})
	if err != nil {
		return err
	}

	return nil
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

func SubscribeJSON[T any](conn *amqp.Connection, exchange, queueName, key string, queueType SimpleQueueType, handler func(T) Acktype) error {
	ch, _, err := DeclareAndBind(conn, exchange, queueName, key, queueType)
	if err != nil {
		return err
	}

	deliveryChan, err := ch.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	go func() {
		for msg := range deliveryChan {
			var payload T
			if err := json.Unmarshal(msg.Body, &payload); err != nil {
				fmt.Printf("failed to unmarshal message: %v\n", err)
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

			// if err := msg.Ack(false); err != nil {
			// 	fmt.Printf("There was an error acknowledging the message: %v\n", err)
			// }
		}
	}()

	return nil
}
