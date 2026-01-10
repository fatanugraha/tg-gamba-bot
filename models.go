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
	return slotFace((v - 1) & 3)
}

func (v slotMachineValue) center() slotFace {
	return slotFace(((v - 1) >> 2) & 3)
}

func (v slotMachineValue) right() slotFace {
	return slotFace(((v - 1) >> 4) & 3)
}

