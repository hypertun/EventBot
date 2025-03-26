package handler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/go-telegram/ui/keyboard/inline"
)

// ------------------- Data Structures -------------------

// ChatState holds the state data for a given chat.
type ChatState struct {
	State        int
	CurrentEvent Event

	// Additional fields (such as personal notes) remain available.
	SignedUpEvents         []Event
	PersonalNoteEventIndex int    // Event index for which notes are being edited.
	PersonalNote           string // The chat's personal note.
}

// Global map to store chat states by chat ID.
var chatStates = make(map[int64]*ChatState)

// Event represents an event with several details.
type Event struct {
	Date        string // Date and time (formatted)
	Name        string // Title of the event
	Location    string // Event location
	MediaImage  string // A file ID or URL for the event image
	Description string // Short description of the event
}

// fakeEvents generates 15 fake events.
func fakeEvents() []Event {
	events := make([]Event, 15)
	for i := 0; i < 15; i++ {
		dateTime := time.Now().AddDate(0, 0, i).Format("2006-01-02 15:04")

		if i < 5 {
			dateTime = "2023-01-02 15:04"
		}
		events[i] = Event{
			Date:        dateTime,
			Name:        fmt.Sprintf("Tech Innovators Summit 2029 - Event %d", i+1),
			Location:    fmt.Sprintf("Silicon Valley Convention Center, San Jose, CA Postal Code: 12324%d", i+1),
			MediaImage:  "https://ik.imagekit.io/l2o8gnuspz/vd/application/files/4717/3582/7541/Derby_Fake_Festival_2025_event_artwork.jpg?tr=w-790,ar-790-495",
			Description: "The Tech Innovators Summit 2023 is the premier gathering for tech enthusiasts, entrepreneurs, and industry leaders. This two-day event will feature keynote speeches, panel discussions, and hands-on workshops. Whether you're a startup founder, developer, or simply passionate about innovation, this is the place to be!",
		}
	}
	return events
}

// ------------------- Check-In Flow -------------------

// checkInCallbackHandler is triggered when a user taps the "Check In" button
// in the event details inline keyboard.
func checkInCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	// We expect callback data in the form "checkIn_{index}".
	var index int
	_, err := fmt.Sscanf(string(data), "checkIn_%d", &index)
	if err != nil {
		log.Println("error parsing check-in callback index:", err)
		return
	}
	// In showEventDetails we already set the current event in chat state.
	checkInHandler(ctx, b, mes.Message.Chat.ID)
}

// checkInHandler verifies that a current event is selected (via showEventDetails)
// and then prompts the user to enter the 6‑digit check‑in code.
func checkInHandler(ctx context.Context, b *bot.Bot, chatID int64) {
	cs, ok := chatStates[chatID]
	if !ok || cs.CurrentEvent.Name == "" {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "No event selected. Please use /viewEvents to select an event first.",
		})
		if err != nil {
			log.Println("error sending no event message:", err)
		}
		return
	}
	// Set state to check-in.
	cs.State = StateCheckIn
	prompt := fmt.Sprintf("Please enter a 6 digit code for %s", cs.CurrentEvent.Name)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   prompt,
	})
	if err != nil {
		log.Println("error sending check-in prompt:", err)
	}
}

// handleCheckInReply processes the user's reply containing the check‑in code.
func handleCheckInReply(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	cs, ok := chatStates[chatID]
	if !ok || cs.State != StateCheckIn {
		// Not in check-in mode; ignore.
		return
	}
	code := update.Message.Text
	if code != "123456" {
		// Create an inline keyboard with two buttons.
		kb := inline.New(b)
		kb.Row().Button("Try Again", []byte("try_again_checkin"), tryAgainCheckInCallbackHandler)
		kb.Row().Button("Cancel", []byte("cancel_checkin"), cancelCheckInCallbackHandler)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        fmt.Sprintf("Incorrect code. Check in failed. Please try again by entering the correct 6 digit code for %s", cs.CurrentEvent.Name),
			ReplyMarkup: kb,
		})
		if err != nil {
			log.Println("error sending check-in failure message:", err)
		}
		// Do not reset state so the user remains in check-in mode.
		return
	}
	// Successful check-in.
	username := update.Message.From.Username
	if username == "" {
		username = update.Message.From.FirstName
	}
	successMsg := fmt.Sprintf("Thank you %s, check in to %s is successful. Enjoy your time here!", username, cs.CurrentEvent.Name)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   successMsg,
	})
	if err != nil {
		log.Println("error sending check-in success message:", err)
	}
	cs.State = StateIdle
}

