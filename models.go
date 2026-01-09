package main

type slotFace int

const (
	barSlotFace slotFace = iota
	cherrySlotFace
	lemonSlotFace
	sevenSlotFace
)

type slotMachineValue int

func (v slotMachineValue) left() slotFace {
	return slotFace(v & 3)
}

func (v slotMachineValue) center() slotFace {
	return slotFace((v >> 2) & 3)
}

func (v slotMachineValue) right() slotFace {
	return slotFace((v >> 4) & 3)
}

type userStats struct {
	UserID         int64
	Username       string
	BarWins        int
	CherryWins     int
	LemonWins      int
	SevenWins      int
	TotalGames     int
	LastPlayedAt   int64
}

func (u *userStats) Score() int {
	return u.SevenWins*100 + u.BarWins*50 + u.LemonWins*20 + u.CherryWins*10
}
