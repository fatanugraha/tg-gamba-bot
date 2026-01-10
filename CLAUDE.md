# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- `go build` - Build the project
- `go run .` - Run the bot (requires `BOT_TOKEN` and `BOT_USERNAME` environment variables)

## Architecture

This is a Telegram slot machine casino bot that tracks user statistics and balances in a SQLite database.

### Core Components

**main.go** - Bot entry point and handlers
- `casinoController` holds database dependency and handles all Telegram bot interactions
- Handlers: `/stats` (shows win stats), `/balance` (shows balances), and default dice handler for slot machine emoji
- All stat/balance updates happen in transactions using `db.Transaction()` with defer to ensure atomic writes

**db.go** - Database layer (GORM + SQLite)
- `DB` struct wraps `*gorm.DB` for dependency injection (no globals)
- `SlotMachineStats` table: tracks wins per symbol (7️⃣/🍫/🍒/🍋), total games, score, last played time
- `Balance` table: tracks user balance amount separately (score written to both tables for double-write)
- All update methods accept `tx *gorm.DB` parameter - caller manages transactions
- `UpdateStats()` and `UpdateBalance()` use atomic SQL expressions (`gorm.Expr("field + ?", delta)`)

**models.go** - Slot machine decoding logic
- `slotFace` enum: bar, cherry, lemon, seven (iota order matters for bit encoding)
- `slotMachineValue` encoding: Telegram dice value (1-64) encodes 3 slot faces via bit positions
- `left()`, `center()`, `right()` methods decode the faces

### Key Patterns

1. **Double-write**: Score is written to both `SlotMachineStats.Score` and `Balance.Amount` in a single transaction
2. **Atomic updates**: Always use `gorm.Expr("field + ?", delta)` for concurrent-safe increments
3. **Transaction caller-managed**: DB methods accept `*gorm.DB` tx parameter, caller wraps in `db.Transaction()`
4. **No globals**: Database passed through controller struct, not package-level variables

### Slot Machine Scoring

When all 3 faces match:
- 7️⃣ (seven): +100 score
- 🍫 (bar): +50 score
- 🍋 (lemon): +20 score
- 🍒 (cherry): +10 score

Score delta is applied to both stats and balance atomically.
