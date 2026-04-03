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
	fmt.Println("Starting Peril server...")
	gamelogic.PrintServerHelp()

	connString := "amqp://guest:guest@localhost:5672/"
	conn, err := amqp.Dial(connString)
	if err != nil {
		log.Fatal("Failed to establish a connection")
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Fatal("There was an error closing the connection")
		}
	}()

	fmt.Println("Connection was established successfully!")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		<-signalChan
		fmt.Println("Shutting down Peril server...")
		os.Exit(0)
	}()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal("failed to create channel")
	}

REPL:
	for {
		input := gamelogic.GetInput()
		switch input[0] {
		case "pause":
			log.Println("Sending a pause message")
			if err = pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
				IsPaused: true,
			}); err != nil {
				log.Fatal("Failed to publish JSON message")
			}
		case "resume":
			log.Println("Sending a resume message")
			if err = pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
				IsPaused: false,
			}); err != nil {
				log.Fatal("Failed to publish JSON message")
			}
		case "quit":
			log.Println("Exiting...")
			break REPL
		default:
			log.Println("invalid command")
		}
	}
}
