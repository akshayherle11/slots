package service

import (
	"context"
	"fmt"
	"slots/models"
	"slots/repo"
	"time"
)

type SlotsService interface {
	GetById(ctx context.Context, id int) (models.Slot, error)
	GetAll(ctx context.Context) ([]models.Slot, error)
	Hold(ctx context.Context, slotId, userId int) (models.Slot, error)
	Book(ctx context.Context, slotId, userId int) (models.Slot, error)
	Cancel(ctx context.Context, slotId, userId int) (models.Slot, error)
	Reschedule(ctx context.Context, fromSlotId, toSlotId, userId int) (models.Slot, error)
	AddSlot(ctx context.Context, slot models.Slot) (models.Slot, error)
	AddBulk(ctx context.Context, slots []models.Slot) ([]models.Slot, error)
}

type slotsService struct {
	slots   repo.SlotsRepo
	holdFor time.Duration
	now     func() time.Time
}

// NewSlotsService returns a SlotsService where a hold lasts holdFor from the slot's last update.
func NewSlotsService(slots repo.SlotsRepo, holdFor time.Duration) SlotsService {
	return &slotsService{slots: slots, holdFor: holdFor, now: time.Now}
}

// Every slot returned to callers goes through present, so expired holds read as not booked.
func (s *slotsService) present(slot models.Slot, err error) (models.Slot, error) {
	if err != nil {
		return models.Slot{}, err
	}
	slot.ResolveHold(s.now(), s.holdFor)
	return slot, nil
}

func (s *slotsService) GetById(ctx context.Context, id int) (models.Slot, error) {
	return s.present(s.slots.GetById(ctx, id))
}

func (s *slotsService) GetAll(ctx context.Context) ([]models.Slot, error) {
	slots, err := s.slots.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	for i := range slots {
		slots[i].ResolveHold(now, s.holdFor)
	}
	return slots, nil
}

func (s *slotsService) Hold(ctx context.Context, slotId, userId int) (models.Slot, error) {
	return s.present(s.slots.UpdateSlotStatus(ctx, slotId, userId, models.SlotStatusOnHold))
}

func (s *slotsService) Book(ctx context.Context, slotId, userId int) (models.Slot, error) {
	return s.present(s.slots.UpdateSlotStatus(ctx, slotId, userId, models.SlotStatusBooked))
}

func (s *slotsService) Cancel(ctx context.Context, slotId, userId int) (models.Slot, error) {
	return s.present(s.slots.UpdateSlotStatus(ctx, slotId, userId, models.SlotStatusNotBooked))
}

func (s *slotsService) Reschedule(ctx context.Context, fromSlotId, toSlotId, userId int) (models.Slot, error) {
	if fromSlotId == toSlotId {
		return models.Slot{}, ErrSameSlot
	}
	return s.present(s.slots.Reschedule(ctx, fromSlotId, toSlotId, userId))
}

func (s *slotsService) AddSlot(ctx context.Context, slot models.Slot) (models.Slot, error) {
	if err := prepareNewSlot(&slot); err != nil {
		return models.Slot{}, err
	}
	return s.slots.AddSlot(ctx, slot)
}

func (s *slotsService) AddBulk(ctx context.Context, slots []models.Slot) ([]models.Slot, error) {
	if len(slots) == 0 {
		return nil, invalid("at least one slot is required")
	}
	for i := range slots {
		if err := prepareNewSlot(&slots[i]); err != nil {
			return nil, invalid(fmt.Sprintf("slots[%d]: %s", i, err))
		}
	}
	return s.slots.AddBulk(ctx, slots)
}

// prepareNewSlot validates a slot and resets fields that clients must not set on creation.
func prepareNewSlot(slot *models.Slot) error {
	if slot.Date.IsZero() || slot.From.IsZero() || slot.To.IsZero() {
		return invalid("date, from and to are required")
	}
	if !slot.From.Before(slot.To) {
		return invalid("from must be before to")
	}
	slot.ID = 0
	slot.Status = models.SlotStatusNotBooked
	slot.BookedBy = nil
	slot.User = nil
	return nil
}
