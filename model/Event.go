package model

import "time"

type Event struct {
	ID           string    `firestore:"id"`
	UserID       int64     `firestore:"userid"`
	Name         string    `firestore:"name"`
	EDMFileID    string    `firestore:"edmFileID"`
	EventDate    time.Time `firestore:"eventDate"`
	EventDetails []QnA     `firestore:"eventDetails"`
	Participants []string  `firestore:"participants"` //list of participants by id
}

type QnA struct {
	Question string
	Answer   string
}

type Participant struct {
	ID     string `firestore:"id"`
	UserID int64  `firestore:"userid"`
	Name   string `firestore:"name"`
	Code   string `firestore:"code"`

	SignedUpEvents []SignedUpEvent `firestore:"signedUpEvents"` //list of events by id
}

type SignedUpEvent struct {
	EventID       string `firestore:"eventID"`
	PersonalNotes string `firestore:"personalNotes"`
	CheckedIn     bool   `firestore:"checkedIn"`
}

const (
	//Organiser States
	StateIdle = iota
	StateAddingEventName
	StateAddingEventDate
	StateAddingEventPicture
	StateAddingEventDetails
	StateDeleteEvent
	StateListParticipants
	StateBlastMessage
	StateAddingEventDetailsAnswer

	//Participant Bot
	StateCheckIn
	StatePersonalNotes
	StateJoinEvent
)
