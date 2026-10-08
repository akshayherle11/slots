package repo

import (
	"context"
	"slots/models"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const bulkInsertBatchSize = 100

type SlotsRepo interface {
	GetById(ctx context.Context, id int) (models.Slot, error)
	GetAll(ctx context.Context) ([]models.Slot, error)
	UpdateSlotStatus(ctx context.Context, slotId int, userId int, status models.SlotStatus) (models.Slot, error)
	Reschedule(ctx context.Context, fromSlotId, toSlotId, userId int) (models.Slot, error)
	AddSlot(ctx context.Context, slot models.Slot) (models.Slot, error)
	AddBulk(ctx context.Context, slots []models.Slot) ([]models.Slot, error)
}

type slotsRepo struct {
	db      *gorm.DB
	holdFor time.Duration
}

// NewSlotsRepo returns a SlotsRepo where a hold lasts holdFor from the slot's last update.
func NewSlotsRepo(db *gorm.DB, holdFor time.Duration) SlotsRepo {
	return &slotsRepo{db: db, holdFor: holdFor}
}

func (r *slotsRepo) GetById(ctx context.Context, id int) (models.Slot, error) {
	var slot models.Slot
	if err := r.db.WithContext(ctx).First(&slot, id).Error; err != nil {
		return models.Slot{}, mapErr(err)
	}
	return slot, nil
}

func (r *slotsRepo) GetAll(ctx context.Context) ([]models.Slot, error) {
	var slots []models.Slot
	err := r.db.WithContext(ctx).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "date"}}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "from"}}).
		Find(&slots).Error
	if err != nil {
		return nil, err
	}
	return slots, nil
}

// UpdateSlotStatus moves a slot to status on behalf of userId:
//   - OnHold:    hold a free slot (or one whose hold has expired) for holdFor.
//   - Booked:    book a free slot, or confirm the user's own active hold.
//   - NotBooked: cancel the user's booking or release their active hold.
//
// An expired hold counts as free for everyone; an active hold blocks everyone except the
// holder. The slot row is locked with SELECT ... FOR UPDATE inside a transaction, so
// concurrent requests for the same slot are serialized and only the first can take it.
func (r *slotsRepo) UpdateSlotStatus(ctx context.Context, slotId int, userId int, status models.SlotStatus) (models.Slot, error) {
	if !status.IsValid() {
		return models.Slot{}, ErrInvalidStatus
	}
	uid := int64(userId)

	var updated models.Slot
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slot models.Slot
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&slot, slotId).Error; err != nil {
			return mapErr(err)
		}

		// Decide on the effective state (expired hold => free); guard the write on the raw row.
		current := slot
		current.ResolveHold(time.Now(), r.holdFor)

		var updates map[string]any
		var action models.SlotAction
		switch status {
		case models.SlotStatusOnHold:
			switch {
			case current.IsBooked():
				return ErrSlotAlreadyBooked
			case current.IsOnHold():
				return ErrSlotOnHold
			}
			if err := requireUser(tx, userId); err != nil {
				return err
			}
			updates = map[string]any{"status": models.SlotStatusOnHold, "booked_by": uid}
			action = models.SlotActionHeld
		case models.SlotStatusBooked:
			switch {
			case current.IsBooked():
				return ErrSlotAlreadyBooked
			case current.IsOnHold() && !current.HeldOrBookedBy(uid):
				return ErrSlotOnHold
			}
			if err := requireUser(tx, userId); err != nil {
				return err
			}
			updates = map[string]any{"status": models.SlotStatusBooked, "booked_by": uid}
			action = models.SlotActionBooked
		case models.SlotStatusNotBooked:
			if current.Status == models.SlotStatusNotBooked {
				return ErrSlotNotBooked
			}
			if !current.HeldOrBookedBy(uid) {
				return ErrSlotNotOwned
			}
			updates = map[string]any{"status": models.SlotStatusNotBooked, "booked_by": nil}
			action = models.SlotActionUnbooked
		}

		if err := guardedUpdate(tx, slot, updates); err != nil {
			return err
		}

		history := models.SlotHistory{UserID: uid, SlotID: slot.ID, Action: action}
		if err := tx.Create(&history).Error; err != nil {
			return err
		}

		return tx.First(&updated, slotId).Error
	})
	if err != nil {
		return models.Slot{}, err
	}
	return updated, nil
}

