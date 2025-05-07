package model

import "errors"

var (
	ErrParticipantDoesNotExist = errors.New("participant do not exist")
	ErrEventDoesNotExist       = errors.New("event do not exist")
)
