package handler

import (
	"EventBot/model"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Event creation states
const (
	StateIdle = iota
	StateAddingEventName
	StateAddingEventPicture
	StateAddingEventDetails

	//For participant bot
	StatePersonalNotes
	StateCheckIn
)

// User state data
type UserState struct {
	State        int
	CurrentEvent model.Event
}

// Global map to store user states
var userStates = make(map[int64]*UserState)

func OrganiserHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
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
			text = "Hello! I'm your EventBot. Use /addEvent to create a new event."
		case "/help":
			text = "I'm your EventBot. I can help you manage events. Use /addEvent to start creating an event."
		case "/addEvent":
			text = "Okay, let's create a new event. What's the name of the event?"
			userState.State = StateAddingEventName
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
			text = fmt.Sprintf("Event '%s' created successfully! Here are the details:\n", userState.CurrentEvent.Name)
			for _, detail := range userState.CurrentEvent.EventDetails {
				text += fmt.Sprintf("- %s: %s\n", detail.Question, detail.Answer)
			}
			text += fmt.Sprintf("EDM File ID: %s", userState.CurrentEvent.EDMFileID)
			// Reset user state
			userState.State = StateIdle
			userState.CurrentEvent = model.Event{}
		} else {
			// Ask for the answer
			question := update.Message.Text
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
			// Add a temporary state to handle the answer
			tempState := &UserState{State: StateAddingEventDetailsAnswer, CurrentEvent: userState.CurrentEvent}
			userStates[userID] = tempState
			return
		}
	case StateAddingEventDetailsAnswer:
		// Get the previous question
		// previousQuestion := userStates[userID].CurrentEvent.EventDetails[len(userStates[userID].CurrentEvent.EventDetails)-1].Question
		answer := update.Message.Text
		// Add the answer to the event details
		userStates[userID].CurrentEvent.EventDetails[len(userStates[userID].CurrentEvent.EventDetails)-1].Answer = answer
		// Update the user state
		userStates[userID] = &UserState{State: StateAddingEventDetails, CurrentEvent: userStates[userID].CurrentEvent}
		text = "Detail added. Send another question or 'done' to finish."
	default:
		text = "An error occurred."
		userState.State = StateIdle
	}

	if params == nil {
		params = &bot.SendMessageParams{
			ChatID: chatID,
			Text:   text,
		}
	}

	_, err := b.SendMessage(ctx, params)
	if err != nil {
		log.Println("error sending message:", err)
	}
	if userState.State == StateAddingEventDetails && strings.ToLower(update.Message.Text) != "done" && userStates[userID].State != StateAddingEventDetailsAnswer {
		userStates[userID].CurrentEvent.EventDetails = append(userStates[userID].CurrentEvent.EventDetails, model.EventDetails{Question: update.Message.Text})
	}
}

const (
	StateAddingEventDetailsAnswer = iota + 10
)
