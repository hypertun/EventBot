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
			/help - Show this help message`
		case "/help":
			text = "I'm your EventBot. I can help you manage events. Use /addEvent to start creating an event with details and RSVP questions, or /deleteEvent to delete an existing event."
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
					if len(event.RSVPQuestions) > 0 {
						text += "  RSVP Questions:\n"
						for _, q := range event.RSVPQuestions {
							text += fmt.Sprintf("    - Q: %s\n", q.Question)
							text += fmt.Sprintf("      Type: %s\n", getRSVPTypeString(q.Type))
							if len(q.Options) > 0 {
								text += "      Options: " + strings.Join(q.Options, ", ") + "\n"
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
			text = "Got it! Now, let's add some event details. Send me a question, and I'll ask for the answer. Send 'done' when you're finished."
			userState.State = model.StateAddingEventDetails
		} else {
			text = "Please send a picture file."
		}
	case model.StateAddingEventDetails:
		if strings.ToLower(update.Message.Text) == "done" {
			// Move to adding RSVP questions
			text = "Now, let's add RSVP questions for your participants. These will be required when participants join your event.\n\nPlease enter your first RSVP question or 'skip' if you don't want to add any RSVP questions."
			userState.State = model.StateAddingRSVPQuestion
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
		text = "Detail added. Send another question or 'done' to finish adding details and move to RSVP questions."
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

			text = fmt.Sprintf("Event '%s' created successfully without RSVP questions! Here's the Reference Code: %s",
				userState.CurrentEvent.Name, refKey)
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			break
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

			text = fmt.Sprintf("Event '%s' created successfully with %d RSVP questions! Here's the Reference Code: %s",
				userState.CurrentEvent.Name, len(userState.CurrentEvent.RSVPQuestions), refKey)
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			break
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
