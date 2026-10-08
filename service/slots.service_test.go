package service

import (
	"context"
	"errors"
	"slots/models"
	"strings"
	"testing"
	"time"
)

var (
	day    = time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	ten    = time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	eleven = time.Date(2026, 10, 10, 11, 0, 0, 0, time.UTC)
)

const testHold = 10 * time.Minute

func validSlot() models.Slot {
	return models.Slot{Date: day, From: ten, To: eleven}
}

func TestAddSlotValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*models.Slot)
	}{
		{"missing date", func(s *models.Slot) { s.Date = time.Time{} }},
		{"missing from", func(s *models.Slot) { s.From = time.Time{} }},
		{"missing to", func(s *models.Slot) { s.To = time.Time{} }},
		{"from after to", func(s *models.Slot) { s.From, s.To = eleven, ten }},
		{"from equals to", func(s *models.Slot) { s.To = s.From }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot := validSlot()
			tt.mutate(&slot)
			repo := &fakeSlotsRepo{}
			_, err := NewSlotsService(repo, testHold).AddSlot(context.Background(), slot)

			var vErr *ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if repo.added != nil {
				t.Error("repo.AddSlot called despite invalid input")
			}
		})
	}
}

func TestAddSlotResetsClientControlledFields(t *testing.T) {
	userId := int64(7)
	slot := validSlot()
	slot.ID = 99
	slot.Status = models.SlotStatusBooked
	slot.BookedBy = &userId
	slot.User = &models.User{ID: 7}

	repo := &fakeSlotsRepo{}
	if _, err := NewSlotsService(repo, testHold).AddSlot(context.Background(), slot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := repo.added
	if got.ID != 0 || got.Status != models.SlotStatusNotBooked || got.BookedBy != nil || got.User != nil {
		t.Errorf("new slot not reset to unbooked: %+v", *got)
	}
}

func TestAddBulk(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		repo := &fakeSlotsRepo{}
		_, err := NewSlotsService(repo, testHold).AddBulk(context.Background(), nil)
		var vErr *ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("err = %v, want ValidationError", err)
		}
	})

	t.Run("one invalid item rejects all and names the index", func(t *testing.T) {
		bad := validSlot()
		bad.From, bad.To = eleven, ten
		repo := &fakeSlotsRepo{}
		_, err := NewSlotsService(repo, testHold).AddBulk(context.Background(), []models.Slot{validSlot(), bad})

		var vErr *ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("err = %v, want ValidationError", err)
		}
		if !strings.HasPrefix(vErr.Msg, "slots[1]:") {
			t.Errorf("message %q should start with slots[1]:", vErr.Msg)
		}
		if repo.addedBulk != nil {
			t.Error("repo.AddBulk called despite invalid item")
		}
	})

	t.Run("valid items are reset and saved", func(t *testing.T) {
		s := validSlot()
		s.Status = models.SlotStatusBooked
		repo := &fakeSlotsRepo{}
		got, err := NewSlotsService(repo, testHold).AddBulk(context.Background(), []models.Slot{validSlot(), s})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || len(repo.addedBulk) != 2 {
			t.Fatalf("got %d slots, repo got %d, want 2", len(got), len(repo.addedBulk))
		}
		for i, slot := range repo.addedBulk {
			if slot.Status != models.SlotStatusNotBooked {
				t.Errorf("slot %d status = %d, want not booked", i, slot.Status)
			}
		}
	})
}

func TestBookAndCancelCallRepoWithStatus(t *testing.T) {
	tests := []struct {
		name   string
		call   func(SlotsService) (models.Slot, error)
		status models.SlotStatus
	}{
		{"hold", func(s SlotsService) (models.Slot, error) { return s.Hold(context.Background(), 3, 4) }, models.SlotStatusOnHold},
		{"book", func(s SlotsService) (models.Slot, error) { return s.Book(context.Background(), 3, 4) }, models.SlotStatusBooked},
		{"cancel", func(s SlotsService) (models.Slot, error) { return s.Cancel(context.Background(), 3, 4) }, models.SlotStatusNotBooked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeSlotsRepo{}
			if _, err := tt.call(NewSlotsService(repo, testHold)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := updateCall{slotId: 3, userId: 4, status: tt.status}
			if repo.updateCall == nil || *repo.updateCall != want {
				t.Errorf("repo called with %+v, want %+v", repo.updateCall, want)
			}
		})
	}
}

