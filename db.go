package main

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB

func initDB() error {
	var err error
	db, err = gorm.Open(sqlite.Open("casino.db"), &gorm.Config{})
	if err != nil {
		return err
	}
	return db.AutoMigrate(&stats{})
}

func getOrCreateStats(userID, groupID int64, username string) (*stats, error) {
	var u stats
	result := db.Where("user_id = ? AND group_id = ?", userID, groupID).First(&u)
	if result.Error == gorm.ErrRecordNotFound {
		u = stats{UserID: userID, GroupID: groupID, Username: username}
		if err := db.Create(&u).Error; err != nil {
			return nil, err
		}
		return &u, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &u, nil
}

func saveStats(u *stats) error {
	return db.Save(u).Error
}

func getUsersByGroup(groupID int64) ([]stats, error) {
	var results []stats
	err := db.Where("group_id = ?", groupID).Find(&results).Error
	return results, err
}
