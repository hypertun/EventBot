package handler

import (
	"EventBot/model"
	"EventBot/repo"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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
		switch update.Message.Text {
		case "/start":
			username := p.update.Message.From.Username
			if username == "" {
				username = p.update.Message.From.FirstName
			}
			text = fmt.Sprintf(`Hey %s! I'm your friendly event companion, here to make attending your events smooth and enjoyable—now and in the future.
			Here's how I can help:
			Quickly view events you're attending: /viewEvents
			Revisit past events: /pastEvents
			Join an event: /joinEvent
			Easily check-in at events using a simple code
			Access useful event details and FAQs
			Keep track of your own notes and reminders for each event

			Just type /help anytime to see what else I can do for you!`, username)
		case "/help":
			text = `
			Commands:
			/start – Start interacting with me and see a quick introduction.
			/viewEvents – View your upcoming events and details.
			/pastEvents – See events you've attended previously.
			/joinEvent - Join an event.
			/help – Get a reminder of commands and how to use me.
			`
		case "/viewEvents":
			p.viewEventsHandler(ctx, false)
			userState.State = model.StateIdle
			return
		case "/pastEvents":
			p.viewEventsHandler(ctx, true)
			userState.State = model.StateIdle
			return
		case "/joinEvent":
			text = "Please provide the Event Ref Key of the event you want to join."
			userState.State = model.StateJoinEvent
		default:
			text = "I didn't understand that command. Use /start or /help."
		}
	case model.StateJoinEvent:
		p.handleJoinEvent(ctx)
		userState.State = model.StateIdle
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

func (p *ParticipantBotHandler) handleJoinEvent(ctx context.Context) {
	userID := p.update.Message.From.ID
	eventID := p.update.Message.Text

	participant := &model.Participant{
		UserID: userID,
		Name:   p.update.Message.From.FirstName,
		Code:   strconv.Itoa(rand.Intn(900000) + 100000),
	}

	err := p.FirebaseConnector.CreateParticipant(ctx, eventID, participant)
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

	_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: userID,
		Text:   fmt.Sprintf("You have successfully joined event '%s'!", eventID),
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
		}
		eventsToShow = append(eventsToShow, *event)
	}

	if len(eventsToShow) == 0 {
		text := "You have no upcoming for any events."
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

	sort.Slice(eventsToShow, func(i, j int) bool {
		return eventsToShow[i].EventDate.After(eventsToShow[j].EventDate)
	})

	var messageText string
	messageText = "Here are the events you are signed up for:\n"
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

	_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: p.update.Message.Chat.ID,
		Text:   messageText,
	})
	if err != nil {
		log.Println("error sending message:", err)
	}
}

// // checkInCallbackHandler is triggered when a user taps the "Check In" button
// // in the event details inline keyboard.
// func checkInCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
// 	// We expect callback data in the form "checkIn_{index}".
// 	var index int
// 	_, err := fmt.Sscanf(string(data), "checkIn_%d", &index)
// 	if err != nil {
// 		log.Println("error parsing check-in callback index:", err)
// 		return
// 	}
// 	// In showEventDetails we already set the current event in chat state.
// 	checkInHandler(ctx, b, mes.Message.Chat.ID)
// }

// // checkInHandler verifies that a current event is selected (via showEventDetails)
// // and then prompts the user to enter the 6‑digit check‑in code.
// func checkInHandler(ctx context.Context, b *bot.Bot, chatID int64) {
// 	cs, ok := chatStates[chatID]
// 	if !ok || cs.CurrentEvent.Name == "" {
// 		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 			ChatID: chatID,
// 			Text:   "No event selected. Please use /viewEvents to select an event first.",
// 		})
// 		if err != nil {
// 			log.Println("error sending no event message:", err)
// 		}
// 		return
// 	}
// 	// Set state to check-in.
// 	cs.State = StateCheckIn
// 	prompt := fmt.Sprintf("Please enter a 6 digit code for %s", cs.CurrentEvent.Name)
// 	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 		ChatID: chatID,
// 		Text:   prompt,
// 	})
// 	if err != nil {
// 		log.Println("error sending check-in prompt:", err)
// 	}
// }

