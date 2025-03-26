package handler

import (
	"EventBot/model"
	"EventBot/repo"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type OrganiserBotHandler struct {
	FirebaseConnector repo.FirestoreConnector
}

func NewOrganiserBotHandler(
	FirebaseConnector repo.FirestoreConnector,
) *OrganiserBotHandler {
	return &OrganiserBotHandler{
		FirebaseConnector: FirebaseConnector,
	}
}

// Event creation states
const (
	StateIdle = iota
	StateAddingEventName
	StateAddingEventPicture
	StateAddingEventDetails
	StateDeleteEvent
	StateListParticipants
	StateAddingEventDetailsAnswer = iota + 10
)

// User state data
type UserState struct {
	State        int
	CurrentEvent model.Event
	LastQuestion string // Store the last question asked
	EventRefKey  string // Store the event's reference key
}

// Global map to store user states
var userStates = make(map[int64]*UserState)

func (o *OrganiserBotHandler) Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	log.Printf("[%s] %s", update.Message.From.Username, update.Message.Text)

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID

	// Get or create user state
	userState, ok := userStates[userID]
	if !ok {
		userState = &UserState{State: StateIdle}
		userStates[userID] = userState
	}

	var text string
	var params *bot.SendMessageParams

	switch userState.State {
	case StateIdle:
		switch update.Message.Text {
		case "/start":
			text = "Hello! I'm your EventBot. Use /addEvent to create a new event, or /deleteEvent to delete an existing event."
		case "/help":
			text = "I'm your EventBot. I can help you manage events. Use /addEvent to start creating an event, or /deleteEvent to delete an existing event."
		case "/addEvent":
			text = "Okay, let's create a new event. What's the name of the event?"
			userState.State = StateAddingEventName
		case "/listParticipants":
			text = "Okay, let's list the participants of an event. Please provide the Event Ref Key of the event."
			userState.State = StateListParticipants
		case "/deleteEvent":
			text = "Okay, let's delete an event. Please provide the Event Ref Key of the event you want to delete."
			userState.State = StateDeleteEvent
		default:
			text = "I didn't understand that command. Use /start or /help."
		}
	case StateAddingEventName:
		userState.CurrentEvent.Name = update.Message.Text
		text = "Great! Now, please send me the EDM for the event."
		userState.State = StateAddingEventPicture
	case StateAddingEventPicture:
		if update.Message.Photo != nil {
			// Get the largest photo
			largestPhoto := update.Message.Photo[len(update.Message.Photo)-1]
			userState.CurrentEvent.EDMFileID = largestPhoto.FileID
			text = "Got it! Now, let's add some details. Send me a question, and I'll ask for the answer. Send 'done' when you're finished."
			userState.State = StateAddingEventDetails
		} else {
			text = "Please send a picture file."
		}
	case StateAddingEventDetails:
		if strings.ToLower(update.Message.Text) == "done" {
			// Create the event in Firestore
			refKey, err := o.FirebaseConnector.CreateEvent(ctx, userState.CurrentEvent)
			if err != nil {
				log.Println("error creating event:", err)
				text = "Error creating event. Please try again."
				userState.State = StateIdle
				userState.CurrentEvent = model.Event{}
				break
			}
			userState.EventRefKey = refKey

			// Update the event with the refKey
			userState.CurrentEvent.DocumentID = refKey
			err = o.FirebaseConnector.UpdateEvent(ctx, refKey, userState.CurrentEvent)
			if err != nil {
				log.Println("error updating event with ref key:", err)
				text = "Error updating event. Please try again."
				userState.State = StateIdle
				userState.CurrentEvent = model.Event{}
				break
			}

			text = fmt.Sprintf("Event '%s' created successfully! Here are the details:\n", userState.CurrentEvent.Name)
			for _, detail := range userState.CurrentEvent.EventDetails {
				text = fmt.Sprintf("- %s: %s\n", detail.Question, detail.Answer)
			}
			text += fmt.Sprintf("EDM File ID: %s\n", userState.CurrentEvent.EDMFileID)
			text += fmt.Sprintf("Event Ref Key: %s", userState.EventRefKey)
			// Reset user state
			userState.State = StateIdle
			userState.CurrentEvent = model.Event{}
			userState.EventRefKey = ""
		} else {
			// Ask for the answer
			question := update.Message.Text
			if !strings.HasSuffix(question, "?") {
				text = "Please make sure your question ends with a question mark '?'."
				params = &bot.SendMessageParams{
					ChatID: chatID,
					Text:   text,
				}
				_, err := b.SendMessage(ctx, params)
				if err != nil {
					log.Println("error sending message:", err)
				}
				return
			}
			userState.LastQuestion = question // Store the question
			// Create a new message to ask for the answer
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   fmt.Sprintf("What's the answer to '%s'?", question),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}

			// Wait for the answer in the next update
			userState.State = StateAddingEventDetailsAnswer // Change state to wait for answer
			return
		}
	case StateAddingEventDetailsAnswer:
		// Get the previous question
		answer := update.Message.Text
		// Add the answer to the event details
		userState.CurrentEvent.EventDetails = append(userState.CurrentEvent.EventDetails, model.QnA{Question: userState.LastQuestion, Answer: answer})
		// Update the user state
		userState.State = StateAddingEventDetails
		text = "Detail added. Send another question or 'done' to finish."
	case StateDeleteEvent:
		eventID := update.Message.Text
		err := o.FirebaseConnector.DeleteEvent(ctx, eventID)
		if err != nil {
			log.Println("error deleting event:", err)
			text = fmt.Sprintf("Error deleting event with ID '%s'. Please check the ID and try again.", eventID)
		} else {
			text = fmt.Sprintf("Event with ID '%s' has been successfully deleted.", eventID)
		}
		userState.State = StateIdle
	case StateListParticipants:
		eventID := update.Message.Text
		event, err := o.FirebaseConnector.ReadEvent(ctx, eventID)
		if err != nil {
			log.Println("error reading event:", err)
			text = fmt.Sprintf("Error reading event with ID '%s'. Please check the ID and try again.", eventID)
			userState.State = StateIdle
			break
		}

		if len(event.Participants) == 0 {
			text = fmt.Sprintf("No participants found for event '%s'.", event.Name)
		} else {
			text = fmt.Sprintf("Participants for event '%s':\n", event.Name)
			for _, participant := range event.Participants {
				text += fmt.Sprintf("- Name: %s\n", participant.Name)
				text += fmt.Sprintf("  Code: %s\n", participant.Code)
				if len(participant.QnAs) > 0 {
					text += "  QnAs:\n"
					for _, qna := range participant.QnAs {
						text += fmt.Sprintf("    - Q: %s\n", qna.Question)
						text += fmt.Sprintf("      A: %s\n", qna.Answer)
					}
				}
			}
		}
		userState.State = StateIdle
	default:
		text = "An error occurred."
		userState.State = StateIdle
	}

	params = &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}

	_, err := b.SendMessage(ctx, params)
	if err != nil {
		log.Println("error sending message:", err)
	}
}