// tryAgainCheckInCallbackHandler re-prompts the user for the check-in code.
func tryAgainCheckInCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	chatID := mes.Message.Chat.ID
	cs, ok := chatStates[chatID]
	if !ok || cs.State != StateCheckIn {
		return
	}
	prompt := fmt.Sprintf("Please enter a 6 digit code for %s", cs.CurrentEvent.Name)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   prompt,
	})
	if err != nil {
		log.Println("error sending try again check-in prompt:", err)
	}
}

// cancelCheckInCallbackHandler resets the state and returns the user to the events list.
func cancelCheckInCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	chatID := mes.Message.Chat.ID
	if cs, ok := chatStates[chatID]; ok {
		cs.State = StateIdle
	}
	// Return to the events list.
	viewEventsHandler(ctx, b, &models.Update{Message: mes.Message})
}

// ------------------- Past Event Details, FAQ, and Personal Notes -------------------

// pastEventsHandler builds an inline keyboard with one event per row.
func pastEventsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	events := fakeEvents()
	kb := inline.New(b)
	for i, event := range events {
		rawLabel := fmt.Sprintf("%s - %s", event.Date, event.Name)
		label := fmt.Sprintf("%-30s", rawLabel)
		callbackData := []byte(fmt.Sprintf("viewEvent_%d", i))
		kb.Row().Button(label, callbackData, pastEventsKeyboardSelect)
	}
	kb.Row().Button("Cancel", []byte("cancel"), pastEventsKeyboardSelect)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "These are all your upcoming events that you have signed up for:",
		ReplyMarkup: kb,
	})
	if err != nil {
		log.Println("error sending event keyboard:", err)
	}
}

// viewEventsKeyboardSelect handles the callback when an event button is pressed.
func pastEventsKeyboardSelect(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	cmd := string(data)
	if cmd == "cancel" {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: mes.Message.Chat.ID,
			Text:   "Take a look at your upcoming events using /viewEvents commands. For more help, look at /help",
		})
		if err != nil {
			log.Println("error sending cancel message:", err)
		}
		return
	}
	var index int
	_, err := fmt.Sscanf(cmd, "viewEvent_%d", &index)
	if err != nil {
		log.Println("error parsing event index:", err)
		return
	}
	showEventDetails(ctx, b, mes.Message.Chat.ID, index)
}

// ------------------- Upcoming Events Flow -------------------

// viewEventsHandler builds an inline keyboard listing upcoming events.
func viewEventsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	events := fakeEvents()
	kb := inline.New(b)
	for i, event := range events {
		rawLabel := fmt.Sprintf("%s - %s", event.Date, event.Name)
		label := fmt.Sprintf("%-30s", rawLabel)
		// The callback for each event is "viewEvent_{index}"
		callbackData := []byte(fmt.Sprintf("viewEvent_%d", i))
		kb.Row().Button(label, callbackData, viewEventsKeyboardSelect)
	}
	kb.Row().Button("Cancel", []byte("cancel"), viewEventsKeyboardSelect)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "These are your upcoming events. Please select one:",
		ReplyMarkup: kb,
	})
	if err != nil {
		log.Println("error sending event keyboard:", err)
	}
}

// viewEventsKeyboardSelect handles event selection callbacks.
func viewEventsKeyboardSelect(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	cmd := string(data)
	if cmd == "cancel" {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: mes.Message.Chat.ID,
			Text:   "Operation cancelled. You can view events using /viewEvents.",
		})
		if err != nil {
			log.Println("error sending cancel message:", err)
		}
		return
	}
	var index int
	_, err := fmt.Sscanf(cmd, "viewEvent_%d", &index)
	if err != nil {
		log.Println("error parsing event index:", err)
		return
	}
	showEventDetails(ctx, b, mes.Message.Chat.ID, index)
}

