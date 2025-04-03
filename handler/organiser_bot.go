package handler

import (
	"EventBot/model"
	"EventBot/repo"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

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

// Global map to store user states
var userOBotStates = make(map[int64]*model.UserState)

func (o *OrganiserBotHandler) Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID

	// Get or create user state
	userState, ok := userOBotStates[userID]
	if !ok {
		userState = &model.UserState{State: model.StateIdle}
		userOBotStates[userID] = userState
	}

	var text string
	var params *bot.SendMessageParams

	switch userState.State {
	case model.StateIdle:
		switch update.Message.Text {
		case "/start":
			text = "Hello! I'm your EventBot. Use /addEvent to create a new event, or /deleteEvent to delete an existing event."
		case "/help":
			text = "I'm your EventBot. I can help you manage events. Use /addEvent to start creating an event, or /deleteEvent to delete an existing event."
		case "/addEvent":
			text = "Okay, let's create a new event. What's the name of the event?"
			userState.State = model.StateAddingEventName
		case "/addEventDate":
			text = "Okay, let's add the date of the event. What's the date of the event?"
			userState.State = model.StateAddingEventDate
		case "/listParticipants":
			text = "Okay, let's list the participants of an event. Please provide the Event Ref Key of the event."
			userState.State = model.StateListParticipants
		case "/deleteEvent":
			text = "Okay, let's delete an event. Please provide the Event Ref Key of the event you want to delete."
			userState.State = model.StateDeleteEvent
		default:
			text = "I didn't understand that command. Use /start or /help."
		}
	case model.StateAddingEventName:
		userState.CurrentEvent.Name = update.Message.Text
		text = "Great! Now, please send me the date of the event in this format: 'YYYY-MM-DD'."
		userState.State = model.StateAddingEventDate
	case model.StateAddingEventDate:
		// Validate the date format
		eventDate, err := time.Parse("2006-01-02", update.Message.Text)
		if err != nil {
			text = "Invalid date format. Please use 'YYYY-MM-DD' (e.g., 2023-12-25)."
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   text,
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			return
		} else {
			// Date format is correct, store it and move to the next state
			userState.CurrentEvent.EventDate = eventDate
			text = "Great! Now, please send me the EDM for the event."
			userState.State = model.StateAddingEventPicture
		}
	case model.StateAddingEventPicture:
		if update.Message.Photo != nil {
			// Get the largest photo
			largestPhoto := update.Message.Photo[len(update.Message.Photo)-1]
			userState.CurrentEvent.EDMFileID = largestPhoto.FileID
			text = "Got it! Now, let's add some details. Send me a question, and I'll ask for the answer. Send 'done' when you're finished."
			userState.State = model.StateAddingEventDetails
		} else {
			text = "Please send a picture file."
		}
	case model.StateAddingEventDetails:
		if strings.ToLower(update.Message.Text) == "done" {
			// Create the event in Firestore
			refKey, err := o.FirebaseConnector.CreateEvent(ctx, userState.CurrentEvent)
			if err != nil {
				log.Println("error creating event:", err)
				text = "Error creating event. Please try again."
				userState.State = model.StateIdle
				userState.CurrentEvent = model.Event{}
				break
			}
			userState.EventRefKey = refKey

			// Update the event with the refKey
			userState.CurrentEvent.ID = refKey
			err = o.FirebaseConnector.UpdateEvent(ctx, refKey, userState.CurrentEvent)
			if err != nil {
				log.Println("error updating event with ref key:", err)
				text = "Error updating event. Please try again."
				userState.State = model.StateIdle
				userState.CurrentEvent = model.Event{}
				break
			}

			text = fmt.Sprintf("Event '%s' created successfully! Here are the details:\n", userState.CurrentEvent.Name)
			text += fmt.Sprintf("EDM File ID: %s\n", userState.CurrentEvent.EDMFileID)
			text += fmt.Sprintf("Event Ref Key: %s", userState.EventRefKey)
			// Reset user state
			userState.State = model.StateIdle
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
			userState.State = model.StateAddingEventDetailsAnswer // Change state to wait for answer
			return
		}
	case model.StateAddingEventDetailsAnswer:
		// Get the previous question
		answer := update.Message.Text
		// Add the answer to the event details
		userState.CurrentEvent.EventDetails = append(userState.CurrentEvent.EventDetails, model.QnA{Question: userState.LastQuestion, Answer: answer})
		// Update the user state
		userState.State = model.StateAddingEventDetails
		text = "Detail added. Send another question or 'done' to finish."
	case model.StateDeleteEvent:
		eventID := update.Message.Text
		err := o.FirebaseConnector.DeleteEvent(ctx, eventID)
		if err != nil {
			log.Println("error deleting event:", err)
			text = fmt.Sprintf("Error deleting event with ID '%s'. Please check the ID and try again.", eventID)
		} else {
			text = fmt.Sprintf("Event with ID '%s' has been successfully deleted.", eventID)
		}
		userState.State = model.StateIdle
	case model.StateListParticipants:
		eventID := update.Message.Text
		event, err := o.FirebaseConnector.ReadEvent(ctx, eventID)
		if err != nil {
			log.Println("error reading event:", err)
			text = fmt.Sprintf("Error reading event with ID '%s'. Please check the ID and try again.", eventID)
			userState.State = model.StateIdle
			break
		}

		if len(event.Participants) == 0 {
			text = fmt.Sprintf("No participants found for event '%s'.", event.Name)
		} else {
			text = fmt.Sprintf("Participants for event '%s':\n", event.Name)

			participants, err := o.FirebaseConnector.ListParticipants(ctx, eventID)
			if err != nil {
				log.Println(fmt.Sprintf("error reading participants for event(ID: %s): %v", eventID, err))
				text = fmt.Sprintf("Error reading participants for event with ID '%s'. Please check the ID and try again.",
					eventID)
			}

			for i := range participants {
				text += fmt.Sprintf("- Name: %s\n", participants[i].Name)
				text += fmt.Sprintf("  Code: %s\n", participants[i].Code)
				if len(participants[i].QnAs) > 0 {
					text += "  QnAs:\n"
					for _, qna := range participants[i].QnAs {
						text += fmt.Sprintf("    - Q: %s\n", qna.Question)
						text += fmt.Sprintf("      A: %s\n", qna.Answer)
					}
				}
			}
		}
		userState.State = model.StateIdle
	default:
		text = "An error occurred."
		userState.State = model.StateIdle
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