// // handleCheckInReply processes the user's reply containing the check‑in code.
// func handleCheckInReply(ctx context.Context, b *bot.Bot, update *models.Update) {
// 	chatID := update.Message.Chat.ID
// 	cs, ok := chatStates[chatID]
// 	if !ok || cs.State != StateCheckIn {
// 		// Not in check-in mode; ignore.
// 		return
// 	}
// 	code := update.Message.Text
// 	if code != "123456" {
// 		// Create an inline keyboard with two buttons.
// 		kb := inline.New(b)
// 		kb.Row().Button("Try Again", []byte("try_again_checkin"), tryAgainCheckInCallbackHandler)
// 		kb.Row().Button("Cancel", []byte("cancel_checkin"), cancelCheckInCallbackHandler)
// 		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 			ChatID:      chatID,
// 			Text:        fmt.Sprintf("Incorrect code. Check in failed. Please try again by entering the correct 6 digit code for %s", cs.CurrentEvent.Name),
// 			ReplyMarkup: kb,
// 		})
// 		if err != nil {
// 			log.Println("error sending check-in failure message:", err)
// 		}
// 		// Do not reset state so the user remains in check-in mode.
// 		return
// 	}
// 	// Successful check-in.
// 	username := update.Message.From.Username
// 	if username == "" {
// 		username = update.Message.From.FirstName
// 	}
// 	successMsg := fmt.Sprintf("Thank you %s, check in to %s is successful. Enjoy your time here!", username, cs.CurrentEvent.Name)
// 	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 		ChatID: chatID,
// 		Text:   successMsg,
// 	})
// 	if err != nil {
// 		log.Println("error sending check-in success message:", err)
// 	}
// 	cs.State = StateIdle
// }

// // tryAgainCheckInCallbackHandler re-prompts the user for the check-in code.
// func tryAgainCheckInCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
// 	chatID := mes.Message.Chat.ID
// 	cs, ok := chatStates[chatID]
// 	if !ok || cs.State != StateCheckIn {
// 		return
// 	}
// 	prompt := fmt.Sprintf("Please enter a 6 digit code for %s", cs.CurrentEvent.Name)
// 	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 		ChatID: chatID,
// 		Text:   prompt,
// 	})
// 	if err != nil {
// 		log.Println("error sending try again check-in prompt:", err)
// 	}
// }

// // cancelCheckInCallbackHandler resets the state and returns the user to the events list.
// func cancelCheckInCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
// 	chatID := mes.Message.Chat.ID
// 	if cs, ok := chatStates[chatID]; ok {
// 		cs.State = StateIdle
// 	}
// 	// Return to the events list.
// 	viewEventsHandler(ctx, b, &models.Update{Message: mes.Message})
// }

// // viewEventsKeyboardSelect handles event selection callbacks.
// func viewEventsKeyboardSelect(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
// 	cmd := string(data)
// 	if cmd == "cancel" {
// 		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 			ChatID: mes.Message.Chat.ID,
// 			Text:   "Operation cancelled. You can view events using /viewEvents.",
// 		})
// 		if err != nil {
// 			log.Println("error sending cancel message:", err)
// 		}
// 		return
// 	}
// 	var index int
// 	_, err := fmt.Sscanf(cmd, "viewEvent_%d", &index)
// 	if err != nil {
// 		log.Println("error parsing event index:", err)
// 		return
// 	}
// 	showEventDetails(ctx, b, mes.Message.Chat.ID, index)
// }

// showEventDetails displays details of the selected event.
// It also updates the chat's current event and adds an inline keyboard
// that includes a "Check In" button.
// func showEventDetails(ctx context.Context, b *bot.Bot, chatID int64, index int) {
// 	if index < 0 || index >= len(events) {
// 		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 			ChatID: chatID,
// 			Text:   "Invalid event selected.",
// 		})
// 		if err != nil {
// 			log.Println("error sending message:", err)
// 		}
// 		return
// 	}
// 	event := events[index]

