package main

import (
	"os"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type DB struct {
	*gorm.DB
}

type SlotMachineStats struct {
	UserID       int64     `gorm:"primaryKey"`
	GroupID      int64     `gorm:"primaryKey"`
	Username     string
	BarWins      int64
	CherryWins   int64
	LemonWins    int64
	SevenWins    int64
	TotalGames   int64
	Score        int64
	LastPlayedAt time.Time
}

type Balance struct {
	UserID  int64 `gorm:"primaryKey"`
	GroupID int64 `gorm:"primaryKey"`
	Amount  int64
}

func OpenDB() (*DB, error) {
	if err := os.MkdirAll("data", 0755); err != nil {
		return nil, err
	}
	gormDB, err := gorm.Open(sqlite.Open("data/casino.db"), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := gormDB.AutoMigrate(&SlotMachineStats{}, &Balance{}); err != nil {
		return nil, err
	}
	return &DB{gormDB}, nil
}

func (db *DB) GetOrCreateStats(userID, groupID int64, username string) (*SlotMachineStats, error) {
	var u SlotMachineStats
	result := db.Where("user_id = ? AND group_id = ?", userID, groupID).First(&u)
	if result.Error == gorm.ErrRecordNotFound {
		u = SlotMachineStats{UserID: userID, GroupID: groupID, Username: username}
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

type StatsDelta struct {
	TotalGames int
	Score      int
	SevenWins  int
	BarWins    int
	CherryWins int
	LemonWins  int
}

func (db *DB) GetOrCreateBalance(userID, groupID int64) (*Balance, error) {
	var b Balance
	result := db.Where("user_id = ? AND group_id = ?", userID, groupID).First(&b)
	if result.Error == gorm.ErrRecordNotFound {
		b = Balance{UserID: userID, GroupID: groupID, Amount: 0}
		if err := db.Create(&b).Error; err != nil {
			return nil, err
		}
		return &b, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &b, nil
}

func (db *DB) UpdateBalance(tx *gorm.DB, userID, groupID int64, amountDelta int) error {
	return tx.Model(&Balance{}).
		Where("user_id = ? AND group_id = ?", userID, groupID).
		Update("amount", gorm.Expr("amount + ?", amountDelta)).Error
}

func (db *DB) UpdateStats(tx *gorm.DB, userID, groupID int64, lastPlayedAt time.Time, delta StatsDelta) error {
	return tx.Model(&SlotMachineStats{}).
		Where("user_id = ? AND group_id = ?", userID, groupID).
		Updates(map[string]interface{}{
			"total_games":    gorm.Expr("total_games + ?", delta.TotalGames),
			"score":          gorm.Expr("score + ?", delta.Score),
			"seven_wins":     gorm.Expr("seven_wins + ?", delta.SevenWins),
			"bar_wins":       gorm.Expr("bar_wins + ?", delta.BarWins),
			"cherry_wins":    gorm.Expr("cherry_wins + ?", delta.CherryWins),
			"lemon_wins":     gorm.Expr("lemon_wins + ?", delta.LemonWins),
			"last_played_at": lastPlayedAt,
		}).Error
}

func (db *DB) GetStatsByGroup(groupID int64) ([]SlotMachineStats, error) {
	var results []SlotMachineStats
	err := db.Where("group_id = ?", groupID).Find(&results).Error
	return results, err
}

func (db *DB) GetBalancesByGroup(groupID int64) ([]Balance, error) {
	var results []Balance
	err := db.Where("group_id = ?", groupID).Find(&results).Error
	return results, err
}
