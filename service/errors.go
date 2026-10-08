package service

import "slots/repo"

// Re-exported so callers of the service don't need to import the repo package.
var (
	ErrNotFound          = repo.ErrNotFound
	ErrDuplicate         = repo.ErrDuplicate
	ErrInvalidStatus     = repo.ErrInvalidStatus
	ErrSlotAlreadyBooked = repo.ErrSlotAlreadyBooked
	ErrSlotOnHold        = repo.ErrSlotOnHold
	ErrSlotNotBooked     = repo.ErrSlotNotBooked
	ErrSlotNotOwned      = repo.ErrSlotNotOwned
	ErrConcurrentUpdate  = repo.ErrConcurrentUpdate
	ErrSameSlot          = repo.ErrSameSlot
)

type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string {
	return e.Msg
}

func invalid(msg string) error {
	return &ValidationError{Msg: msg}
}