// 	// Update the chat state with the current event.
// 	cs, ok := chatStates[chatID]
// 	if !ok {
// 		cs = &ChatState{State: StateIdle}
// 		chatStates[chatID] = cs
// 	}
// 	cs.CurrentEvent = event

// 	detailsText := fmt.Sprintf(
// 		"Event Details:\n\nTitle: %s\nDate & Time: %s\nLocation: %s\n\nDescription: %s",
// 		event.Name,
// 		event.Date,
// 		event.Location,
// 		event.Description,
// 	)
// 	kb := inline.New(b)
// 	// Add buttons for FAQ and Personal Notes (existing flow)
// 	kb.Row().Button("FAQ", []byte(fmt.Sprintf("faq_%d", index)), faqCallbackHandler)

// 	parsedTime, err := time.Parse("2006-01-02 15:04", event.Date)
// 	if err != nil {
// 		log.Println("error parsing event date:", err)
// 		// Handle error (perhaps assume event is not in the past)
// 	}
// 	var isEventInThePast = parsedTime.Before(time.Now())

// 	if !isEventInThePast {
// 		// NEW: Add Check In button specific to this event.
// 		kb.Button("Check In", []byte(fmt.Sprintf("checkIn_%d", index)), checkInCallbackHandler)
// 	}
// 	// Print boolean and string using fmt.Printf
// 	fmt.Printf("Boolean: %t, String: %s\n", isEventInThePast, event.Date)

// 	kb.Row().Button("Add Own Notes for Reference", []byte(fmt.Sprintf("notes_%d", index)), personalNotesCallbackHandler)

// 	// Back button to return to the events list.
// 	kb.Row().Button("Back", []byte(fmt.Sprintf("back_%d", index)), backToEventsCallbackHandler)
// 	// Send event details (with photo if available)
// 	if event.MediaImage != "" {
// 		_, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
// 			ChatID:      chatID,
// 			Photo:       &models.InputFileString{Data: event.MediaImage},
// 			Caption:     detailsText,
// 			ReplyMarkup: kb,
// 		})
// 		if err != nil {
// 			log.Println("error sending photo with details:", err)
// 		}
// 	} else {
// 		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 			ChatID:      chatID,
// 			Text:        detailsText,
// 			ReplyMarkup: kb,
// 		})
// 		if err != nil {
// 			log.Println("error sending event details:", err)
// 		}
// 	}
// }

// // ------------------- Personal Notes, FAQ, and Back Flows -------------------

// // (Existing functions for personal notes, FAQ, and back navigation remain unchanged.)

// func personalNotesCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
// 	input := string(data)
// 	const prefix = "notes_"
// 	if len(input) <= len(prefix) || input[:len(prefix)] != prefix {
// 		log.Printf("invalid callback data format, expected prefix 'notes_', got: %q", input)
// 		return
// 	}
// 	var index int
// 	_, err := fmt.Sscanf(input, "notes_%d", &index)
// 	if err != nil {
// 		log.Println("error parsing personal notes index:", err)
// 		return
// 	}
// 	chatID := mes.Message.Chat.ID
// 	cs, ok := chatStates[chatID]
// 	if !ok {
// 		cs = &ChatState{}
// 		chatStates[chatID] = cs
// 	}
// 	cs.State = StatePersonalNotes
// 	cs.PersonalNoteEventIndex = index
// 	log.Printf("Chat %d set to StatePersonalNotes for event %d", chatID, index)
// 	showPersonalNotes(ctx, b, chatID, index)
// }

// func showPersonalNotes(ctx context.Context, b *bot.Bot, chatID int64, index int) {
// 	notesText := "Your Saved Notes (Tap to Copy):\n<code>{placeholder}</code>\n\n\nDo know that by replying to this, you will overwrite your prior saved notes"
// 	kb := inline.New(b)
// 	kb.Row().Button("Back", []byte(fmt.Sprintf("back_%d", index)), backToEventDetailsCallbackHandler)
// 	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 		ChatID:      chatID,
// 		Text:        notesText,
// 		ReplyMarkup: kb,
// 		ParseMode:   "HTML",
// 	})
// 	if err != nil {
// 		log.Println("error sending personal notes message:", err)
// 	}
// }