// showEventDetails displays details of the selected event.
// It also updates the chat's current event and adds an inline keyboard
// that includes a "Check In" button.
func showEventDetails(ctx context.Context, b *bot.Bot, chatID int64, index int) {
	events := fakeEvents()
	if index < 0 || index >= len(events) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Invalid event selected.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}
	event := events[index]

	// Update the chat state with the current event.
	cs, ok := chatStates[chatID]
	if !ok {
		cs = &ChatState{State: StateIdle}
		chatStates[chatID] = cs
	}
	cs.CurrentEvent = event

	detailsText := fmt.Sprintf(
		"Event Details:\n\nTitle: %s\nDate & Time: %s\nLocation: %s\n\nDescription: %s",
		event.Name,
		event.Date,
		event.Location,
		event.Description,
	)
	kb := inline.New(b)
	// Add buttons for FAQ and Personal Notes (existing flow)
	kb.Row().Button("FAQ", []byte(fmt.Sprintf("faq_%d", index)), faqCallbackHandler)

	parsedTime, err := time.Parse("2006-01-02 15:04", event.Date)
	if err != nil {
		log.Println("error parsing event date:", err)
		// Handle error (perhaps assume event is not in the past)
	}
	var isEventInThePast = parsedTime.Before(time.Now())

	if !isEventInThePast {
		// NEW: Add Check In button specific to this event.
		kb.Button("Check In", []byte(fmt.Sprintf("checkIn_%d", index)), checkInCallbackHandler)
	}
	// Print boolean and string using fmt.Printf
	fmt.Printf("Boolean: %t, String: %s\n", isEventInThePast, event.Date)

	kb.Row().Button("Add Own Notes for Reference", []byte(fmt.Sprintf("notes_%d", index)), personalNotesCallbackHandler)

	// Back button to return to the events list.
	kb.Row().Button("Back", []byte(fmt.Sprintf("back_%d", index)), backToEventsCallbackHandler)
	// Send event details (with photo if available)
	if event.MediaImage != "" {
		_, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:      chatID,
			Photo:       &models.InputFileString{Data: event.MediaImage},
			Caption:     detailsText,
			ReplyMarkup: kb,
		})
		if err != nil {
			log.Println("error sending photo with details:", err)
		}
	} else {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        detailsText,
			ReplyMarkup: kb,
		})
		if err != nil {
			log.Println("error sending event details:", err)
		}
	}
}

// ------------------- Personal Notes, FAQ, and Back Flows -------------------

// (Existing functions for personal notes, FAQ, and back navigation remain unchanged.)

func personalNotesCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	input := string(data)
	const prefix = "notes_"
	if len(input) <= len(prefix) || input[:len(prefix)] != prefix {
		log.Printf("invalid callback data format, expected prefix 'notes_', got: %q", input)
		return
	}
	var index int
	_, err := fmt.Sscanf(input, "notes_%d", &index)
	if err != nil {
		log.Println("error parsing personal notes index:", err)
		return
	}
	chatID := mes.Message.Chat.ID
	cs, ok := chatStates[chatID]
	if !ok {
		cs = &ChatState{}
		chatStates[chatID] = cs
	}
	cs.State = StatePersonalNotes
	cs.PersonalNoteEventIndex = index
	log.Printf("Chat %d set to StatePersonalNotes for event %d", chatID, index)
	showPersonalNotes(ctx, b, chatID, index)
}

func showPersonalNotes(ctx context.Context, b *bot.Bot, chatID int64, index int) {
	notesText := "Your Saved Notes (Tap to Copy):\n<code>{placeholder}</code>\n\n\nDo know that by replying to this, you will overwrite your prior saved notes"
	kb := inline.New(b)
	kb.Row().Button("Back", []byte(fmt.Sprintf("back_%d", index)), backToEventDetailsCallbackHandler)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        notesText,
		ReplyMarkup: kb,
		ParseMode:   "HTML",
	})
	if err != nil {
		log.Println("error sending personal notes message:", err)
	}
}

func handlePersonalNotesReply(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	cs, ok := chatStates[chatID]
	if !ok || cs.State != StatePersonalNotes {
		return
	}
	cs.PersonalNote = update.Message.Text
	cs.State = StateIdle
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Your note has been saved!",
	})
	if err != nil {
		log.Println("error sending note confirmation:", err)
	}
	showEventDetails(ctx, b, chatID, cs.PersonalNoteEventIndex)
}

func backToEventDetailsCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	var index int
	_, err := fmt.Sscanf(string(data), "back_%d", &index)
	if err != nil {
		log.Println("error parsing back index:", err)
		return
	}
	chatID := mes.Message.Chat.ID
	if cs, ok := chatStates[chatID]; ok {
		cs.State = StateIdle
	}
	showEventDetails(ctx, b, chatID, index)
}

func backToEventsCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	viewEventsHandler(ctx, b, &models.Update{Message: mes.Message})
}

