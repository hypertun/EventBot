package handler

import (
	"EventBot/model"
	"EventBot/repo"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

type ParticipantBotHandler struct {
	FirebaseConnector repo.FirestoreConnector
	bot               *bot.Bot
	update            *models.Update
}

func NewParticipantBotHandler(
	FirebaseConnector repo.FirestoreConnector,
) *ParticipantBotHandler {
	return &ParticipantBotHandler{
		FirebaseConnector: FirebaseConnector,
	}
}

var userPBotStates = make(map[int64]*model.UserState)

func (p *ParticipantBotHandler) Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	p.bot = b
	p.update = update

	if p.update.Message == nil {
		return
	}

	chatID := p.update.Message.Chat.ID
	userID := p.update.Message.From.ID

	// Get or create user state
	userState, ok := userPBotStates[userID]
	if !ok {
		userState = &model.UserState{State: model.StateIdle}
		userPBotStates[userID] = userState
	}

	var text string
	var params *bot.SendMessageParams

	switch userState.State {
	case model.StateIdle:
		switch {
		case strings.HasPrefix(update.Message.Text, "/start"):
			// Process deep link if present
			if len(update.Message.Text) > 7 {
				param := update.Message.Text[7:]

				if strings.HasPrefix(param, "join_") {
					eventID := strings.TrimPrefix(param, "join_")

					userState.State = model.StateJoinEvent
					p.update.Message.Text = eventID
					p.handleJoinEvent(ctx)
					return
				}
			}

			// Regular start command processing
			username := p.update.Message.From.Username
			if username == "" {
				username = p.update.Message.From.FirstName
			}
			text = fmt.Sprintf(`Hey %s! I'm your friendly event companion, here to make attending your events smooth and enjoyable—now and in the future.
	Here's how I can help:
	Quickly view events you're attending: /viewEvents
	Revisit past events: /pastEvents
	Join an event: /joinEvent
	Keep track of your own notes and reminders for each event: /notes
	Check in to an event: /checkIn

	Easily check-in at events using a simple code
	Access useful event details and FAQs
	Keep track of your own notes and reminders for each event

	Just type /help anytime to see what else I can do for you!`, username)

			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getParticipantMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			return

		case update.Message.Text == "/help":
			text = `
	Commands:
	/start – Start interacting with me and see a quick introduction.
	/viewEvents – View your upcoming events and details.
	/pastEvents – See events you've attended previously.
	/joinEvent - Join an event (you'll be prompted to answer any RSVP questions).
	/help – Get a reminder of commands and how to use me.
	/notes - Add or view personal notes for an event.
	/checkIn - Check in to an event.
	`

			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getParticipantMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			return
		case update.Message.Text == "/viewEvents":
			p.viewEventsHandler(ctx, false)
			return
		case update.Message.Text == "/pastEvents":
			p.viewEventsHandler(ctx, true)
			return
		case update.Message.Text == "/joinEvent":
			text = "Please provide the Event Reference Code of the event you want to join."

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
			userState.State = model.StateJoinEvent
			return

		case update.Message.Text == "/notes":
			text = `Please provide the Event Reference Code to view/add personal notes.`

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
			userState.State = model.StatePersonalNotes
			return

		case update.Message.Text == "/checkIn":
			text = "Please provide the Event Reference Code of the event you want to check in to."

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
			userState.State = model.StateCheckIn
			return

		// Add a case to handle the "Cancel" button in various states
		case update.Message.Text == "Cancel":
			text = "Operation cancelled. What would you like to do next?"

			params = &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: getParticipantMainMenuKeyboard(),
			}
			_, err := b.SendMessage(ctx, params)
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			userState.RSVPQuestionIndex = 0
			userState.TempOptions = nil
			return
		default:
			text = "I didn't understand that command. Use /start or /help."
		}
	case model.StateJoinEvent:
		p.handleJoinEvent(ctx)
		return
	case model.StatePersonalNotes:
		if userState.CurrentEvent == nil {
			p.handlePersonalNotes(ctx)
		} else {
			p.handlePersonalNotesReply(ctx)
		}
		return
	case model.StateCheckIn:
		p.handleCheckIn(ctx)
		return
	case model.StateSelectEventForRSVP:
		// Handle selecting which event to complete RSVP for from events list
		if update.Message.Text == "0" {
			_, err := p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "You need to complete the RSVP questions to fully join these events. Please use /viewEvents later to complete them.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			userState.TempOptions = nil
			return
		}

		// If this is a yes/no response to a single event
		if len(userState.TempOptions) == 1 && (strings.ToLower(update.Message.Text) == "yes" || strings.ToLower(update.Message.Text) == "no") {
			if strings.ToLower(update.Message.Text) == "yes" {
				// Start the RSVP questions flow immediately
				p.askRSVPQuestion(ctx, userState)
			} else {
				_, err := p.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: chatID,
					Text:   "You need to complete the RSVP questions to fully join this event. Please use /viewEvents later to complete it.",
				})
				if err != nil {
					log.Println("error sending message:", err)
				}
				userState.State = model.StateIdle
				userState.CurrentEvent = nil
				userState.TempOptions = nil
			}
			return
		}

		// Try to parse the number for multiple events
		choiceNum, err := strconv.Atoi(update.Message.Text)
		if err != nil || choiceNum < 1 || choiceNum > len(userState.TempOptions) {
			_, err := p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   fmt.Sprintf("Invalid selection. Please enter a number between 1 and %d, or '0' to skip for now.", len(userState.TempOptions)),
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			return
		}

		// Get the selected event
		selectedEventID := userState.TempOptions[choiceNum-1]
		event, err := p.FirebaseConnector.ReadEvent(ctx, selectedEventID)
		if err != nil {
			log.Println("error reading event:", err)
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Error retrieving the event. Please try again later.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.TempOptions = nil
			return
		}

		// Set up for answering RSVP questions
		userState.CurrentEvent = event
		userState.RSVPQuestionIndex = 0
		userState.TempOptions = nil

		// Start asking RSVP questions
		p.askRSVPQuestion(ctx, userState)
		return
	case model.StateAnsweringRSVPQuestion:
		p.handleRSVPQuestionAnswer(ctx)
		return
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

