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
	"github.com/rs/zerolog/log"
)

func main() {
	// Replace "YOUR_BOT_TOKEN" with your actual bot token
	botToken := "7510313727:AAHj81Bx4Iu32B6wFqpM4i28X_Z20ZEHryA"

	_, err := InitializeFirebase(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Error initializing Firebase")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	opts := []bot.Option{
		bot.WithDefaultHandler(handler.OrganiserHandler),
	}

	b, err := bot.New(botToken, opts...)
	if err != nil {
		log.Fatal("error creating bot: ", err)
	}

	b.Start(ctx)
	<-ctx.Done()
	log.Println("Bot stopped")
}

// InitializeFirebase initializes the Firebase connector and returns it
func InitializeFirebase(ctx context.Context) (*repo.FirebaseConnector, error) {
	// Get the service account key path from environment variable
	serviceAccountKeyPath := os.Getenv("FIREBASE_SERVICE_ACCOUNT_KEY_PATH")
	if serviceAccountKeyPath == "" {
		return nil, fmt.Errorf("FIREBASE_SERVICE_ACCOUNT_KEY_PATH environment variable not set")
	}

	// Get the database URL from environment variable
	databaseURL := os.Getenv("FIREBASE_DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("FIREBASE_DATABASE_URL environment variable not set")
	}

	// Create a new Firebase connector
	firebaseConnector, err := NewFirebaseConnector(ctx, serviceAccountKeyPath, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("error creating Firebase connector: %v", err)
	}

	return firebaseConnector, nil
}
