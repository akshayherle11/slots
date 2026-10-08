package main

import (
	"fmt"
	"log"
	"slots/config"
	"slots/models"
	"slots/repo"
	"slots/service"
	"slots/transport"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL()), &gorm.Config{TranslateError: true})
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Slot{}, &models.SlotHistory{}); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	holdFor := cfg.Booking.HoldDuration()
	userHandler := transport.NewUserHandler(service.NewUserService(repo.NewUserRepo(db), repo.NewHistoryRepo(db), holdFor))
	slotsHandler := transport.NewSlotsHandler(service.NewSlotsService(repo.NewSlotsRepo(db, holdFor), holdFor))
	router := transport.NewRouter(userHandler, slotsHandler)

	if err := router.Run(fmt.Sprintf(":%d", cfg.Server.Port)); err != nil {
		log.Fatalf("server: %v", err)
	}
}
