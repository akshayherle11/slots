package models

import "time"

type SlotAction int

const (
	SlotActionBooked      SlotAction = 1
	SlotActionUnbooked    SlotAction = 2
	SlotActionRescheduled SlotAction = 3
	SlotActionHeld        SlotAction = 4
)

func (a SlotAction) String() string {
	switch a {
	case SlotActionBooked:
		return "booked"
	case SlotActionUnbooked:
		return "unbooked"
	case SlotActionRescheduled:
		return "rescheduled"
	case SlotActionHeld:
		return "held"
	default:
		return "unknown"
	}
}

func (a SlotAction) IsValid() bool {
	return a == SlotActionBooked || a == SlotActionUnbooked || a == SlotActionRescheduled || a == SlotActionHeld
}

// SlotHistory records one booking change made by a user. For a reschedule, SlotID is the
// slot moved to and PreviousSlotID the slot moved from; for other actions PreviousSlotID is nil.
type SlotHistory struct {
	ID             int64      `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int64      `json:"userId" gorm:"column:user_id;not null;index:idx_slot_history_user_created,priority:1"`
	SlotID         int64      `json:"slotId" gorm:"column:slot_id;not null;index"`
	PreviousSlotID *int64     `json:"previousSlotId,omitempty" gorm:"column:previous_slot_id;index"`
	Action         SlotAction `json:"action" gorm:"column:action;type:smallint;not null"`
	CreatedAt      time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime;index:idx_slot_history_user_created,priority:2"`

	User         *User `json:"-" gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Slot         *Slot `json:"slot,omitempty" gorm:"foreignKey:SlotID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	PreviousSlot *Slot `json:"previousSlot,omitempty" gorm:"foreignKey:PreviousSlotID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
}

func (SlotHistory) TableName() string {
	return "slot_history"
}