func faqCallbackHandler(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	input := string(data)
	const prefix = "faq_"
	if len(input) <= len(prefix) || input[:len(prefix)] != prefix {
		log.Printf("invalid callback data format, expected prefix 'faq_', got: %q", input)
		return
	}
	var index int
	_, err := fmt.Sscanf(input, "faq_%d", &index)
	if err != nil {
		log.Println("error parsing FAQ index:", err)
		return
	}
	showFAQ(ctx, b, mes.Message.Chat.ID, index)
}

func showFAQ(ctx context.Context, b *bot.Bot, chatID int64, index int) {
	events := fakeEvents()
	if index < 0 || index >= len(events) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Invalid event for FAQ.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}
	event := events[index]
	faqText := "FAQs:\n1. Q: What is this event?\n   A: It's a demo event.\n\n2. Q: How can I sign up?\n   A: Visit our website for details."
	kb := inline.New(b)
	kb.Row().Button("Back", []byte(fmt.Sprintf("back_%d", index)), backToEventDetailsCallbackHandler)
	if event.MediaImage != "" {
		_, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:      chatID,
			Photo:       &models.InputFileString{Data: event.MediaImage},
			Caption:     faqText,
			ReplyMarkup: kb,
		})
		if err != nil {
			log.Println("error sending FAQ photo:", err)
		}
	} else {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        faqText,
			ReplyMarkup: kb,
		})
		if err != nil {
			log.Println("error sending FAQ message:", err)
		}
	}
}

// ------------------- Main Commands and Participant Handler -------------------

// viewAllCommandsHandler builds an inline keyboard with command buttons.
// Notice that /checkIn is now omitted from this list.
func viewAllCommandsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	commands := []string{"/start", "/viewEvents", "/pastEvents", "/help"}
	kb := inline.New(b)
	for i, cmd := range commands {
		if i%2 == 0 {
			kb.Row()
		}
		kb.Button(cmd, []byte(cmd), viewAllCommandsKeyboardSelect)
	}
	kb.Row().Button("Cancel", []byte("cancel"), viewAllCommandsKeyboardSelect)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "Commands\n\nUse /viewEvents to see your upcoming events, /pastEvents for past events, and /help for assistance.",
		ReplyMarkup: kb,
	})
	if err != nil {
		log.Println("error sending command keyboard:", err)
	}
}

// viewAllCommandsKeyboardSelect creates a new update with the selected command.
func viewAllCommandsKeyboardSelect(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	cmd := string(data)
	if cmd == "cancel" {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: mes.Message.Chat.ID,
			Text:   "Command cancelled.",
		})
		if err != nil {
			log.Println("error sending cancel message:", err)
		}
		return
	}
	newUpdate := &models.Update{
		Message: &models.Message{
			Chat: mes.Message.Chat,
			From: mes.Message.From,
			Text: cmd,
		},
	}
	ParticipantHandler(ctx, b, newUpdate)
}

// ParticipantHandler is the main update handler.
func ParticipantHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	chatID := update.Message.Chat.ID

	// Retrieve or initialize chat state.
	cs, ok := chatStates[chatID]
	if !ok {
		cs = &ChatState{State: StateIdle}
		chatStates[chatID] = cs
	}
	log.Printf("Chat %d [%s] %s (state: %d)", chatID, update.Message.From.Username, update.Message.Text, cs.State)

	// Special state handlers.
	switch cs.State {
	case StatePersonalNotes:
		handlePersonalNotesReply(ctx, b, update)
		return
	case StateCheckIn:
		handleCheckInReply(ctx, b, update)
		return
	}

	// Process standard commands.
	var text string
	switch update.Message.Text {
	case "/start":
		username := update.Message.From.Username
		if username == "" {
			username = update.Message.From.FirstName
		}
		text = fmt.Sprintf("Hey %s! I'm your friendly event companion, here to make attending your events smooth and enjoyable—now and in the future.\n\nHere's how I can help:\n• Quickly view events you're attending: /viewEvents\n• Revisit past events: /pastEvents\n• Easily check-in at events using a simple code\n• Access useful event details and FAQs\n• Keep track of your own notes and reminders for each event\n\nJust type /help anytime to see what else I can do for you!", username)
	case "/help":
		viewAllCommandsHandler(ctx, b, update)
		return
	case "/viewEvents":
		viewEventsHandler(ctx, b, update)
		return
	case "/pastEvents":
		pastEventsHandler(ctx, b, update)
		return
	default:
		// If the user sends an unknown command in idle state.
		text = "I didn't understand that command. Please use /start, /viewEvents, /pastEvents, or /help."
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		log.Println("error sending message:", err)
	}
}
