package repo

import (
	"errors"

	"gorm.io/gorm"
)

var (
	ErrNotFound          = errors.New("record not found")
	ErrDuplicate         = errors.New("record already exists")
	ErrInvalidStatus     = errors.New("invalid slot status")
	ErrSlotAlreadyBooked = errors.New("slot is already booked")
	ErrSlotOnHold        = errors.New("slot is on hold")
	ErrSlotNotBooked     = errors.New("slot is not booked")
	ErrSlotNotOwned      = errors.New("slot is booked by another user")
	ErrConcurrentUpdate  = errors.New("slot was modified concurrently, retry")
	ErrSameSlot          = errors.New("cannot reschedule to the same slot")
)

func mapErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicate
	}
	return err
}
