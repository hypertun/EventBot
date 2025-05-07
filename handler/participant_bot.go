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
			Keep track of your own notes and reminders for each event: /notes
			Check in to an event: /checkIn


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
			/notes - Add or view personal notes for an event.
			/checkIn - Check in to an event.

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
			text = "Please provide the Event Reference Code of the event you want to join."
			userState.State = model.StateJoinEvent
		case "/notes":
			text = `Please provide the Event Reference Code to view/add personal notes.`
			userState.State = model.StatePersonalNotes
		case "/checkIn":
			text = "Please provide the Event Reference Code of the event you want to check in to."
			userState.State = model.StateCheckIn
		default:
			text = "I didn't understand that command. Use /start or /help."
		}
	case model.StateJoinEvent:
		p.handleJoinEvent(ctx)
		userState.State = model.StateIdle
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

func (p *ParticipantBotHandler) handleCheckIn(ctx context.Context) {
	userID := p.update.Message.From.ID
	if userPBotStates[userID].CurrentEvent == nil {
		event, err := p.FirebaseConnector.ReadEvent(ctx, p.update.Message.Text)
		if err != nil {
			log.Println("error reading event:", err)
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "Error checking you in. Please try again.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userPBotStates[p.update.Message.From.ID].State = model.StateIdle
			return
		}
		if event.EventDate.Format(time.DateOnly) != time.Now().Format(time.DateOnly) {
			_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: p.update.Message.Chat.ID,
				Text:   "You can only check in to this event on the date of the event itself.",
			})
			if err != nil {
				log.Println("error sending message:", err)
			}
			userPBotStates[p.update.Message.From.ID].State = model.StateIdle
			return
		}

		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "Please key in your check in code.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		userPBotStates[userID].CurrentEvent = event
		return
	}

	defer func() {
		userPBotStates[p.update.Message.From.ID].State = model.StateIdle
		userPBotStates[p.update.Message.From.ID].CurrentEvent = nil
	}()

	eventID := userPBotStates[p.update.Message.From.ID].CurrentEvent.ID
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

	if p.update.Message.Text != participant.Code {
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "Wrong check in code. Please try again.",
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
			Text:   "You are not registered for this event. Please join the event first.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	if signedUpEvent.CheckedIn {
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "You have already checked in to this event.",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	if participant.Code != p.update.Message.Text {
		_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: p.update.Message.Chat.ID,
			Text:   "You entered the wrong check in",
		})
		if err != nil {
			log.Println("error sending message:", err)
		}
		return
	}

	signedUpEvent.CheckedIn = true
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
		return
	}

	_, err = p.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: p.update.Message.Chat.ID,
		Text:   "You have successfully checked in!",
	})
	if err != nil {
		log.Println("error sending message:", err)
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
		Text: fmt.Sprintf(`You have successfully joined event '%s'!
This is you checkIn code: %s
		`, eventID, participant.Code),
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
