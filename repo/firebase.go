package repo

import (
	"EventBot/model"
	"context"
	"fmt"
	"log"
	"slices"

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
		return nil, err
	}

	// Get a Firestore client
	client, err := app.Firestore(ctx)
	if err != nil {
		return nil, err
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
		return "", err
	}
	return docRef.ID, nil
}

// ReadEvent reads an event from Firestore by its ID
func (fc *FirestoreConnector) ReadEvent(ctx context.Context, eventID string) (*model.Event, error) {
	doc, err := fc.client.Collection("events").Doc(eventID).Get(ctx)
	if err != nil {
		return nil, err
	}

	var event model.Event
	err = doc.DataTo(&event)
	if err != nil {
		return nil, fmt.Errorf("error converting document data to event: %w", err)
	}
	return &event, nil
}

// UpdateEvent updates an existing event in Firestore
func (fc *FirestoreConnector) UpdateEvent(ctx context.Context, eventID string, event model.Event) error {
	_, err := fc.client.Collection("events").Doc(eventID).Set(ctx, event)
	if err != nil {
		return err
	}
	return nil
}

// DeleteEvent deletes an event from Firestore by its ID
func (fc *FirestoreConnector) DeleteEvent(ctx context.Context, eventID string) error {
	_, err := fc.client.Collection("events").Doc(eventID).Delete(ctx)
	if err != nil {
		return err
	}
	return nil
}

// ListEventsByUserID now lists events where the user is owner or coowner
func (fc *FirestoreConnector) ListEventsByUserID(ctx context.Context, userID int64) ([]model.Event, error) {
	// Get all events where the user is the primary owner
	ownerIter := fc.client.Collection("events").Where("userid", "==", userID).Documents(ctx)

	// Get all events where the user is a coowner
	coownerIter := fc.client.Collection("events").Where("coowners", "array-contains", userID).Documents(ctx)

	// Collect events
	var events []model.Event

	// Process owner events
	for {
		doc, err := ownerIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var event model.Event
		err = doc.DataTo(&event)
		if err != nil {
			log.Printf("error converting document data to event: %v", err)
			continue
		}
		event.ID = doc.Ref.ID
		events = append(events, event)
	}

	// Process coowner events
	for {
		doc, err := coownerIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var event model.Event
		err = doc.DataTo(&event)
		if err != nil {
			log.Printf("error converting document data to event: %v", err)
			continue
		}
		event.ID = doc.Ref.ID

		// Avoid duplicates
		duplicate := false
		for _, existingEvent := range events {
			if existingEvent.ID == event.ID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			events = append(events, event)
		}
	}

	return events, nil
}

// CreateParticipant adds a new participant to the participants collection and then adds the participant's ID to the event's participant list
func (fc *FirestoreConnector) CreateParticipant(ctx context.Context, eventID string, participant *model.Participant) error {
	// Check if the event exists
	event, err := fc.ReadEvent(ctx, eventID)
	if err != nil {
		return model.ErrEventDoesNotExist
	}

	// Check if the participant already exists for the event
	existingParticipant, err := fc.ReadParticipantByUserID(ctx, participant.UserID)
	if err == nil && existingParticipant != nil {
		log.Printf("Participant with userID '%d' already exists", participant.UserID)

		// Check if the participant is already signed up for the event
		exist := false
		for i := range existingParticipant.SignedUpEvents {
			if existingParticipant.SignedUpEvents[i].EventID == eventID {
				exist = true
				break
			}
		}

		if !exist {
			existingParticipant.SignedUpEvents = append(existingParticipant.SignedUpEvents, model.SignedUpEvent{
				EventID:       eventID,
				PersonalNotes: "",
				CheckedIn:     false,
			})
			err = fc.UpdateParticipant(ctx, *existingParticipant)
			if err != nil {
				return err
			}
		}

		participant = existingParticipant
	} else if err != nil {
		return err
	} else {
		participant.SignedUpEvents = append(participant.SignedUpEvents, model.SignedUpEvent{
			EventID:       eventID,
			PersonalNotes: "",
			CheckedIn:     false,
		})
		docRef, _, err := fc.client.Collection("participants").Add(ctx, participant)
		if err != nil {
			return err
		}

		participant.ID = docRef.ID
		err = fc.UpdateParticipant(ctx, *participant)
		if err != nil {
			return err
		}
	}

	if !slices.Contains(event.Participants, participant.ID) {
		// Add the participant's ID to the event's participants list
		event.Participants = append(event.Participants, participant.ID)
		err = fc.UpdateEvent(ctx, eventID, *event)
		if err != nil {
			return err
		}
	}

	return nil
}

