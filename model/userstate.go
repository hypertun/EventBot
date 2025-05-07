package model

type UserState struct {
	State        int
	CurrentEvent *Event
	LastQuestion string // Store the last question asked
}
