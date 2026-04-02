package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/StephenCotterrell/peril/internal/gamelogic"
	"github.com/StephenCotterrell/peril/internal/pubsub"
	"github.com/StephenCotterrell/peril/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Starting Peril client...")

	connString := "amqp://guest:guest@localhost:5672/"

	conn, err := amqp.Dial(connString)
	if err != nil {
		log.Fatal("Failed to establish connection")
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Fatal("There was an error closing the connection")
		}
	}()

	fmt.Println("Connection was established successfully")

	userName, err := gamelogic.ClientWelcome()
	if err != nil {
		log.Fatal("failed to get username")
	}

	queueName := fmt.Sprintf("%s.%s", routing.PauseKey, userName)

	_, _, err = pubsub.DeclareAndBind(conn, routing.ExchangePerilDirect, queueName, routing.PauseKey, pubsub.Transient)
	if err != nil {
		log.Fatal("there was an error binding the connection to the queue")
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	fmt.Println("Shutting down Peril client server...")
	os.Exit(0)
}