// func handlePersonalNotesReply(ctx context.Context, b *bot.Bot, update *models.Update) {
// 	chatID := update.Message.Chat.ID
// 	cs, ok := chatStates[chatID]
// 	if !ok || cs.State != StatePersonalNotes {
// 		return
// 	}
// 	cs.PersonalNote = update.Message.Text
// 	cs.State = StateIdle
// 	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 		ChatID: chatID,
// 		Text:   "Your note has been saved!",
// 	})
// 	if err != nil {
// 		log.Println("error sending note confirmation:", err)
// 	}
// 	showEventDetails(ctx, b, chatID, cs.PersonalNoteEventIndex)
// }

// func backToEventDetailsCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
// 	var index int
// 	_, err := fmt.Sscanf(string(data), "back_%d", &index)
// 	if err != nil {
// 		log.Println("error parsing back index:", err)
// 		return
// 	}
// 	chatID := mes.Message.Chat.ID
// 	if cs, ok := chatStates[chatID]; ok {
// 		cs.State = StateIdle
// 	}
// 	showEventDetails(ctx, b, chatID, index)
// }

// func backToEventsCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
// 	viewEventsHandler(ctx, b, &models.Update{Message: mes.Message})
// }

// func faqCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
// 	input := string(data)
// 	const prefix = "faq_"
// 	if len(input) <= len(prefix) || input[:len(prefix)] != prefix {
// 		log.Printf("invalid callback data format, expected prefix 'faq_', got: %q", input)
// 		return
// 	}
// 	var index int
// 	_, err := fmt.Sscanf(input, "faq_%d", &index)
// 	if err != nil {
// 		log.Println("error parsing FAQ index:", err)
// 		return
// 	}
// 	showFAQ(ctx, b, mes.Message.Chat.ID, index)
// }

// func showFAQ(ctx context.Context, b *bot.Bot, chatID int64, index int) {
// 	events := fakeEvents()
// 	if index < 0 || index >= len(events) {
// 		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 			ChatID: chatID,
// 			Text:   "Invalid event for FAQ.",
// 		})
// 		if err != nil {
// 			log.Println("error sending message:", err)
// 		}
// 		return
// 	}
// 	event := events[index]
// 	faqText := "FAQs:\n1. Q: What is this event?\n   A: It's a demo event.\n\n2. Q: How can I sign up?\n   A: Visit our website for details."
// 	kb := inline.New(b)
// 	kb.Row().Button("Back", []byte(fmt.Sprintf("back_%d", index)), backToEventDetailsCallbackHandler)
// 	if event.MediaImage != "" {
// 		_, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
// 			ChatID:      chatID,
// 			Photo:       &models.InputFileString{Data: event.MediaImage},
// 			Caption:     faqText,
// 			ReplyMarkup: kb,
// 		})
// 		if err != nil {
// 			log.Println("error sending FAQ photo:", err)
// 		}
// 	} else {
// 		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
// 			ChatID:      chatID,
// 			Text:        faqText,
// 			ReplyMarkup: kb,
// 		})
// 		if err != nil {
// 			log.Println("error sending FAQ message:", err)
// 		}
// 	}
// }

// 	func checkEventExists(ctx context.Context, eventID string, cs *ChatState) (*model.Event, error) {
// 		//check if event exists
// 		for _, event := range events {
// 			if event.DocumentID == eventID {
// 				cs.EventRefKey = eventID
// 				return &model.Event{
// 					Name: event.Name,
// 				}, nil
// 			}
// 		}
// 		return nil, fmt.Errorf("event with ID '%s' not found", eventID)
// 	}

// 	func addParticipantToEvent(ctx context.Context, eventID string, user *models.User, cs *ChatState) error {
// 		// Add the user to the event's participant list
// 		// In a real implementation, you would interact with Firestore here.
// 		// For this example, we'll just log the action.
// 		log.Printf("User %s (%d) joined event %s", user.FirstName, user.ID, eventID)
// 		return nil
// 	 }
