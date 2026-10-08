package service

import (
	"context"
	"slots/models"
)

type fakeUserRepo struct {
	getUser   models.User
	getErr    error
	created   *models.User
	createErr error
}

func (f *fakeUserRepo) GetById(ctx context.Context, id int) (models.User, error) {
	return f.getUser, f.getErr
}

func (f *fakeUserRepo) Create(ctx context.Context, user models.User) (models.User, error) {
	f.created = &user
	if f.createErr != nil {
		return models.User{}, f.createErr
	}
	user.ID = 1
	return user, nil
}

type fakeHistoryRepo struct {
	entries                      []models.SlotHistory // returned when set
	err                          error
	called                       bool
	gotUser, gotLimit, gotOffset int
}

func (f *fakeHistoryRepo) ListByUser(ctx context.Context, userId, limit, offset int) ([]models.SlotHistory, error) {
	f.called, f.gotUser, f.gotLimit, f.gotOffset = true, userId, limit, offset
	if f.entries != nil {
		return f.entries, f.err
	}
	return []models.SlotHistory{{UserID: int64(userId), Action: models.SlotActionBooked}}, f.err
}

type updateCall struct {
	slotId, userId int
	status         models.SlotStatus
}

type fakeSlotsRepo struct {
	err        error
	added      *models.Slot
	addedBulk  []models.Slot
	updateCall *updateCall
	reschedule *[3]int       // from, to, user
	stored     []models.Slot // returned by GetAll / GetById when set
	updated    *models.Slot  // returned by UpdateSlotStatus when set
}

func (f *fakeSlotsRepo) Reschedule(ctx context.Context, fromSlotId, toSlotId, userId int) (models.Slot, error) {
	f.reschedule = &[3]int{fromSlotId, toSlotId, userId}
	return models.Slot{ID: int64(toSlotId), Status: models.SlotStatusBooked}, f.err
}

func (f *fakeSlotsRepo) GetById(ctx context.Context, id int) (models.Slot, error) {
	if f.stored != nil {
		return f.stored[0], f.err
	}
	return models.Slot{ID: int64(id)}, f.err
}

func (f *fakeSlotsRepo) GetAll(ctx context.Context) ([]models.Slot, error) {
	if f.stored != nil {
		return f.stored, f.err
	}
	return []models.Slot{}, f.err
}

func (f *fakeSlotsRepo) UpdateSlotStatus(ctx context.Context, slotId int, userId int, status models.SlotStatus) (models.Slot, error) {
	f.updateCall = &updateCall{slotId, userId, status}
	if f.err != nil {
		return models.Slot{}, f.err
	}
	if f.updated != nil {
		return *f.updated, nil
	}
	return models.Slot{ID: int64(slotId), Status: status}, nil
}

func (f *fakeSlotsRepo) AddSlot(ctx context.Context, slot models.Slot) (models.Slot, error) {
	f.added = &slot
	return slot, f.err
}

func (f *fakeSlotsRepo) AddBulk(ctx context.Context, slots []models.Slot) ([]models.Slot, error) {
	f.addedBulk = slots
	return slots, f.err
}