// Helper function to download an image, send it, and delete it
func downloadSendAndDeleteImage(ctx context.Context, b *bot.Bot, chatID int64, imageURL string, caption string) error {
	// Create temporary directory if it doesn't exist
	tempDir := "./temp_images"
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Generate a unique filename to avoid conflicts
	uniqueID := uuid.NewString()
	tempFilePath := filepath.Join(tempDir, uniqueID+".jpg")

	// Download the file
	err = downloadFile(imageURL, tempFilePath)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}

	// Ensure we delete the file when done
	defer os.Remove(tempFilePath)

	// Open the file for reading
	file, err := os.Open(tempFilePath)
	if err != nil {
		return fmt.Errorf("failed to open downloaded file: %w", err)
	}
	defer file.Close()

	// Send the photo using the file
	_, err = b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:  chatID,
		Photo:   &models.InputFileUpload{Filename: "image.jpg", Data: file},
		Caption: caption,
	})

	if err != nil {
		return fmt.Errorf("failed to send photo: %w", err)
	}

	return nil
}

// Helper function to download a file from URL
func downloadFile(url string, filepath string) error {
	// Create an HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make the HTTP request
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check if the response was successful
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the response body to file
	_, err = io.Copy(out, resp.Body)
	return err
}
func (p *ParticipantBotHandler) askRSVPQuestion(ctx context.Context, userState *model.UserState) {
	// Check if we've gone through all questions
	if userState.RSVPQuestionIndex >= len(userState.CurrentEvent.RSVPQuestions) {
		// Save the participant's answers
		p.saveRSVPAnswers(ctx, userState)
		return
	}

	// Get the current question
	question := userState.CurrentEvent.RSVPQuestions[userState.RSVPQuestionIndex]

	// Prepare the message text
	text := fmt.Sprintf("Question %d/%d: %s",
		userState.RSVPQuestionIndex+1,
		len(userState.CurrentEvent.RSVPQuestions),
		question.Question)

	// Create the appropriate reply markup based on question type
	var replyMarkup interface{}

	switch question.Type {
	case model.QuestionTypeYesNo:
		// Create a keyboard with Yes/No options
		keyboard := [][]models.KeyboardButton{
			{
				{Text: "Yes"},
				{Text: "No"},
			},
		}
		replyMarkup = &models.ReplyKeyboardMarkup{
			Keyboard:        keyboard,
			OneTimeKeyboard: true,
			ResizeKeyboard:  true,
		}

	case model.QuestionTypeMCQ:
		// Create a keyboard with MCQ options
		var rows [][]models.KeyboardButton
		for _, option := range question.Options {
			rows = append(rows, []models.KeyboardButton{{Text: option}})
		}
		replyMarkup = &models.ReplyKeyboardMarkup{
			Keyboard:        rows,
			OneTimeKeyboard: true,
			ResizeKeyboard:  true,
		}

	case model.QuestionTypeMultiSelect:
		// For multi-select, we'll handle this differently
		text += "\nSelect multiple options by sending them as a comma-separated list (e.g., 'Option 1, Option 3').\nAvailable options:\n"
		for i, option := range question.Options {
			text += fmt.Sprintf("%d. %s\n", i+1, option)
		}

	case model.QuestionTypeShortAnswer:
		// For short answer, just prompt for free text
		text += "\nPlease provide your answer as free text."
	}

	// Send the question text first
	params := &bot.SendMessageParams{
		ChatID: p.update.Message.Chat.ID,
		Text:   text,
	}

	if replyMarkup != nil {
		params.ReplyMarkup = replyMarkup
	}

	_, err := p.bot.SendMessage(ctx, params)
	if err != nil {
		log.Println("error sending message:", err)
	}

	// If there's an image URL, download and send it
	if question.ImageFileURL != "" && question.ImageFileURL != "N/A" {
		log.Printf("Attempting to download and send image: %s", question.ImageFileURL)

		err := downloadSendAndDeleteImage(
			ctx,
			p.bot,
			p.update.Message.Chat.ID,
			question.ImageFileURL,
			"Image for question "+strconv.Itoa(userState.RSVPQuestionIndex+1),
		)

		if err != nil {
			log.Printf("Failed to send image: %v", err)

			// Fall back to a reliable image if the download/send fails
			fallbackURL := "https://upload.wikimedia.org/wikipedia/commons/thumb/8/83/Telegram_2019_Logo.svg/512px-Telegram_2019_Logo.svg.png"

			err = downloadSendAndDeleteImage(
				ctx,
				p.bot,
				p.update.Message.Chat.ID,
				fallbackURL,
				"Image for question "+strconv.Itoa(userState.RSVPQuestionIndex+1)+" (fallback)",
			)

			if err != nil {
				log.Printf("Failed to send fallback image: %v", err)

				// Last resort - notify the user
				_, _ = p.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: p.update.Message.Chat.ID,
					Text:   "Note: There is an image for this question that couldn't be displayed.",
				})
			}
		}
	}

	userState.State = model.StateAnsweringRSVPQuestion
}

