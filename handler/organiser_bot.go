package handler

import (
	"EventBot/model"
	"EventBot/repo"
	"context"
	"fmt"
	"log"
	"sort"
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
			text = `Hello! I'm your EventBot. Use the following commands to manage events:
			/addEvent - Create a new event
			/deleteEvent <Event_Reference_Code> - Delete an existing event
			/listParticipants <Event_Reference_Code> - List participants of an event
			/blast <Event_Reference_Code> - Send a message to all participants
			/viewEvents - View all your events
			/help - Show this help message`
		case "/help":
			text = "I'm your EventBot. I can help you manage events. Use /addEvent to start creating an event, or /deleteEvent to delete an existing event."
		case "/addEvent":
			text = "Okay, let's create a new event. What's the name of the event?"
			userState.State = model.StateAddingEventName
		case "/addEventDate":
			text = "Okay, let's add the date of the event. What's the date of the event?"
			userState.State = model.StateAddingEventDate
		case "/listParticipants":
			text = "Okay, let's list the participants of an event. Please provide the Reference Code of the event."
			userState.State = model.StateListParticipants
		case "/deleteEvent":
			text = "Okay, let's delete an event. Please provide the Reference Code of the event you want to delete."
			userState.State = model.StateDeleteEvent
		case "/blast":
			text = "Okay, let's send a message to all participants. Please provide the Reference Code of the event."
			userState.State = model.StateBlastMessage
		case "/viewEvents":
			events, err := o.FirebaseConnector.ListEventsByUserID(ctx, update.Message.From.ID)
			if err != nil {
				log.Println("error listing events:", err)
				text = "Error retrieving your events. Please try again."
			} else if len(events) == 0 {
				text = "You have not created any events yet."
			} else {
				text = "Here are your events:\n"
				for _, event := range events {
					text += fmt.Sprintf("- %s (Reference Code: %s)\n", event.Name, event.ID)
					text += fmt.Sprintf("  Date: %s\n", event.EventDate.Format("2006-01-02"))
					if len(event.EventDetails) > 0 {
						text += "  Details:\n"
						for _, detail := range event.EventDetails {
							text += fmt.Sprintf("    - Q: %s\n", detail.Question)
							text += fmt.Sprintf("      A: %s\n", detail.Answer)
						}
					}
				}
			}
			userState.State = model.StateIdle
		default:
			text = "I didn't understand that command. Use /start or /help."
		}
	case model.StateAddingEventName:
		userState.CurrentEvent = &model.Event{
			Name:   update.Message.Text,
			UserID: update.Message.From.ID,
		}
		text = "Great! Now, please send me the date of the event in this format: 'YYYY-MM-DD'."
		userState.State = model.StateAddingEventDate
	case model.StateAddingEventDate:
		// Validate the date format
		eventDate, err := time.Parse("2006-01-02", update.Message.Text)
		if err != nil || eventDate.Before(time.Now()) {
			text = "Invalid date format. Please use 'YYYY-MM-DD' (e.g., 2023-12-25) and ensure it's not in the past."
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
			refKey, err := o.FirebaseConnector.CreateEvent(ctx, *userState.CurrentEvent)
			if err != nil {
				log.Println("error creating event:", err)
				text = "Error creating event. Please try again."
				userState.State = model.StateIdle
				userState.CurrentEvent = nil
				break
			}

			// Update the event with the refKey
			userState.CurrentEvent.ID = refKey
			err = o.FirebaseConnector.UpdateEvent(ctx, refKey, *userState.CurrentEvent)
			if err != nil {
				log.Println("error updating event with Reference Code:", err)
				text = "Error updating event. Please try again."
				userState.State = model.StateIdle
				userState.CurrentEvent = nil
				break
			}

			text = fmt.Sprintf("Event '%s' created successfully! Here are the details:\n", userState.CurrentEvent.Name)
			text += fmt.Sprintf("EDM File ID: %s\n", userState.CurrentEvent.EDMFileID)
			text += fmt.Sprintf("Event Reference Code: %s", userState.CurrentEvent.ID)
			// Reset user state
			userState.State = model.StateIdle
			userState.CurrentEvent = &model.Event{}
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
			}
		}
		userState.State = model.StateIdle
	case model.StateBlastMessage:
		if userState.CurrentEvent == nil {
			event, err := o.FirebaseConnector.ReadEvent(ctx, update.Message.Text)
			if err != nil {
				log.Println("error reading event:", err)
				_, err = b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: update.Message.Chat.ID,
					Text:   "Error retrieving event. Please try again.",
				})
				if err != nil {
					log.Println("error sending message:", err)
				}
				userState.State = model.StateIdle
				return
			}
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "Please key in the message you want to send to all participants.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.CurrentEvent = event
			return
		}

		defer func() {
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
		}()

		eventID := userState.CurrentEvent.ID
		message := update.Message.Text

		participants, err := o.FirebaseConnector.ListParticipants(ctx, eventID)
		if err != nil {
			log.Println(fmt.Sprintf("error reading participants for event(ID: %s): %v", eventID, err))
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "Error retrieving participants. Please try again.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			return
		}

		sort.Slice(participants, func(i, j int) bool {
			return participants[i].Name < participants[j].Name
		})
		fmt.Println(update.Message.Chat.ID)
		for _, participant := range participants {
			var eventDetails string
			if len(userState.CurrentEvent.EventDetails) > 0 {
				eventDetails += "Event Details:\n"
				for _, detail := range userState.CurrentEvent.EventDetails {
					eventDetails += fmt.Sprintf("  - Q: %s\n", detail.Question)
					eventDetails += fmt.Sprintf("    A: %s\n", detail.Answer)
				}
			}
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: participant.UserID,
				Text: fmt.Sprintf(`Message from the organiser:
		%s		
		%s
		Event Name: %s
		Event Date: %s`, message, eventDetails, userState.CurrentEvent.Name, userState.CurrentEvent.EventDate.Format("2006-01-02")),
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
		}
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
