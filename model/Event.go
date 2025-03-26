package model

type Event struct {
	Name         string        `firestore:"name"`
	EDMFileID    string        `firestore:"edmFileID"`
	EventDetails []QnA         `firestore:"eventDetails"`
	Participants []Participant `firestore:"participants"`

	//firebase
	DocumentID string `firestore:"documentID"`
}

type QnA struct {
	Question string
	Answer   string
}

type Participant struct {
	Name string `firestore:"name"`
	QnAs []QnA  `firestore:"qnas"`
	Code string `firestore:"code"`
}
