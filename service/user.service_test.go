package service

import (
	"context"
	"errors"
	"slots/models"
	"testing"
	"time"
)

func TestUserCreateValidation(t *testing.T) {
	tests := []struct {
		name, userName, email string
	}{
		{"empty name", "", "a@b.com"},
		{"blank name", "   ", "a@b.com"},
		{"empty email", "Asha", ""},
		{"invalid email", "Asha", "not-an-email"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeUserRepo{}
			_, err := NewUserService(repo, &fakeHistoryRepo{}, testHold).Create(context.Background(), tt.userName, tt.email)

			var vErr *ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if repo.created != nil {
				t.Error("repo.Create called despite invalid input")
			}
		})
	}
}

func TestUserCreateNormalizesInput(t *testing.T) {
	repo := &fakeUserRepo{}
	user, err := NewUserService(repo, &fakeHistoryRepo{}, testHold).Create(context.Background(), "  Asha  ", "  Asha@Example.COM ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.created.Name != "Asha" || repo.created.Email != "asha@example.com" {
		t.Errorf("saved %+v, want trimmed name and lower-cased email", *repo.created)
	}
	if user.ID != 1 {
		t.Errorf("returned user id = %d, want 1", user.ID)
	}
}

func TestUserCreatePropagatesDuplicate(t *testing.T) {
	repo := &fakeUserRepo{createErr: ErrDuplicate}
	_, err := NewUserService(repo, &fakeHistoryRepo{}, testHold).Create(context.Background(), "Asha", "a@b.com")
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestUserGetByIdPropagatesNotFound(t *testing.T) {
	repo := &fakeUserRepo{getErr: ErrNotFound}
	_, err := NewUserService(repo, &fakeHistoryRepo{}, testHold).GetById(context.Background(), 5)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestHistory(t *testing.T) {
	t.Run("defaults limit and passes paging", func(t *testing.T) {
		history := &fakeHistoryRepo{}
		svc := NewUserService(&fakeUserRepo{}, history, testHold)

		if _, err := svc.History(context.Background(), 7, 0, 0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if history.gotUser != 7 || history.gotLimit != DefaultHistoryLimit || history.gotOffset != 0 {
			t.Errorf("repo got user=%d limit=%d offset=%d", history.gotUser, history.gotLimit, history.gotOffset)
		}

		if _, err := svc.History(context.Background(), 7, 10, 20); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if history.gotLimit != 10 || history.gotOffset != 20 {
			t.Errorf("repo got limit=%d offset=%d, want 10 and 20", history.gotLimit, history.gotOffset)
		}
	})

	for name, p := range map[string][2]int{
		"negative limit":  {-1, 0},
		"limit too large": {MaxHistoryLimit + 1, 0},
		"negative offset": {10, -1},
	} {
		t.Run(name, func(t *testing.T) {
			history := &fakeHistoryRepo{}
			_, err := NewUserService(&fakeUserRepo{}, history, testHold).History(context.Background(), 1, p[0], p[1])
			var vErr *ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if history.called {
				t.Error("repo called despite invalid paging")
			}
		})
	}

	t.Run("unknown user", func(t *testing.T) {
		history := &fakeHistoryRepo{}
		_, err := NewUserService(&fakeUserRepo{getErr: ErrNotFound}, history, testHold).History(context.Background(), 1, 0, 0)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		if history.called {
			t.Error("repo called for unknown user")
		}
	})
}

func TestHistoryShowsExpiredHoldsAsFree(t *testing.T) {
	user := int64(1)
	expired := &models.Slot{ID: 1, Status: models.SlotStatusOnHold, BookedBy: &user, UpdatedAt: time.Now().Add(-time.Hour)}
	active := &models.Slot{ID: 2, Status: models.SlotStatusOnHold, BookedBy: &user, UpdatedAt: time.Now()}
	history := &fakeHistoryRepo{entries: []models.SlotHistory{
		{ID: 2, SlotID: 2, Action: models.SlotActionHeld, Slot: active},
		{ID: 1, SlotID: 1, Action: models.SlotActionHeld, Slot: expired, PreviousSlot: expired},
	}}
	got, err := NewUserService(&fakeUserRepo{}, history, testHold).History(context.Background(), 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Slot.Status != models.SlotStatusOnHold || got[0].Slot.HoldExpiresAt == nil {
		t.Errorf("active hold in history = %+v", *got[0].Slot)
	}
	if got[1].Slot.Status != models.SlotStatusNotBooked || got[1].PreviousSlot.Status != models.SlotStatusNotBooked {
		t.Errorf("expired hold in history = %+v", *got[1].Slot)
	}
}