// ReadParticipant reads a participant from an event in Firestore by their code
func (fc *FirestoreConnector) ReadParticipantByID(ctx context.Context, participantID string) (*model.Participant, error) {
	doc, err := fc.client.Collection("participants").Doc(participantID).Get(ctx)
	if err != nil {
		return nil, err
	}

	var participant model.Participant
	err = doc.DataTo(&participant)
	if err != nil {
		return nil, err
	}

	return &participant, nil
}

// ReadParticipant reads a participant from an event in Firestore by their code
func (fc *FirestoreConnector) ReadParticipantByUserID(ctx context.Context, userID int64) (*model.Participant, error) {
	iter := fc.client.Collection("participants").Where("userid", "==", userID).Documents(ctx)
	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, nil // Participant not found, return nil without error
		}
		return nil, err
	}

	var participant model.Participant
	err = doc.DataTo(&participant)
	if err != nil {
		return nil, err
	}

	return &participant, nil
}

// UpdateParticipant updates an existing participant in an event in Firestore
func (fc *FirestoreConnector) UpdateParticipant(ctx context.Context, participant model.Participant) error {
	_, err := fc.client.Collection("participants").Doc(participant.ID).Set(ctx, participant)
	if err != nil {
		return err
	}

	return nil
}

// DeleteParticipant deletes a participant from an event in Firestore by their code
func (fc *FirestoreConnector) DeleteParticipant(ctx context.Context, eventID string, participantCode string) error {
	_, err := fc.client.Collection("participants").Doc(participantCode).Delete(ctx)
	if err != nil {
		return err
	}

	return nil
}

// ListParticipants lists all participants from an event in Firestore
func (fc *FirestoreConnector) ListParticipants(ctx context.Context, eventID string) ([]model.Participant, error) {
	// Check if the event exists
	event, err := fc.ReadEvent(ctx, eventID)
	if err != nil {
		return nil, model.ErrEventDoesNotExist
	}

	var participants []model.Participant
	for _, participantID := range event.Participants {
		participant, err := fc.ReadParticipantByID(ctx, participantID)
		if err != nil {
			log.Printf("error reading participant with ID '%s': %v", participantID, err)
			continue // Skip this participant and continue with the next one
		}
		participants = append(participants, *participant)
	}
	return participants, nil
}

func (fc *FirestoreConnector) ListEventsByParticipantUserID(ctx context.Context, userID int64) ([]model.Event, error) {
	// First, get the participant to check if they exist
	participant, err := fc.ReadParticipantByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if participant == nil {
		return nil, model.ErrParticipantDoesNotExist
	}

	participantEvents := make([]model.Event, 0, len(participant.SignedUpEvents))

	for i := range participant.SignedUpEvents {
		event, err := fc.ReadEvent(ctx, participant.SignedUpEvents[i].EventID)
		if err != nil {
			return participantEvents, err
		}

		participantEvents = append(participantEvents, *event)
	}

	return participantEvents, nil
}

// Close closes the Firestore client
func (fc *FirestoreConnector) Close() error {
	return fc.client.Close()
}

// IsEventOwner checks if the user is the owner or a coowner of the event
func (fc *FirestoreConnector) IsEventOwner(ctx context.Context, eventID string, userID int64) (bool, error) {
	event, err := fc.ReadEvent(ctx, eventID)
	if err != nil {
		return false, err
	}

	// Check if user is primary owner or in coowners list
	return event.UserID == userID || slices.Contains(event.Coowners, userID), nil
}

// AddCoowner adds a coowner to an event
func (fc *FirestoreConnector) AddCoowner(ctx context.Context, eventID string, primaryOwnerID, coownerID int64) error {
	// First, verify that the user trying to add a coowner is the primary owner
	event, err := fc.ReadEvent(ctx, eventID)
	if err != nil {
		return err
	}

	// Check if the primary owner is actually the owner
	if event.UserID != primaryOwnerID {
		return fmt.Errorf("only the primary owner can add coowners")
	}

	// Check if the coowner is already in the list
	if slices.Contains(event.Coowners, coownerID) {
		return fmt.Errorf("user is already a coowner")
	}

	// Add the coowner
	event.Coowners = append(event.Coowners, coownerID)

	// Update the event
	return fc.UpdateEvent(ctx, eventID, *event)
}
