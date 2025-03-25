package repo

import (
	"EventBot/model"
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// FirestoreConnector struct to hold Firestore client
type FirestoreConnector struct {
	app    *firebase.App
	client *firestore.Client
}

// NewFirestoreConnector creates a new Firestore connector
func NewFirestoreConnector(ctx context.Context, serviceAccountKeyPath string, projectID string) (*FirestoreConnector, error) {
	// Load the service account key file
	opt := option.WithCredentialsFile(serviceAccountKeyPath)

	// Initialize the Firebase app
	config := &firebase.Config{
		ProjectID: projectID,
	}
	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		return nil, fmt.Errorf("error initializing Firebase app: %v", err)
	}

	// Get a Firestore client
	client, err := app.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting Firestore client: %v", err)
	}

	return &FirestoreConnector{
		app:    app,
		client: client,
	}, nil
}

// CreateEvent creates a new event in Firestore
func (fc *FirestoreConnector) CreateEvent(ctx context.Context, event model.Event) (string, error) {
	// Add a new document with a generated ID
	docRef, _, err := fc.client.Collection("events").Add(ctx, event)
	if err != nil {
		return "", fmt.Errorf("error creating event: %v", err)
	}
	return docRef.ID, nil
}

// ReadEvent reads an event from Firestore by its ID
func (fc *FirestoreConnector) ReadEvent(ctx context.Context, eventID string) (*model.Event, error) {
	doc, err := fc.client.Collection("events").Doc(eventID).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("error reading event: %v", err)
	}

	var event model.Event
	err = doc.DataTo(&event)
	if err != nil {
		return nil, fmt.Errorf("error converting document data to event: %v", err)
	}
	event.DocumentID = doc.Ref.ID
	return &event, nil
}

// UpdateEvent updates an existing event in Firestore
func (fc *FirestoreConnector) UpdateEvent(ctx context.Context, eventID string, event model.Event) error {
	_, err := fc.client.Collection("events").Doc(eventID).Set(ctx, event)
	if err != nil {
		return fmt.Errorf("error updating event: %v", err)
	}
	return nil
}

// DeleteEvent deletes an event from Firestore by its ID
func (fc *FirestoreConnector) DeleteEvent(ctx context.Context, eventID string) error {
	_, err := fc.client.Collection("events").Doc(eventID).Delete(ctx)
	if err != nil {
		return fmt.Errorf("error deleting event: %v", err)
	}
	return nil
}

// ListEvents lists all events from Firestore
func (fc *FirestoreConnector) ListEvents(ctx context.Context) ([]model.Event, error) {
	iter := fc.client.Collection("events").Documents(ctx)
	var events []model.Event
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating through events: %v", err)
		}

		var event model.Event
		err = doc.DataTo(&event)
		if err != nil {
			log.Printf("error converting document data to event: %v", err)
			continue // Skip this document and continue with the next one
		}
		event.DocumentID = doc.Ref.ID
		events = append(events, event)
	}
	return events, nil
}

// Close closes the Firestore client
func (fc *FirestoreConnector) Close() error {
	return fc.client.Close()
}
