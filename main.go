package main

import (
	"EventBot/handler"
	"EventBot/repo"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-telegram/bot"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

func main() {
	err := godotenv.Load("deploy.env")
	if err != nil {
		log.Fatal().Msgf("Error loading .env file: %v", err)
	}

	organiserBotToken := os.Getenv("ORGANISER_BOT_TOKEN")
	if organiserBotToken == "" {
		log.Fatal().Msg("ORGANISER_BOT_TOKEN environment variable not set")
	}

	participantBotToken := os.Getenv("PARTICIPANT_BOT_TOKEN")
	if participantBotToken == "" {
		log.Fatal().Msg("PARTICIPANT_BOT_TOKEN environment variable not set")
	}

	firebaseConnector, err := InitializeFirebase(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Error initializing Firebase")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	organiserBotHandler := handler.NewOrganiserBotHandler(
		*firebaseConnector,
	)

	participantBotHandler := handler.NewParticipantBotHandler(
		*firebaseConnector,
	)

	b, err := bot.New(organiserBotToken, []bot.Option{
		bot.WithDefaultHandler(organiserBotHandler.Handler),
	}...)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating organiser bot")
	}

	c, err := bot.New(participantBotToken, []bot.Option{
		bot.WithDefaultHandler(participantBotHandler.Handler),
	}...)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating participant bot")
	}

	go b.Start(ctx)
	go c.Start(ctx)

	<-ctx.Done()
	log.Info().Msg("Bots stopped")
}

// InitializeFirebase initializes the Firebase connector and returns it
func InitializeFirebase(ctx context.Context) (*repo.FirestoreConnector, error) {
	// Get the service account key path from environment variable
	serviceAccountKeyPath := os.Getenv("FIREBASE_SA")
	if serviceAccountKeyPath == "" {
		return nil, fmt.Errorf("FIREBASE_SA environment variable not set")
	}

	// Get the project ID from environment variable
	projectID := os.Getenv("FIREBASE_PROJECT_ID") // Changed environment variable name
	if projectID == "" {
		return nil, fmt.Errorf("FIREBASE_PROJECT_ID environment variable not set")
	}

	// Create a new Firestore connector
	firestoreConnector, err := repo.NewFirestoreConnector(ctx, serviceAccountKeyPath, projectID) // Changed function name
	if err != nil {
		return nil, fmt.Errorf("error creating Firestore connector: %v", err)
	}

	return firestoreConnector, nil
}
