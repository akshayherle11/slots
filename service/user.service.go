package service

import (
	"context"
	"net/mail"
	"slots/models"
	"slots/repo"
	"strings"
	"time"
)

const (
	DefaultHistoryLimit = 50
	MaxHistoryLimit     = 200
)

type UserService interface {
	GetById(ctx context.Context, id int) (models.User, error)
	Create(ctx context.Context, name, email string) (models.User, error)
	History(ctx context.Context, userId, limit, offset int) ([]models.SlotHistory, error)
}

type userService struct {
	users   repo.UserRepo
	history repo.HistoryRepo
	holdFor time.Duration
	now     func() time.Time
}

// NewUserService returns a UserService; holdFor is used to show expired holds in history as not booked.
func NewUserService(users repo.UserRepo, history repo.HistoryRepo, holdFor time.Duration) UserService {
	return &userService{users: users, history: history, holdFor: holdFor, now: time.Now}
}

func (s *userService) GetById(ctx context.Context, id int) (models.User, error) {
	return s.users.GetById(ctx, id)
}

func (s *userService) Create(ctx context.Context, name, email string) (models.User, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	if name == "" {
		return models.User{}, invalid("name is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return models.User{}, invalid("email is invalid")
	}
	return s.users.Create(ctx, models.User{Name: name, Email: email})
}

// History returns the user's booking history, newest first. A limit of 0 means DefaultHistoryLimit.
func (s *userService) History(ctx context.Context, userId, limit, offset int) ([]models.SlotHistory, error) {
	if limit == 0 {
		limit = DefaultHistoryLimit
	}
	if limit < 0 || limit > MaxHistoryLimit {
		return nil, invalid("limit must be between 1 and 200")
	}
	if offset < 0 {
		return nil, invalid("offset must not be negative")
	}
	if _, err := s.users.GetById(ctx, userId); err != nil {
		return nil, err
	}
	history, err := s.history.ListByUser(ctx, userId, limit, offset)
	if err != nil {
		return nil, err
	}
	now := s.now()
	for i := range history {
		if history[i].Slot != nil {
			history[i].Slot.ResolveHold(now, s.holdFor)
		}
		if history[i].PreviousSlot != nil {
			history[i].PreviousSlot.ResolveHold(now, s.holdFor)
		}
	}
	return history, nil
}
