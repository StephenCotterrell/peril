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

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal("failed to create channel")
	}

	// subscribing to the pause command
	pauseQueueName := fmt.Sprintf("%s.%s", routing.PauseKey, userName)

	if err = pubsub.SubscribeJSON(conn, routing.ExchangePerilDirect, pauseQueueName, routing.PauseKey, pubsub.Transient, handlerPause(gameState)); err != nil {
		fmt.Printf("there was an error subscribing to the pause exchange: %v", err)
	}

	// subscribing to the move command
	moveQueueName := fmt.Sprintf("%s.%s", routing.ArmyMovesPrefix, userName)

	if err = pubsub.SubscribeJSON(conn, routing.ExchangePerilTopic, moveQueueName, routing.ArmyMovesPrefix+".*", pubsub.Transient, handlerArmyMoves(gameState, ch)); err != nil {
		fmt.Printf("there was an error subscribing to the army_moves exchange: %v", err)
	}

	// subscribing to all moves

	if err = pubsub.SubscribeJSON(conn, routing.ExchangePerilTopic, routing.WarRecognitionsPrefix, routing.WarRecognitionsPrefix+".*", pubsub.Durable, handlerAllMoves(gameState)); err != nil {
		fmt.Printf("there was an error subscribing to the all moves exchange: %v", err)
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
			move, err := gameState.CommandMove(input)
			if err != nil {
				fmt.Printf("error executing move command: %v\n", err)
				continue
			}

			err = pubsub.PublishJSON(ch, routing.ExchangePerilTopic, moveQueueName, move)
			if err != nil {
				fmt.Printf("error publishing move: %v\n", err)
				continue
			}

			fmt.Printf("move was successfull: %v\n", move)

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

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.Acktype {
	return func(ps routing.PlayingState) pubsub.Acktype {
		defer fmt.Printf("> ")
		gs.HandlePause(ps)
		return pubsub.Ack
	}
}

func handlerArmyMoves(gs *gamelogic.GameState, ch *amqp.Channel) func(gamelogic.ArmyMove) pubsub.Acktype {
	return func(move gamelogic.ArmyMove) pubsub.Acktype {
		defer fmt.Printf("> ")
		moveOutcome := gs.HandleMove(move)
		switch moveOutcome {
		case gamelogic.MoveOutComeSafe:
			return pubsub.Ack
		case gamelogic.MoveOutcomeMakeWar:
			exchangeKey := fmt.Sprintf("%s.%s", routing.WarRecognitionsPrefix, move.Player.Username)
			err := pubsub.PublishJSON(ch, routing.ExchangePerilTopic, exchangeKey, gamelogic.RecognitionOfWar{
				Attacker: move.Player,
				Defender: gs.GetPlayerSnap(),
			})
			if err != nil {
				fmt.Printf("There was an error publishing the move outcome: %v\n", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.MoveOutcomeSamePlayer:
			return pubsub.NackDiscard
		default:
			return pubsub.NackDiscard
		}
	}
}

func handlerAllMoves(gs *gamelogic.GameState) func(gamelogic.RecognitionOfWar) pubsub.Acktype {
	return func(recog gamelogic.RecognitionOfWar) pubsub.Acktype {
		defer fmt.Printf("> ")
		warOutcome, _, _ := gs.HandleWar(recog)
		switch warOutcome {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.NackRequeue
		case gamelogic.WarOutcomeNoUnits:
			return pubsub.Ack
		case gamelogic.WarOutcomeOpponentWon:
			return pubsub.Ack
		case gamelogic.WarOutcomeYouWon:
			return pubsub.Ack
		case gamelogic.WarOutcomeDraw:
			return pubsub.Ack
		default:
			fmt.Println("move not recognized")
			return pubsub.NackDiscard
		}
	}
}