// Reschedule moves userId's booking from fromSlotId to toSlotId atomically and records it
// in the history. The target may be free, an expired hold, or the user's own active hold;
// another user's active hold blocks it. Both rows are locked in id order, so concurrent
// reschedules and bookings touching the same slots cannot deadlock or double-book.
func (r *slotsRepo) Reschedule(ctx context.Context, fromSlotId, toSlotId, userId int) (models.Slot, error) {
	if fromSlotId == toSlotId {
		return models.Slot{}, ErrSameSlot
	}
	uid := int64(userId)

	var updated models.Slot
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked []models.Slot
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", []int{fromSlotId, toSlotId}).
			Order("id").
			Find(&locked).Error
		if err != nil {
			return err
		}
		if len(locked) != 2 {
			return ErrNotFound
		}
		from, to := locked[0], locked[1]
		if from.ID != int64(fromSlotId) {
			from, to = to, from
		}

		now := time.Now()
		currentFrom, currentTo := from, to
		currentFrom.ResolveHold(now, r.holdFor)
		currentTo.ResolveHold(now, r.holdFor)

		if !currentFrom.IsBooked() {
			return ErrSlotNotBooked
		}
		if !currentFrom.HeldOrBookedBy(uid) {
			return ErrSlotNotOwned
		}
		switch {
		case currentTo.IsBooked():
			return ErrSlotAlreadyBooked
		case currentTo.IsOnHold() && !currentTo.HeldOrBookedBy(uid):
			return ErrSlotOnHold
		}

		if err := guardedUpdate(tx, from, map[string]any{"status": models.SlotStatusNotBooked, "booked_by": nil}); err != nil {
			return err
		}
		if err := guardedUpdate(tx, to, map[string]any{"status": models.SlotStatusBooked, "booked_by": uid}); err != nil {
			return err
		}

		history := models.SlotHistory{
			UserID:         uid,
			SlotID:         to.ID,
			PreviousSlotID: &from.ID,
			Action:         models.SlotActionRescheduled,
		}
		if err := tx.Create(&history).Error; err != nil {
			return err
		}

		return tx.First(&updated, to.ID).Error
	})
	if err != nil {
		return models.Slot{}, err
	}
	return updated, nil
}

func requireUser(tx *gorm.DB, userId int) error {
	if err := tx.Select("id").First(&models.User{}, userId).Error; err != nil {
		return mapErr(err)
	}
	return nil
}

// guardedUpdate applies updates only if the row still has the status and booked_by we read,
// as a second guard in case the row lock isn't honoured (e.g. a driver that ignores FOR UPDATE).
func guardedUpdate(tx *gorm.DB, read models.Slot, updates map[string]any) error {
	res := tx.Model(&models.Slot{}).
		Where("id = ? AND status = ? AND booked_by IS NOT DISTINCT FROM ?", read.ID, read.Status, read.BookedBy).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrConcurrentUpdate
	}
	return nil
}

func (r *slotsRepo) AddSlot(ctx context.Context, slot models.Slot) (models.Slot, error) {
	if !slot.Status.IsValid() {
		return models.Slot{}, ErrInvalidStatus
	}
	if err := r.db.WithContext(ctx).Create(&slot).Error; err != nil {
		return models.Slot{}, err
	}
	return slot, nil
}

// AddBulk inserts all slots in a single transaction; either all are created or none.
func (r *slotsRepo) AddBulk(ctx context.Context, slots []models.Slot) ([]models.Slot, error) {
	if len(slots) == 0 {
		return slots, nil
	}
	for _, s := range slots {
		if !s.Status.IsValid() {
			return nil, ErrInvalidStatus
		}
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.CreateInBatches(&slots, bulkInsertBatchSize).Error
	})
	if err != nil {
		return nil, err
	}
	return slots, nil
}