func TestBookPropagatesRepoErrors(t *testing.T) {
	for _, want := range []error{ErrSlotAlreadyBooked, ErrNotFound, ErrConcurrentUpdate} {
		repo := &fakeSlotsRepo{err: want}
		_, err := NewSlotsService(repo, testHold).Book(context.Background(), 1, 1)
		if !errors.Is(err, want) {
			t.Errorf("err = %v, want %v", err, want)
		}
	}
}

func TestReschedule(t *testing.T) {
	t.Run("same slot", func(t *testing.T) {
		repo := &fakeSlotsRepo{}
		_, err := NewSlotsService(repo, testHold).Reschedule(context.Background(), 2, 2, 1)
		if !errors.Is(err, ErrSameSlot) {
			t.Errorf("err = %v, want ErrSameSlot", err)
		}
		if repo.reschedule != nil {
			t.Error("repo called for same-slot reschedule")
		}
	})

	t.Run("passes ids", func(t *testing.T) {
		repo := &fakeSlotsRepo{}
		if _, err := NewSlotsService(repo, testHold).Reschedule(context.Background(), 2, 3, 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.reschedule == nil || *repo.reschedule != [3]int{2, 3, 1} {
			t.Errorf("repo got %v, want [2 3 1]", repo.reschedule)
		}
	})

	t.Run("propagates repo error", func(t *testing.T) {
		repo := &fakeSlotsRepo{err: ErrSlotAlreadyBooked}
		if _, err := NewSlotsService(repo, testHold).Reschedule(context.Background(), 2, 3, 1); !errors.Is(err, ErrSlotAlreadyBooked) {
			t.Errorf("err = %v, want ErrSlotAlreadyBooked", err)
		}
	})
}

func TestSlotsShowExpiredHoldsAsFree(t *testing.T) {
	user := int64(5)
	now := time.Now()
	stored := []models.Slot{
		{ID: 1, Status: models.SlotStatusOnHold, BookedBy: &user, UpdatedAt: now.Add(-testHold - time.Second)},
		{ID: 2, Status: models.SlotStatusOnHold, BookedBy: &user, UpdatedAt: now.Add(-time.Minute)},
		{ID: 3, Status: models.SlotStatusBooked, BookedBy: &user, UpdatedAt: now.Add(-24 * time.Hour)},
		{ID: 4, Status: models.SlotStatusNotBooked, UpdatedAt: now.Add(-24 * time.Hour)},
	}
	repo := &fakeSlotsRepo{stored: stored}
	got, err := NewSlotsService(repo, testHold).GetAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if got[0].Status != models.SlotStatusNotBooked || got[0].BookedBy != nil || got[0].HoldExpiresAt != nil {
		t.Errorf("expired hold shown as %+v, want not booked", got[0])
	}
	if got[1].Status != models.SlotStatusOnHold || got[1].HoldExpiresAt == nil ||
		!got[1].HoldExpiresAt.Equal(stored[1].UpdatedAt.Add(testHold)) {
		t.Errorf("active hold shown as %+v, want on hold with expiry", got[1])
	}
	if got[2].Status != models.SlotStatusBooked || got[3].Status != models.SlotStatusNotBooked {
		t.Errorf("booked/free slots changed: %+v %+v", got[2], got[3])
	}

	one, _ := NewSlotsService(&fakeSlotsRepo{stored: stored[:1]}, testHold).GetById(context.Background(), 1)
	if one.Status != models.SlotStatusNotBooked {
		t.Errorf("GetById expired hold status = %d, want 0", one.Status)
	}
}

func TestHoldReturnsExpiry(t *testing.T) {
	user := int64(5)
	updatedAt := time.Now()
	repo := &fakeSlotsRepo{updated: &models.Slot{ID: 1, Status: models.SlotStatusOnHold, BookedBy: &user, UpdatedAt: updatedAt}}
	got, err := NewSlotsService(repo, testHold).Hold(context.Background(), 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got.HoldExpiresAt == nil || !got.HoldExpiresAt.Equal(updatedAt.Add(testHold)) {
		t.Errorf("HoldExpiresAt = %v, want %v", got.HoldExpiresAt, updatedAt.Add(testHold))
	}
}
