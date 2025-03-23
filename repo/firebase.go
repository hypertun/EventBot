package repo

import (
	"EventBot/model"
	"context"
	"fmt"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/db"
	"google.golang.org/api/option"
)

// FirebaseConnector struct to hold Firebase client and database reference
type FirebaseConnector struct {
	app    *firebase.App
	client *db.Client
}

// NewFirebaseConnector creates a new Firebase connector
func NewFirebaseConnector(ctx context.Context, serviceAccountKeyPath string, databaseURL string) (*FirebaseConnector, error) {
	// Load the service account key file
	opt := option.WithCredentialsFile(serviceAccountKeyPath)

	// Initialize the Firebase app
	config := &firebase.Config{
		DatabaseURL: databaseURL,
	}
	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		return nil, fmt.Errorf("error initializing Firebase app: %v", err)
	}

	// Get a database client
	client, err := app.Database(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting database client: %v", err)
	}

	return &FirebaseConnector{
		app:    app,
		client: client,
	}, nil
}

// CreateEvent creates a new event in Firebase
func (fc *FirebaseConnector) CreateEvent(ctx context.Context, event model.Event) (string, error) {
	ref := fc.client.NewRef("events")
	newRef, err := ref.Push(ctx, event)
	if err != nil {
		return "", fmt.Errorf("error creating event: %v", err)
	}
	return newRef.Key, nil
}

// ReadEvent reads an event from Firebase by its ID
func (fc *FirebaseConnector) ReadEvent(ctx context.Context, eventID string) (*model.Event, error) {
	ref := fc.client.NewRef("events").Child(eventID)
	var event model.Event
	err := ref.Get(ctx, &event)
	if err != nil {
		return nil, fmt.Errorf("error reading event: %v", err)
	}
	return &event, nil
}

// UpdateEvent updates an existing event in Firebase
func (fc *FirebaseConnector) UpdateEvent(ctx context.Context, eventID string, event model.Event) error {
	ref := fc.client.NewRef("events").Child(eventID)
	err := ref.Set(ctx, event)
	if err != nil {
		return fmt.Errorf("error updating event: %v", err)
	}
	return nil
}

// DeleteEvent deletes an event from Firebase by its ID
func (fc *FirebaseConnector) DeleteEvent(ctx context.Context, eventID string) error {
	ref := fc.client.NewRef("events").Child(eventID)
	err := ref.Delete(ctx)
	if err != nil {
		return fmt.Errorf("error deleting event: %v", err)
	}
	return nil
}

// ListEvents lists all events from Firebase
func (fc *FirebaseConnector) ListEvents(ctx context.Context) ([]model.Event, error) {
	ref := fc.client.NewRef("events")
	var events map[string]model.Event
	err := ref.Get(ctx, &events)
	if err != nil {
		return nil, fmt.Errorf("error listing events: %v", err)
	}

	var eventList []model.Event
	for key, event := range events {
		event.DocumentID = key
		eventList = append(eventList, event)
	}

	return eventList, nil
}

// Close closes the Firebase app
func (fc *FirebaseConnector) Close() error {
	return fc.Close()
}

// InitializeFirebase initializes the Firebase connector and returns it
func InitializeFirebase(ctx context.Context) (*FirebaseConnector, error) {
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
