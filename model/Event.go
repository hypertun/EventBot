package model

import "time"

type Event struct {
	ID           string    `firestore:"id"`
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
	QnAs   []QnA  `firestore:"qnas"`
	Code   string `firestore:"code"`

	SignedUpEvents []string `firestore:"signedUpEvents"` //list of events by id
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
	StateAddingEventDetailsAnswer = iota + 10

	//Participant Bot
	StateCheckIn
	StatePersonalNotes
	StateJoinEvent
)
