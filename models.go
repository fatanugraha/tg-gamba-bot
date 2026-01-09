package main

import "time"

type slotFace int

const (
	barSlotFace slotFace = iota
	cherrySlotFace
	lemonSlotFace
	sevenSlotFace
)

type slotMachineValue int

func (v slotMachineValue) left() slotFace {
	return slotFace((v - 1) & 3)
}

func (v slotMachineValue) center() slotFace {
	return slotFace(((v - 1) >> 2) & 3)
}

func (v slotMachineValue) right() slotFace {
	return slotFace(((v - 1) >> 4) & 3)
}

type stats struct {
	UserID       int64 `gorm:"primaryKey"`
	GroupID      int64 `gorm:"primaryKey"`
	Username     string
	BarWins      int64
	CherryWins   int64
	LemonWins    int64
	SevenWins    int64
	TotalGames   int64
	LastPlayedAt time.Time
}

func (u *stats) Score() int64 {
	return u.SevenWins*100 + u.BarWins*50 + u.LemonWins*20 + u.CherryWins*10
}