func (p *ParticipantBotHandler) handleRSVPQuestionAnswer(ctx context.Context) {
	userID := p.update.Message.From.ID
	userState := userPBotStates[userID]

	// Get the current question
	question := userState.CurrentEvent.RSVPQuestions[userState.RSVPQuestionIndex]

	// Process the answer based on question type
	var answers []string

	switch question.Type {
	case model.QuestionTypeYesNo, model.QuestionTypeMCQ:
		// Single selection
		answers = []string{p.update.Message.Text}

	case model.QuestionTypeMultiSelect:
		// Multiple selection
		options := strings.Split(p.update.Message.Text, ",")
		for _, option := range options {
			answers = append(answers, strings.TrimSpace(option))
		}

	case model.QuestionTypeShortAnswer:
		// Short answer
		answers = []string{p.update.Message.Text}
	}

	// Store the answer in the user state
	if userState.CurrentEvent.ID != "" && question.ID != "" {
		// Create or update the RSVP answer
		participant, err := p.FirebaseConnector.ReadParticipantByUserID(ctx, userID)
		if err != nil || participant == nil {
			log.Println("error reading participant:", err)
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "Error retrieving your details. Please try again.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			return
		}

		// Find the user's sign-up for this event
		var signedUpEvent *model.SignedUpEvent
		var eventIndex int
		for i := range participant.SignedUpEvents {
			if participant.SignedUpEvents[i].EventID == userState.CurrentEvent.ID {
				signedUpEvent = &participant.SignedUpEvents[i]
				eventIndex = i
				break
			}
		}

		if signedUpEvent != nil {
			// Check if this question has already been answered
			var answered bool
			for i, rsvpAnswer := range signedUpEvent.RSVPAnswers {
				if rsvpAnswer.QuestionID == question.ID {
					// Update the existing answer
					signedUpEvent.RSVPAnswers[i].Answers = answers
					answered = true
					break
				}
			}

			if !answered {
				// Add a new answer
				signedUpEvent.RSVPAnswers = append(signedUpEvent.RSVPAnswers, model.RSVPAnswer{
					QuestionID: question.ID,
					Answers:    answers,
				})
			}

			// Update the user's sign-up event
			participant.SignedUpEvents[eventIndex] = *signedUpEvent

			// Save back to Firebase
			err = p.FirebaseConnector.UpdateParticipant(ctx, *participant)
			if err != nil {
				log.Println("error updating participant:", err)
			}
		}
	}

	// Move to the next question
	userState.RSVPQuestionIndex++

	// Send a positive acknowledgment
	_, err := p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      p.update.Message.Chat.ID,
		Text:        "Answer recorded!",
		ReplyMarkup: &models.ReplyKeyboardRemove{RemoveKeyboard: true},
	})
	if err != nil {
		log.Println("error sending message:", err)
	}

	// Ask the next question or finish
	p.askRSVPQuestion(ctx, userState)
}

