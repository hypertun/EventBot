package model

import "time"

// QuestionType defines the type of question for RSVP
type QuestionType int

const (
	// RSVP Question Types
	QuestionTypeYesNo QuestionType = iota
	QuestionTypeMCQ
	QuestionTypeMultiSelect
	QuestionTypeShortAnswer
)

type Event struct {
	ID            string         `firestore:"id"`
	UserID        int64          `firestore:"userid"`
	Name          string         `firestore:"name"`
	EDMFileID     string         `firestore:"edmFileID"`
	EDMFileURL    string         `firestore:"edmFileURL"`
	EventDate     time.Time      `firestore:"eventDate"`
	EventDetails  []QnA          `firestore:"eventDetails"`
	RSVPQuestions []RSVPQuestion `firestore:"rsvpQuestions"`
	Participants  []string       `firestore:"participants"` //list of participants by id
	CheckInCode   string         `firestore:"checkInCode"`  // New field for the check-in code set by organizer
}

type QnA struct {
	Question     string `firestore:"question"`
	Answer       string `firestore:"answer"`
	ImageFileID  string `firestore:"imageFileID"`  // Field for storing image file ID
	ImageFileURL string `firestore:"imageFileURL"` // Field for storing image URL
}

type RSVPQuestion struct {
	ID           string       `firestore:"id"`
	Question     string       `firestore:"question"`
	Type         QuestionType `firestore:"type"`
	Options      []string     `firestore:"options"`      // Used for MCQ and MultiSelect
	ImageFileID  string       `firestore:"imageFileID"`  // Optional image for the question
	ImageFileURL string       `firestore:"imageFileURL"` // URL to access the image
}

type RSVPAnswer struct {
	QuestionID string   `firestore:"questionID"`
	Answers    []string `firestore:"answers"` // Can be multiple for MultiSelect
}

type Participant struct {
	ID     string `firestore:"id"`
	UserID int64  `firestore:"userid"`
	Name   string `firestore:"name"`

	SignedUpEvents []SignedUpEvent `firestore:"signedUpEvents"` //list of events by id
}

type SignedUpEvent struct {
	EventID       string       `firestore:"eventID"`
	PersonalNotes string       `firestore:"personalNotes"`
	CheckedIn     bool         `firestore:"checkedIn"`
	RSVPAnswers   []RSVPAnswer `firestore:"rsvpAnswers"`
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
	StateAddingEventDetailsImage       // State for deciding about adding an image
	StateAddingEventDetailsImageUpload // State for uploading an image
	StateSettingEventCheckInCode       // New state for setting check-in code
	StateUpdatingEventCheckInCode      // New state for updating check-in code

	// New RSVP states for organiser
	StateAddingRSVP
	StateAddingRSVPQuestion
	StateSelectingRSVPType
	StateAddingRSVPOptions
	StateAddingRSVPImage
	StateConfirmRSVPQuestion

	//Participant Bot
	StateCheckIn
	StatePersonalNotes
	StateJoinEvent
	StateAnsweringRSVPQuestion
	StateSelectEventForRSVP
	StateEnteringCheckInCode // New state for entering check-in code
)
