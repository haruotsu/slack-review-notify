package main

import (
	"log"
	"slack-review-notify/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	db, err := gorm.Open(sqlite.Open("review_tasks.db"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	err = db.AutoMigrate(&models.ReviewTask{})
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Database migrated")

}