func (p *ParticipantBotHandler) saveRSVPAnswers(ctx context.Context, userState *model.UserState) {
	// This function is called when all questions have been answered
	_, err := p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      p.update.Message.Chat.ID,
		Text:        "Thank you for completing the RSVP questions! Your event registration is now complete.",
		ReplyMarkup: &models.ReplyKeyboardRemove{RemoveKeyboard: true},
	})
	if err != nil {
		log.Println("error sending message:", err)
	}

	// Reset the state
	userState.State = model.StateIdle
	userState.CurrentEvent = nil
	userState.RSVPQuestionIndex = 0
}

// Update the handle check-in method to use the event's check-in code
func (p *ParticipantBotHandler) handleCheckIn(ctx context.Context) {
	userID := p.update.Message.From.ID
	userState := userPBotStates[userID]

	// First stage: Get the event reference code
	if userState.CurrentEvent == nil {
		eventID := p.update.Message.Text
		event, err := p.FirebaseConnector.ReadEvent(ctx, eventID)
		if err != nil {
			log.Println("error reading event:", err)
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "Error checking you in. Please check the event reference code and try again.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			return
		}

		// Check if event has a check-in code
		if event.CheckInCode == "" {
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "This event doesn't have a check-in code set by the organizer yet. Please try again later.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			return
		}

		// Check if it's the event day
		if event.EventDate.Format(time.DateOnly) != time.Now().Format(time.DateOnly) {
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "You can only check in to this event on the date of the event itself.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			return
		}

		// Verify the participant is registered for this event
		participant, err := p.FirebaseConnector.ReadParticipantByUserID(ctx, userID)
		if err != nil || participant == nil {
			log.Println("error reading participant:", err)
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "You are not registered for this event. Please join the event first.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			return
		}

		// Check if participant is registered for this specific event
		var isRegistered bool
		for _, signedUpEvent := range participant.SignedUpEvents {
			if signedUpEvent.EventID == eventID {
				isRegistered = true

				// Check if already checked in
				if signedUpEvent.CheckedIn {
					_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
						ChatID: p.update.Message.Chat.ID,
						Text:   "You have already checked in to this event.",
					})
					if err != nil {
						log.Println("error sending message:", err)
					}
					userState.State = model.StateIdle
					return
				}
				break
			}
		}

		if !isRegistered {
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "You are not registered for this event. Please join the event first.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			return
		}

		// Store event in user state and prompt for check-in code
		userState.CurrentEvent = event
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "Please enter the 4-digit check-in code provided by the event organizer:",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		userState.State = model.StateEnteringCheckInCode
		return
	}

	// Second stage: Verify the check-in code
	if userState.State == model.StateEnteringCheckInCode {
		enteredCode := p.update.Message.Text
		event := userState.CurrentEvent

		// Verify the check-in code
		if enteredCode != event.CheckInCode {
			_, err := p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "Incorrect check-in code. Please try again or contact the event organizer.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		// Code is correct, mark participant as checked in
		participant, err := p.FirebaseConnector.ReadParticipantByUserID(ctx, userID)
		if err != nil || participant == nil {
			log.Println("error reading participant:", err)
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "Error retrieving your details. Please try again.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		// Find and update the correct signed up event
		var updated bool
		for i, signedUpEvent := range participant.SignedUpEvents {
			if signedUpEvent.EventID == event.ID {
				participant.SignedUpEvents[i].CheckedIn = true
				updated = true
				break
			}
		}

		if !updated {
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "You are not registered for this event. Please join the event first.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		// Save the updated participant
		err = p.FirebaseConnector.UpdateParticipant(ctx, *participant)
		if err != nil {
			log.Println("error updating participant:", err)
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "Error checking you in. Please try again.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}

		// Success message
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "You have successfully checked in! Enjoy the event.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}

		userState.State = model.StateIdle
		userState.CurrentEvent = nil
	}
}

func (p *ParticipantBotHandler) handlePersonalNotes(ctx context.Context) {
	eventID := p.update.Message.Text
	participant, err := p.FirebaseConnector.ReadParticipantByUserID(ctx, p.update.Message.From.ID)
	if err != nil {
		log.Println("error reading participant:", err)
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "Error retrieving your details. Please try again.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	if participant == nil {
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "You are not registered for any events. Please join an event first.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	var signedUpEvent *model.SignedUpEvent
	for i := range participant.SignedUpEvents {
		if participant.SignedUpEvents[i].EventID == eventID {
			signedUpEvent = &participant.SignedUpEvents[i]
			break
		}
	}

	if signedUpEvent == nil {
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "You are not registered for this events. Please join the event first.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		userPBotStates[p.update.Message.From.ID].State = model.StateIdle
		return
	}

	_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    p.update.Message.Chat.ID,
		Text:      fmt.Sprintf("Your current notes for this event:\n<code>%s</code>\n\nDo you want to modify these notes? (yes/no)", signedUpEvent.PersonalNotes),
		ParseMode: "HTML",
	})
	if err != nil {
		log.Println("error sending message:", err)
	}
	userPBotStates[p.update.Message.From.ID].CurrentEvent = &model.Event{
		ID: eventID,
	}
	userPBotStates[p.update.Message.From.ID].State = model.StatePersonalNotes
}

func (p *ParticipantBotHandler) handlePersonalNotesReply(ctx context.Context) {
	userID := p.update.Message.From.ID
	userState := userPBotStates[userID]

	eventID := userPBotStates[userID].CurrentEvent.ID
	participant, err := p.FirebaseConnector.ReadParticipantByUserID(ctx, p.update.Message.From.ID)
	if err != nil {
		log.Println("error reading participant:", err)
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "Error retrieving your details. Please try again.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	if participant == nil {
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "You are not registered for any events. Please join an event first.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	if userState.State == model.StatePersonalNotes {
		if p.update.Message.Text == "yes" {
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "Please enter your new notes.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StatePersonalNotes
			return
		} else if p.update.Message.Text == "no" {
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "Okay, your notes remain unchanged.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userState.State = model.StateIdle
			userState.CurrentEvent = nil
			return
		}
	}

	newNotes := p.update.Message.Text
	for i, signedUpEvent := range participant.SignedUpEvents {
		if signedUpEvent.EventID == eventID {
			participant.SignedUpEvents[i].PersonalNotes = newNotes
			break
		}
	}

	err = p.FirebaseConnector.UpdateParticipant(ctx, *participant)
	if err != nil {
		log.Println("error updating participant:", err)
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "Error updating your notes. Please try again.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: p.update.Message.Chat.ID,
		Text:   "Your notes have been updated.",
	})
	if err != nil {
		log.Println("error sending message:", err)
	}
	userState.CurrentEvent = nil
	userState.State = model.StateIdle
}

func (p *ParticipantBotHandler) handleJoinEvent(ctx context.Context) {
	userID := p.update.Message.From.ID
	eventID := p.update.Message.Text

	// Get event details first to check if RSVP questions exist
	event, err := p.FirebaseConnector.ReadEvent(ctx, eventID)
	if err != nil {
		log.Println("error reading event:", err)
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   fmt.Sprintf("Error finding event with ID '%s'. Please check the ID and try again.", eventID),
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	// Create participant without a check-in code
	participant := &model.Participant{
		UserID: userID,
		Name:   p.update.Message.From.FirstName,
		// Code field removed
	}

	err = p.FirebaseConnector.CreateParticipant(ctx, eventID, participant)
	if err != nil {
		log.Println("error creating participant:", err)
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   fmt.Sprintf("Error joining event '%s'. Please try again.", eventID),
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	// Send event image if available
	if event.EDMFileURL != "" && event.EDMFileURL != "N/A" {
		log.Printf("Attempting to download and send EDM image: %s", event.EDMFileURL)

		err := downloadSendAndDeleteImage(
			ctx,
			p.bot,
			userID,
			event.EDMFileURL,
			fmt.Sprintf("Event: %s", event.Name),
		)

		if err != nil {
			log.Printf("Failed to send EDM image: %v", err)

			// Send a message without image if failed
			_, _ = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: userID,
				Text:   fmt.Sprintf("Event: %s (image unavailable)", event.Name),
			})
		}
	}

	// Send event details
	err = p.sendEventDetailsWithImages(ctx, event)
	if err != nil {
		log.Printf("Failed to send event details: %v", err)
	}

	// Send the confirmation message (without check-in code)
	joinMessage := fmt.Sprintf(`You have successfully joined event '%s'!

To check in on the day of the event, use the /checkIn command and the organizer will provide you with a 4-digit check-in code.`, event.Name)

	_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: userID,
		Text:   joinMessage,
	})
	if err != nil {
		log.Println("error sending message:", err)
	}

	// If the event has RSVP questions, start the RSVP flow immediately
	if event != nil && len(event.RSVPQuestions) > 0 {
		time.Sleep(1 * time.Second) // Small delay for better UX

		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   "This event requires you to answer some RSVP questions. Let's go through them now.",
		})
		if err != nil {
			log.Println("error sending RSVP prompt:", err)
		}

		// Set up state for answering RSVP
		userState := userPBotStates[userID]
		userState.CurrentEvent = event
		userState.RSVPQuestionIndex = 0
		userState.State = model.StateAnsweringRSVPQuestion

		// Start asking RSVP questions immediately
		p.askRSVPQuestion(ctx, userState)
	} else {
		// No RSVP questions, joining is complete
		userState := userPBotStates[userID]
		userState.State = model.StateIdle
	}

	_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      userID,
		Text:        "What would you like to do next?",
		ReplyMarkup: getParticipantMainMenuKeyboard(),
	})
	if err != nil {
		log.Println("error sending message:", err)
	}
}

