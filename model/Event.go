package model

type Event struct {
	Name         string
	EDMFileID    string
	EventDetails []EventDetails

	//firebase
	DocumentID string
}

type EventDetails struct {
	Question string
	Answer   string
}
