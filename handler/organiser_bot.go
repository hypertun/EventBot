package handler

import (
	"EventBot/model"
	"EventBot/repo"
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

type OrganiserBotHandler struct {
	FirebaseConnector repo.FirestoreConnector
	ImageService      *repo.ImageService
	BotToken          string
}

func NewOrganiserBotHandler(
	FirebaseConnector repo.FirestoreConnector,
	botToken string,
) *OrganiserBotHandler {
	return &OrganiserBotHandler{
		FirebaseConnector: FirebaseConnector,
		ImageService:      repo.NewImageService(botToken),
		BotToken:          botToken,
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
			/addEvent - Create a new event with details and RSVP questions
			/deleteEvent <Event_Reference_Code> - Delete an existing event
			/listParticipants <Event_Reference_Code> - List participants of an event
			/blast <Event_Reference_Code> - Send a message to all participants
			/viewEvents - View all your events
			/setCheckInCode <Event_Reference_Code> - Set or update the check-in code for an event
			/help - Show this help message`

			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			return

		case "/help":
			text = "I'm your EventBot. I can help you manage events. Use the buttons below or type commands to manage your events."

			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			return
		case "/setCheckInCode":
			text = "Please provide the Reference Code of the event you want to set a check-in code for."
			userState.State = model.StateSettingEventCheckInCode
		case "/addEvent":
			text = "Okay, let's create a new event. What's the name of the event?"

			// Add a Cancel button
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   text,
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard: [][]models.KeyboardButton{
						{
							{Text: "Cancel"},
						},
					},
					ResizeKeyboard:  true,
					OneTimeKeyboard: true,
				},
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateAddingEventName
			return
		case "/addEventDate":
			text = "Okay, let's add the date of the event. What's the date of the event?"
			userState.State = model.StateAddingEventDate
		case "/listParticipants":
			text = "Okay, let's list the participants of an event. Please provide the Reference Code of the event."

			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   text,
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard: [][]models.KeyboardButton{
						{
							{Text: "Cancel"},
						},
					},
					ResizeKeyboard:  true,
					OneTimeKeyboard: true,
				},
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateListParticipants
			return
		case "Cancel":
			text = "Operation cancelled. What would you like to do next?"

			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			userState.LastQuestion = ""
			userState.CurrentRSVPQuestion = nil
			userState.RSVPQuestionIndex = 0
			userState.TempOptions = nil
			return
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

					// Show check-in code if set
					if event.CheckInCode != "" {
						text += fmt.Sprintf("  Check-in Code: %s\n", event.CheckInCode)
					} else {
						text += "  Check-in Code: Not set (use /setCheckInCode)\n"
					}

					if len(event.EventDetails) > 0 {
						text += "  Details:\n"
						for _, detail := range event.EventDetails {
							text += fmt.Sprintf("    - Q: %s\n", detail.Question)
							text += fmt.Sprintf("      A: %s\n", detail.Answer)
							if detail.ImageFileURL != "" && detail.ImageFileURL != "N/A" {
								text += "      (Has image)\n"
							}
						}
					}
					if len(event.RSVPQuestions) > 0 {
						text += "  RSVP Questions:\n"
						for _, q := range event.RSVPQuestions {
							text += fmt.Sprintf("    - Q: %s\n", q.Question)
							text += fmt.Sprintf("      Type: %s\n", getRSVPTypeString(q.Type))
							if len(q.Options) > 0 {
								text += "      Options: " + strings.Join(q.Options, ", ") + "\n"
							}
							if q.ImageFileURL != "" && q.ImageFileURL != "N/A" {
								text += "      (Has image)\n"
							}
						}
					}
				}
			}
			userState.State = model.StateIdle
		default:
			text = "I didn't understand that command. Use /start or /help."
		}
	case model.StateAddingEventName:
		if update.Message.Text == "Cancel" {
			text = "Event creation cancelled. What would you like to do next?"
			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		userState.CurrentEvent = &model.Event{
			Name:   update.Message.Text,
			UserID: update.Message.From.ID,
		}
		text = "Great! Now, please send me the date of the event in this format: 'YYYY-MM-DD'."

		// Add a Cancel button
		params = &bot.SendMessageParams{
			ChatID: chatID,
			Text:   text,
			ReplyMarkup: &models.ReplyKeyboardMarkup{
				Keyboard: [][]models.KeyboardButton{
					{
						{Text: "Cancel"},
					},
				},
				ResizeKeyboard:  true,
				OneTimeKeyboard: true,
			},
		}
		_, err := b.SendMessage(ctx, params)
		if err != nil {
			log.Println("error sending message:", err)
		}
		userState.State = model.StateAddingEventDate
		return
	case model.StateAddingEventDate:
		if update.Message.Text == "Cancel" {
			text = "Event creation cancelled. What would you like to do next?"
			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		// Validate the date format
		eventDate, err := time.Parse("2006-01-02", update.Message.Text)
		if err != nil || eventDate.Before(time.Now()) {
			text = "Invalid date format. Please use 'YYYY-MM-DD' (e.g., 2023-12-25) and ensure it's not in the past."
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   text,
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard: [][]models.KeyboardButton{
						{
							{Text: "Cancel"},
						},
					},
					ResizeKeyboard:  true,
					OneTimeKeyboard: true,
				},
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

			// Add a Cancel button
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   text,
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard: [][]models.KeyboardButton{
						{
							{Text: "Cancel"},
						},
					},
					ResizeKeyboard:  true,
					OneTimeKeyboard: true,
				},
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateAddingEventPicture
			return
		}
	case model.StateAddingEventPicture:
		if update.Message.Text == "Cancel" {
			text = "Event creation cancelled. What would you like to do next?"
			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		if update.Message.Photo != nil {
			// Get the largest photo
			largestPhoto := update.Message.Photo[len(update.Message.Photo)-1]
			userState.CurrentEvent.EDMFileID = largestPhoto.FileID
			text = "Got it! Now, let's add some event details. Send me a question, and I'll ask for the answer. Send 'done' when you're finished."

			// Add cancel and done options
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   text,
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard: [][]models.KeyboardButton{
						{
							{Text: "done"},
						},
						{
							{Text: "Cancel"},
						},
					},
					ResizeKeyboard:  true,
					OneTimeKeyboard: false,
				},
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateAddingEventDetails
			return
		} else {
			text = "Please send a picture file or type 'Cancel' to abort event creation."
		}
	case model.StateAddingEventDetails:
		if update.Message.Text == "Cancel" {
			text = "Event creation cancelled. What would you like to do next?"
			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		if strings.ToLower(update.Message.Text) == "done" {
			// Move to adding RSVP questions
			text = "Now, let's add RSVP questions for your participants. These will be required when participants join your event.\n\nPlease enter your first RSVP question or 'skip' if you don't want to add any RSVP questions."

			// Add skip and cancel options
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   text,
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard: [][]models.KeyboardButton{
						{
							{Text: "skip"},
						},
						{
							{Text: "Cancel"},
						},
					},
					ResizeKeyboard:  true,
					OneTimeKeyboard: true,
				},
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateAddingRSVPQuestion
			return
		} else {
			// Store the question and ask if they want to add an image
			question := update.Message.Text
			userState.LastQuestion = question // Store the question

			// Create a keyboard for image options
			keyboard := [][]models.KeyboardButton{
				{
					{Text: "Yes, add an image"},
					{Text: "No, continue without image"},
				},
				{
					{Text: "Cancel"},
				},
			}

			// Create and send a message asking about adding an image
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   fmt.Sprintf("Would you like to add an image to the question: '%s'?", question),
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard:        keyboard,
					OneTimeKeyboard: true,
					ResizeKeyboard:  true,
				},
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}

			// Change state to wait for image decision
			userState.State = model.StateAddingEventDetailsImage
			return
		}

	// Add a new state for handling image decisions for event details
	case model.StateAddingEventDetailsImage:
		if update.Message.Text == "Cancel" {
			text = "Event creation cancelled. What would you like to do next?"
			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		if update.Message.Text == "Yes, add an image" {
			// Ask user to send the image
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Please send the image for this question.",
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard: [][]models.KeyboardButton{
						{
							{Text: "Cancel"},
						},
					},
					ResizeKeyboard:  true,
					OneTimeKeyboard: true,
				},
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateAddingEventDetailsImageUpload
			return
		} else {
			// No image, proceed to ask for the answer
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   fmt.Sprintf("What's the answer to '%s'?", userState.LastQuestion),
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard: [][]models.KeyboardButton{
						{
							{Text: "Cancel"},
						},
					},
					ResizeKeyboard:  true,
					OneTimeKeyboard: true,
				},
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}

			// Wait for the answer in the next update
			userState.State = model.StateAddingEventDetailsAnswer
			return
		}

	// Add a new state for handling image uploads for event details
	case model.StateAddingEventDetailsImageUpload:
		// If the user types "Cancel", abort the event creation
		if update.Message.Text == "Cancel" {
			text = "Event creation cancelled. What would you like to do next?"
			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		if update.Message.Photo != nil {
			// Get the largest photo
			largestPhoto := update.Message.Photo[len(update.Message.Photo)-1]

			// Create a temporary QnA with image
			tempQnA := model.QnA{
				Question:    userState.LastQuestion,
				ImageFileID: largestPhoto.FileID,
			}

			// Store it in temporary storage
			userState.CurrentEvent.EventDetails = append(userState.CurrentEvent.EventDetails, tempQnA)

			// Now ask for the answer
			params = &bot.SendMessageParams{
				ChatID: chatID,
				Text:   fmt.Sprintf("Image added. What's the answer to '%s'?", userState.LastQuestion),
				ReplyMarkup: &models.ReplyKeyboardMarkup{
					Keyboard: [][]models.KeyboardButton{
						{
							{Text: "Cancel"},
						},
					},
					ResizeKeyboard:  true,
					OneTimeKeyboard: true,
				},
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}

			userState.State = model.StateAddingEventDetailsAnswer
			return
		} else {
			text = "Please send an image file or type 'skip' to continue without an image."

			// If they typed 'skip', proceed to ask for the answer
			if update.Message.Text == "skip" {
				params = &bot.SendMessageParams{
					ChatID: chatID,
					Text:   fmt.Sprintf("What's the answer to '%s'?", userState.LastQuestion),
					ReplyMarkup: &models.ReplyKeyboardMarkup{
						Keyboard: [][]models.KeyboardButton{
							{
								{Text: "Cancel"},
							},
						},
						ResizeKeyboard:  true,
						OneTimeKeyboard: true,
					},
				}
				_, err := b.SendMessage(ctx, params)
				if err != nil {
					log.Println("error sending message:", err)
				}
				userState.State = model.StateAddingEventDetailsAnswer
				return
			}
		}

	case model.StateAddingEventDetailsAnswer:
		if update.Message.Text == "Cancel" {
			text = "Event creation cancelled. What would you like to do next?"
			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		// Get the previous question
		answer := update.Message.Text

		// Find the last QnA that was added (which might have an image) or create a new one
		var lastQnA *model.QnA
		if len(userState.CurrentEvent.EventDetails) > 0 &&
			userState.CurrentEvent.EventDetails[len(userState.CurrentEvent.EventDetails)-1].Question == userState.LastQuestion {
			// Update the existing QnA that has an image
			lastQnA = &userState.CurrentEvent.EventDetails[len(userState.CurrentEvent.EventDetails)-1]
			lastQnA.Answer = answer
		} else {
			// Add the answer to the event details as a new QnA
			userState.CurrentEvent.EventDetails = append(userState.CurrentEvent.EventDetails, model.QnA{
				Question: userState.LastQuestion,
				Answer:   answer,
			})
		}

		// Update the user state
		userState.State = model.StateAddingEventDetails
		text = "Detail added. Send another question or 'done' to finish adding details and move to RSVP questions."

		// Add done and cancel options
		params = &bot.SendMessageParams{
			ChatID: chatID,
			Text:   text,
			ReplyMarkup: &models.ReplyKeyboardMarkup{
				Keyboard: [][]models.KeyboardButton{
					{
						{Text: "done"},
					},
					{
						{Text: "Cancel"},
					},
				},
				ResizeKeyboard:  true,
				OneTimeKeyboard: false,
			},
		}
		_, err := b.SendMessage(ctx, params)
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
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

		text = fmt.Sprintf("Message sent to %d participants.", len(participants))

	// RSVP Handling
	case model.StateAddingRSVPQuestion:
		if update.Message.Text == "Cancel" {
			text = "Event creation cancelled. What would you like to do next?"
			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getOrganizerMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}
		if strings.ToLower(update.Message.Text) == "skip" {
			// Save the event without RSVP questions
			refKey, err := o.saveEvent(ctx, userState.CurrentEvent)
			if err != nil {
				log.Println("error creating event:", err)
				text = "Error creating event. Please try again."
				userState.State = model.StateIdle
				userState.CurrentEvent = nil
				break
			}

			err = o.sendEventCreationConfirmation(ctx, b, chatID, userState.CurrentEvent, refKey, false)
			if err != nil {
				log.Println("error sending confirmation:", err)
			}

			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return // We've already sent messages, so return instead of setting text
		}

		if strings.ToLower(update.Message.Text) == "done" {
			// Save the event with the RSVP questions we have
			refKey, err := o.saveEvent(ctx, userState.CurrentEvent)
			if err != nil {
				log.Println("error creating event:", err)
				text = "Error creating event. Please try again."
				userState.State = model.StateIdle
				userState.CurrentEvent = nil
				break
			}

			// Send event confirmation and deep link using the helper function
			err = o.sendEventCreationConfirmation(ctx, b, chatID, userState.CurrentEvent, refKey, true)
			if err != nil {
				log.Println("error sending confirmation:", err)
			}

			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return // We've already sent messages, so return instead of setting text
		}

		// Create a new RSVP question
		userState.CurrentRSVPQuestion = &model.RSVPQuestion{
			ID:       uuid.New().String(), // Generate a unique ID
			Question: update.Message.Text,
		}

		text = "Select the type of question:\n" +
			"1. Yes/No (binary choice)\n" +
			"2. Multiple Choice (select one option)\n" +
			"3. Multiple Select (select multiple options)\n" +
			"4. Short Answer (free text)"
		userState.State = model.StateSelectingRSVPType

	case model.StateSelectingRSVPType:
		var questionType model.QuestionType
		switch update.Message.Text {
		case "1":
			questionType = model.QuestionTypeYesNo
			userState.CurrentRSVPQuestion.Type = questionType
			userState.CurrentRSVPQuestion.Options = []string{"Yes", "No"}
			text = "Would you like to add an image to this question? (yes/no)"
			userState.State = model.StateAddingRSVPImage
		case "2":
			questionType = model.QuestionTypeMCQ
			userState.CurrentRSVPQuestion.Type = questionType
			userState.TempOptions = []string{} // Initialize empty options
			text = "Enter option 1 for the multiple-choice question:"
			userState.State = model.StateAddingRSVPOptions
		case "3":
			questionType = model.QuestionTypeMultiSelect
			userState.CurrentRSVPQuestion.Type = questionType
			userState.TempOptions = []string{} // Initialize empty options
			text = "Enter option 1 for the multi-select question:"
			userState.State = model.StateAddingRSVPOptions
		case "4":
			questionType = model.QuestionTypeShortAnswer
			userState.CurrentRSVPQuestion.Type = questionType
			text = "Would you like to add an image to this question? (yes/no)"
			userState.State = model.StateAddingRSVPImage
		default:
			text = "Invalid option. Please enter a number between 1 and 4."
		}

	case model.StateAddingRSVPOptions:
		if strings.ToLower(update.Message.Text) == "done" {
			if len(userState.TempOptions) < 2 {
				text = "You need to add at least two options. Please continue adding options."
				break
			}

			// Save the options to the question
			userState.CurrentRSVPQuestion.Options = userState.TempOptions
			text = "Would you like to add an image to this question? (yes/no)"
			userState.State = model.StateAddingRSVPImage
		} else {
			// Add the option to the temporary list
			userState.TempOptions = append(userState.TempOptions, update.Message.Text)
			text = fmt.Sprintf("Option %d added. Enter option %d or type 'done' to finish adding options:",
				len(userState.TempOptions), len(userState.TempOptions)+1)
		}

	case model.StateAddingRSVPImage:
		if strings.ToLower(update.Message.Text) == "yes" {
			text = "Please send the image for this question."
			userState.State = model.StateConfirmRSVPQuestion
		} else if strings.ToLower(update.Message.Text) == "no" {
			// Add the question to the event without an image
			confirmRSVPQuestion(userState, "")
			text = "RSVP question added. Enter another question or 'done' to finish."
			userState.State = model.StateAddingRSVPQuestion
		} else {
			text = "Please respond with 'yes' or 'no'."
		}

	case model.StateConfirmRSVPQuestion:
		if update.Message.Photo != nil {
			// Get the largest photo
			largestPhoto := update.Message.Photo[len(update.Message.Photo)-1]
			// Add the question to the event with the image
			confirmRSVPQuestion(userState, largestPhoto.FileID)
			text = "RSVP question with image added. Enter another question or 'done' to finish."
			userState.State = model.StateAddingRSVPQuestion
		} else {
			text = "Please send an image file or type 'skip' to proceed without an image."
			if update.Message.Text == "skip" {
				confirmRSVPQuestion(userState, "")
				text = "RSVP question added without an image. Enter another question or 'done' to finish."
				userState.State = model.StateAddingRSVPQuestion
			}
		}
	case model.StateSettingEventCheckInCode:
		eventID := update.Message.Text
		event, err := o.FirebaseConnector.ReadEvent(ctx, eventID)
		if err != nil {
			log.Println("error reading event:", err)
			text = fmt.Sprintf("Error retrieving event with ID '%s'. Please check the ID and try again.", eventID)
			userState.State = model.StateIdle
		} else {
			userState.CurrentEvent = event

			// Show current check-in code if it exists
			currentCode := "not set"
			if event.CheckInCode != "" {
				currentCode = event.CheckInCode
			}

			text = fmt.Sprintf("Current check-in code for event '%s' is: %s\n\nPlease enter a new 4-digit check-in code for this event:", event.Name, currentCode)
			userState.State = model.StateUpdatingEventCheckInCode
		}
	case model.StateUpdatingEventCheckInCode:
		// Validate that the input is a 4-digit code
		code := update.Message.Text
		if len(code) != 4 || !isNumeric(code) {
			text = "Please enter a valid 4-digit numeric code (e.g., 1234)."
			break
		}

		// Update the event with the new check-in code
		userState.CurrentEvent.CheckInCode = code
		err := o.FirebaseConnector.UpdateEvent(ctx, userState.CurrentEvent.ID, *userState.CurrentEvent)
		if err != nil {
			log.Println("error updating event:", err)
			text = "Error updating check-in code. Please try again."
		} else {
			text = fmt.Sprintf("Check-in code for event '%s' has been set to: %s", userState.CurrentEvent.Name, code)
		}
		userState.State = model.StateIdle
		userState.CurrentEvent = nil

	default:
		text = "An error occurred."
		userState.State = model.StateIdle
	}

	if text != "" {
		params = &bot.SendMessageParams{
			ChatID: chatID,
			Text:   text,
		}

		_, err := b.SendMessage(ctx, params)
		if err != nil {
			log.Println("error sending message:", err)
		}
	}
}

// Update the saveEvent method in handler/organiser_bot.go to handle image URLs for event details

// Helper function to save the event to Firebase
func (o *OrganiserBotHandler) saveEvent(ctx context.Context, event *model.Event) (string, error) {
	// Convert all file IDs to URLs
	if event.EDMFileID != "" {
		url, err := o.ImageService.ConvertFileIDToURL(ctx, event.EDMFileID)
		if err != nil {
			log.Printf("Warning: Failed to convert EDM file ID to URL: %v", err)
		} else {
			event.EDMFileURL = url
		}
	}

	// Convert event detail question image file IDs to URLs
	for i := range event.EventDetails {
		if event.EventDetails[i].ImageFileID != "" {
			url, err := o.ImageService.ConvertFileIDToURL(ctx, event.EventDetails[i].ImageFileID)
			if err != nil {
				log.Printf("Warning: Failed to convert event detail image file ID to URL: %v", err)
			} else {
				event.EventDetails[i].ImageFileURL = url
			}
		}
	}

	// Convert RSVP question image file IDs to URLs
	for i := range event.RSVPQuestions {
		if event.RSVPQuestions[i].ImageFileID != "" {
			url, err := o.ImageService.ConvertFileIDToURL(ctx, event.RSVPQuestions[i].ImageFileID)
			if err != nil {
				log.Printf("Warning: Failed to convert RSVP question image file ID to URL: %v", err)
			} else {
				event.RSVPQuestions[i].ImageFileURL = url
			}
		}
	}

	// Create the event in Firestore
	refKey, err := o.FirebaseConnector.CreateEvent(ctx, *event)
	if err != nil {
		return "", err
	}

	// Update the event with the refKey
	event.ID = refKey
	err = o.FirebaseConnector.UpdateEvent(ctx, refKey, *event)
	if err != nil {
		return "", err
	}

	return refKey, nil
}

// Helper function to get the RSVP question type as a string
func getRSVPTypeString(qType model.QuestionType) string {
	switch qType {
	case model.QuestionTypeYesNo:
		return "Yes/No"
	case model.QuestionTypeMCQ:
		return "Multiple Choice"
	case model.QuestionTypeMultiSelect:
		return "Multiple Select"
	case model.QuestionTypeShortAnswer:
		return "Short Answer"
	default:
		return "Unknown"
	}
}

// Helper function to add the current RSVP question to the event
func confirmRSVPQuestion(userState *model.UserState, imageFileID string) {
	userState.CurrentRSVPQuestion.ImageFileID = imageFileID
	userState.CurrentEvent.RSVPQuestions = append(userState.CurrentEvent.RSVPQuestions, *userState.CurrentRSVPQuestion)
	userState.CurrentRSVPQuestion = nil
	userState.TempOptions = nil
}

func (o *OrganiserBotHandler) sendEventCreationConfirmation(ctx context.Context, b *bot.Bot, chatID int64, event *model.Event, refKey string, hasRSVP bool) error {
	// Create success message text
	var successText string
	if hasRSVP {
		successText = fmt.Sprintf("Event '%s' created successfully with %d RSVP questions!",
			event.Name, len(event.RSVPQuestions))
	} else {
		successText = fmt.Sprintf("Event '%s' created successfully without RSVP questions!",
			event.Name)
	}

	// Send success message
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      successText,
		ParseMode: "HTML",
	}
	_, err := b.SendMessage(ctx, params)
	if err != nil {
		log.Println("error sending success message:", err)
		return err
	}

	// Get participant bot username from environment variable
	participantBotUsername := os.Getenv("PARTICIPANT_BOT_NAME")
	if participantBotUsername == "" {
		participantBotUsername = "your_participant_bot" // Fallback if not set
	}

	// Create deep link for participants
	deepLink := fmt.Sprintf("https://t.me/%s?start=join_%s", participantBotUsername, refKey)

	// Follow up with the reference code and join link as a separate message to make it copyable
	codeMsg := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      fmt.Sprintf("Reference Code: <code>%s</code>\n\nParticipants can join using this link:\n%s", refKey, deepLink),
		ParseMode: "HTML", // Using HTML parsing mode to enable code formatting
	}
	_, err = b.SendMessage(ctx, codeMsg)
	if err != nil {
		log.Println("error sending code message:", err)
		return err
	}

	// Prompt for setting a check-in code
	checkInMsg := &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Would you like to set a 4-digit check-in code for this event now? Type '/setCheckInCode " + refKey + "' to set it.",
	}
	_, err = b.SendMessage(ctx, checkInMsg)
	if err != nil {
		log.Println("error sending check-in code prompt:", err)
	}

	return nil
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func getOrganizerMainMenuKeyboard() *models.ReplyKeyboardMarkup {
	return &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{
				{Text: "/addEvent"},
				{Text: "/viewEvents"},
			},
			{
				{Text: "/listParticipants"},
				{Text: "/setCheckInCode"},
			},
			{
				{Text: "/blast"},
				{Text: "/deleteEvent"},
			},
			{
				{Text: "/help"},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false, // Set to false to keep the keyboard visible
	}
}
