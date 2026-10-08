package models

import (
	"testing"
	"time"
)

func TestSlotStatus(t *testing.T) {
	tests := []struct {
		status SlotStatus
		valid  bool
		str    string
	}{
		{SlotStatusNotBooked, true, "not_booked"},
		{SlotStatusOnHold, true, "on_hold"},
		{SlotStatusBooked, true, "booked"},
		{SlotStatus(1), false, "unknown"},
		{SlotStatus(-1), false, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.IsValid(); got != tt.valid {
			t.Errorf("SlotStatus(%d).IsValid() = %v, want %v", tt.status, got, tt.valid)
		}
		if got := tt.status.String(); got != tt.str {
			t.Errorf("SlotStatus(%d).String() = %q, want %q", tt.status, got, tt.str)
		}
	}
}

func TestSlotIsBooked(t *testing.T) {
	if (&Slot{Status: SlotStatusNotBooked}).IsBooked() {
		t.Error("not-booked slot reported as booked")
	}
	if !(&Slot{Status: SlotStatusBooked}).IsBooked() {
		t.Error("booked slot reported as not booked")
	}
}

func TestResolveHold(t *testing.T) {
	const holdFor = 10 * time.Minute
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	user := int64(7)

	t.Run("active hold keeps status and sets expiry", func(t *testing.T) {
		s := Slot{Status: SlotStatusOnHold, BookedBy: &user, UpdatedAt: now.Add(-9 * time.Minute)}
		s.ResolveHold(now, holdFor)
		if s.Status != SlotStatusOnHold || s.BookedBy == nil || s.HoldExpiresAt == nil {
			t.Fatalf("active hold changed: %+v", s)
		}
		if want := now.Add(time.Minute); !s.HoldExpiresAt.Equal(want) {
			t.Errorf("HoldExpiresAt = %v, want %v", s.HoldExpiresAt, want)
		}
	})

	for name, age := range map[string]time.Duration{"exactly expired": holdFor, "long expired": 3 * time.Hour} {
		t.Run(name, func(t *testing.T) {
			s := Slot{Status: SlotStatusOnHold, BookedBy: &user, UpdatedAt: now.Add(-age)}
			s.ResolveHold(now, holdFor)
			if s.Status != SlotStatusNotBooked || s.BookedBy != nil || s.HoldExpiresAt != nil {
				t.Errorf("expired hold not freed: %+v", s)
			}
		})
	}

	t.Run("booked and free slots untouched", func(t *testing.T) {
		old := now.Add(-24 * time.Hour)
		booked := Slot{Status: SlotStatusBooked, BookedBy: &user, UpdatedAt: old}
		booked.ResolveHold(now, holdFor)
		if booked.Status != SlotStatusBooked || booked.BookedBy == nil || booked.HoldExpiresAt != nil {
			t.Errorf("booked slot changed: %+v", booked)
		}
		free := Slot{Status: SlotStatusNotBooked, UpdatedAt: old}
		free.ResolveHold(now, holdFor)
		if free.Status != SlotStatusNotBooked || free.HoldExpiresAt != nil {
			t.Errorf("free slot changed: %+v", free)
		}
	})
}

func TestHeldOrBookedBy(t *testing.T) {
	u := int64(3)
	if (&Slot{}).HeldOrBookedBy(3) {
		t.Error("slot with no bookedBy matched")
	}
	if !(&Slot{BookedBy: &u}).HeldOrBookedBy(3) || (&Slot{BookedBy: &u}).HeldOrBookedBy(4) {
		t.Error("HeldOrBookedBy compared users wrongly")
	}
}
