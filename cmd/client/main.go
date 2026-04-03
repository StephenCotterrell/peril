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

	gameState := gamelogic.NewGameState(userName)

	queueName := fmt.Sprintf("%s.%s", routing.PauseKey, userName)

	if err = pubsub.SubscribeJSON(conn, routing.ExchangePerilDirect, queueName, routing.PauseKey, pubsub.Transient, handlerPause(gameState)); err != nil {
		fmt.Printf("there was an error subscribing to the pause exchange: %v", err)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		<-signalChan
		fmt.Println("Shutting down Peril client server...")
		os.Exit(0)
	}()

	for {
		input := gamelogic.GetInput()
		switch input[0] {
		case "spawn":
			if err = gameState.CommandSpawn(input); err != nil {
				fmt.Printf("error executing spawn command: %v\n", err)
			}

		case "move":
			if move, err := gameState.CommandMove(input); err != nil {
				fmt.Printf("error executing move command: %v\n", err)
			} else {
				fmt.Printf("move was successfull: %v\n", move)
			}

		case "status":
			gameState.CommandStatus()

		case "help":
			gamelogic.PrintClientHelp()

		case "quit":
			gamelogic.PrintQuit()
			return

		case "spam":
			fmt.Println("Spamming not allowed yet!")

		default:
			fmt.Println("command not recognized")
		}
	}
}

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) {
	return func(ps routing.PlayingState) {
		defer fmt.Printf("> ")
		gs.HandlePause(ps)
	}
}
