package models

import "time"

type SlotStatus int

const (
	SlotStatusNotBooked SlotStatus = 0
	SlotStatusOnHold    SlotStatus = 50
	SlotStatusBooked    SlotStatus = 100
)

func (s SlotStatus) String() string {
	switch s {
	case SlotStatusNotBooked:
		return "not_booked"
	case SlotStatusOnHold:
		return "on_hold"
	case SlotStatusBooked:
		return "booked"
	default:
		return "unknown"
	}
}

func (s SlotStatus) IsValid() bool {
	return s == SlotStatusNotBooked || s == SlotStatusOnHold || s == SlotStatusBooked
}

type Slot struct {
	ID        int64      `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Date      time.Time  `json:"date" gorm:"column:date;type:date;not null;index"`
	From      time.Time  `json:"from" gorm:"column:from;not null"`
	To        time.Time  `json:"to" gorm:"column:to;not null"`
	Status    SlotStatus `json:"status" gorm:"column:status;type:smallint;not null;default:0;index"`
	BookedBy  *int64     `json:"bookedBy,omitempty" gorm:"column:booked_by;index"` // User.ID of the booker or holder; nil when not booked
	CreatedAt time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`

	// HoldExpiresAt is computed by ResolveHold for an active hold; it is not stored.
	HoldExpiresAt *time.Time `json:"holdExpiresAt,omitempty" gorm:"-"`

	User *User `json:"user,omitempty" gorm:"foreignKey:BookedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
}

func (Slot) TableName() string {
	return "slots"
}

func (s *Slot) IsBooked() bool {
	return s.Status == SlotStatusBooked
}

func (s *Slot) IsOnHold() bool {
	return s.Status == SlotStatusOnHold
}

// HeldOrBookedBy reports whether userId is the slot's booker or holder.
func (s *Slot) HeldOrBookedBy(userId int64) bool {
	return s.BookedBy != nil && *s.BookedBy == userId
}

// ResolveHold applies hold expiry in memory only. A hold lasts holdFor from the slot's last
// update: once that has passed the slot reads as not booked; while it is active,
// HoldExpiresAt is set. Slots that are not on hold are left unchanged.
func (s *Slot) ResolveHold(now time.Time, holdFor time.Duration) {
	if s.Status != SlotStatusOnHold {
		s.HoldExpiresAt = nil
		return
	}
	expires := s.UpdatedAt.Add(holdFor)
	if !now.Before(expires) {
		s.Status = SlotStatusNotBooked
		s.BookedBy = nil
		s.HoldExpiresAt = nil
		return
	}
	s.HoldExpiresAt = &expires
}
