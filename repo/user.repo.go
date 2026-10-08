package repo

import (
	"context"
	"slots/models"

	"gorm.io/gorm"
)

type UserRepo interface {
	GetById(ctx context.Context, id int) (models.User, error)
	Create(ctx context.Context, user models.User) (models.User, error)
}

type userRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) UserRepo {
	return &userRepo{db: db}
}

func (r *userRepo) GetById(ctx context.Context, id int) (models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return models.User{}, mapErr(err)
	}
	return user, nil
}

func (r *userRepo) Create(ctx context.Context, user models.User) (models.User, error) {
	if err := r.db.WithContext(ctx).Create(&user).Error; err != nil {
		return models.User{}, mapErr(err)
	}
	return user, nil
}