func (p *ParticipantBotHandler) viewEventsHandler(ctx context.Context, past bool) {
	allEvents, err := p.FirebaseConnector.ListEventsByParticipantUserID(ctx, p.update.Message.From.ID)
	if err != nil && errors.Is(err, model.ErrParticipantDoesNotExist) {
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "You are not signed up for any events.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	} else if err != nil {
		log.Println("error listing all events:", err)
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "Error retrieving events. Please try again later.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	var eventsToShow []model.Event
	var incompleteEvents []model.Event
	participant, _ := p.FirebaseConnector.ReadParticipantByUserID(ctx, p.update.Message.From.ID)

	for i := range allEvents {
		event, err := p.FirebaseConnector.ReadEvent(ctx, allEvents[i].ID)
		if err != nil {
			log.Println(fmt.Sprintf("error reading event with event(ID: %s): %v", event.ID, err))
			continue
		}
		if past {
			if event.EventDate.After(time.Now()) {
				continue
			}
		} else {
			if event.EventDate.Before(time.Now()) {
				continue
			}
		}

		// Check if this event has unanswered RSVP questions
		if len(event.RSVPQuestions) > 0 && participant != nil && !past {
			hasAnsweredAll := false

			// Find the user's sign-up for this event
			for _, signedUpEvent := range participant.SignedUpEvents {
				if signedUpEvent.EventID == event.ID {
					if len(signedUpEvent.RSVPAnswers) == len(event.RSVPQuestions) {
						hasAnsweredAll = true
					}
					break
				}
			}

			if !hasAnsweredAll {
				incompleteEvents = append(incompleteEvents, *event)
				continue // Don't add to regular events list
			}
		}

		eventsToShow = append(eventsToShow, *event)
	}

	// If we only have incomplete events with RSVP questions, handle that case
	if len(eventsToShow) == 0 && len(incompleteEvents) == 0 {
		text := "You have no upcoming events."
		if past {
			text = "You have no past events."
		}
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   text,
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	// Sort events by date
	sort.Slice(eventsToShow, func(i, j int) bool {
		return eventsToShow[i].EventDate.After(eventsToShow[j].EventDate)
	})

	sort.Slice(incompleteEvents, func(i, j int) bool {
		return incompleteEvents[i].EventDate.After(incompleteEvents[j].EventDate)
	})

	var messageText string
	var pendingRSVPEventsIDs []string

	// First, show incomplete events that need RSVP
	if len(incompleteEvents) > 0 {
		messageText = "⚠️ Events requiring RSVP completion:\n"
		for _, event := range incompleteEvents {
			messageText += fmt.Sprintf("- %s (Event ID: %s)\n",
				event.Name, event.ID)
			messageText += fmt.Sprintf("  Date: %s\n", event.EventDate.Format("2006-01-02"))
			messageText += "  ⚠️ You must complete the RSVP to fully join this event.\n"
			pendingRSVPEventsIDs = append(pendingRSVPEventsIDs, event.ID)
		}

		messageText += "\n"
	}

	// Then show complete/normal events
	if len(eventsToShow) > 0 {
		if messageText == "" {
			messageText = "Here are the events you are signed up for:\n"
		} else {
			messageText += "Completed Events:\n"
		}

		for _, event := range eventsToShow {
			messageText += fmt.Sprintf("- %s (Event ID: %s)\n",
				event.Name, event.ID)
			messageText += fmt.Sprintf("  Date: %s\n", event.EventDate.Format("2006-01-02"))
			if len(event.EventDetails) > 0 {
				messageText += "  Details:\n"
				for _, detail := range event.EventDetails {
					messageText += fmt.Sprintf("    - Q: %s\n", detail.Question)
					messageText += fmt.Sprintf("      A: %s\n", detail.Answer)
				}
			}
		}
	}

	_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: p.update.Message.Chat.ID,
		Text:   messageText,
	})
	if err != nil {
		log.Println("error sending message:", err)
	}

	// If there are pending RSVP events, ask the user to complete them
	if len(pendingRSVPEventsIDs) > 0 && !past {
		time.Sleep(1 * time.Second) // Small delay

		if len(pendingRSVPEventsIDs) == 1 {
			// Only one event has pending RSVP
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   fmt.Sprintf("You have incomplete RSVP questions for event ID: %s. Would you like to complete them now? (yes/no)", pendingRSVPEventsIDs[0]),
			})

			userState := userPBotStates[p.update.Message.From.ID]
			event, _ := p.FirebaseConnector.ReadEvent(ctx, pendingRSVPEventsIDs[0])
			userState.CurrentEvent = event
			userState.RSVPQuestionIndex = 0
			userState.State = model.StateSelectEventForRSVP
			userState.TempOptions = pendingRSVPEventsIDs
		} else {
			// Multiple events have pending RSVP
			pendingText := "You have incomplete RSVP questions for these events:\n"
			for i, eventID := range pendingRSVPEventsIDs {
				event, _ := p.FirebaseConnector.ReadEvent(ctx, eventID)
				if event != nil {
					pendingText += fmt.Sprintf("%d. %s (Event ID: %s)\n", i+1, event.Name, eventID)
				} else {
					pendingText += fmt.Sprintf("%d. Event ID: %s\n", i+1, eventID)
				}
			}
			pendingText += "\nPlease enter the number of the event you'd like to complete RSVP questions for, or '0' to skip."

			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   pendingText,
			})

			userState := userPBotStates[p.update.Message.From.ID]
			userState.TempOptions = pendingRSVPEventsIDs
			userState.State = model.StateSelectEventForRSVP
		}

		if err != nil {
			log.Println("error sending RSVP prompt:", err)
		}
	}
}

