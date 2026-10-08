package repo

import (
	"context"
	"slots/models"

	"gorm.io/gorm"
)

type HistoryRepo interface {
	// ListByUser returns the user's history, newest first, with Slot and PreviousSlot loaded.
	ListByUser(ctx context.Context, userId, limit, offset int) ([]models.SlotHistory, error)
}

type historyRepo struct {
	db *gorm.DB
}

func NewHistoryRepo(db *gorm.DB) HistoryRepo {
	return &historyRepo{db: db}
}

func (r *historyRepo) ListByUser(ctx context.Context, userId, limit, offset int) ([]models.SlotHistory, error) {
	var history []models.SlotHistory
	err := r.db.WithContext(ctx).
		Preload("Slot").
		Preload("PreviousSlot").
		Where("user_id = ?", userId).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&history).Error
	if err != nil {
		return nil, err
	}
	return history, nil
}
