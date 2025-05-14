package model

type UserState struct {
	State               int
	CurrentEvent        *Event
	LastQuestion        string        // Store the last question asked
	CurrentRSVPQuestion *RSVPQuestion // Current RSVP question being created
	RSVPQuestionIndex   int           // Index of current RSVP question being answered
	TempOptions         []string      // Temporary storage for MCQ or MultiSelect options
}