func (p *ParticipantBotHandler) sendEventDetailsWithImages(ctx context.Context, event *model.Event) error {
	// First send the event name and date
	_, err := p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: p.update.Message.Chat.ID,
		Text:   fmt.Sprintf("Event: %s\nDate: %s", event.Name, event.EventDate.Format("2006-01-02")),
	})
	if err != nil {
		return err
	}

	// If the event has an EDM image, send it
	if event.EDMFileURL != "" && event.EDMFileURL != "N/A" {
		err := downloadSendAndDeleteImage(
			ctx,
			p.bot,
			p.update.Message.Chat.ID,
			event.EDMFileURL,
			"Event banner",
		)
		if err != nil {
			log.Printf("Failed to send EDM image: %v", err)
		}
	}

	// Send each event detail, with images when available
	if len(event.EventDetails) > 0 {
		_, err := p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "Event Details:",
		})
		if err != nil {
			return err
		}

		for _, detail := range event.EventDetails {
			detailText := fmt.Sprintf("Q: %s\nA: %s", detail.Question, detail.Answer)

			// If the detail has an image, send it with the text as caption
			if detail.ImageFileURL != "" && detail.ImageFileURL != "N/A" {
				err := downloadSendAndDeleteImage(
					ctx,
					p.bot,
					p.update.Message.Chat.ID,
					detail.ImageFileURL,
					detailText,
				)
				if err != nil {
					log.Printf("Failed to send detail image: %v", err)
					// Fall back to sending text-only if image fails
					_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
						ChatID: p.update.Message.Chat.ID,
						Text:   detailText + "\n(Image unavailable)",
					})
					if err != nil {
						log.Printf("Failed to send detail text: %v", err)
					}
				}
			} else {
				// No image, just send the text
				_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: p.update.Message.Chat.ID,
					Text:   detailText,
				})
				if err != nil {
					log.Printf("Failed to send detail text: %v", err)
				}
			}
		}
	}

	return nil
}

// Create a helper function to get the main menu keyboard for participants
func getParticipantMainMenuKeyboard() *models.ReplyKeyboardMarkup {
	return &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{
				{Text: "/viewEvents"},
				{Text: "/joinEvent"},
			},
			{
				{Text: "/checkIn"},
				{Text: "/notes"},
			},
			{
				{Text: "/pastEvents"},
				{Text: "/help"},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false, // Set to false to keep the keyboard visible
	}
}
