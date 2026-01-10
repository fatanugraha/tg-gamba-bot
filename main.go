package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"gorm.io/gorm"
)

func main() {
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Panic("BOT_TOKEN environment variable is not set")
	}

	botUsername := os.Getenv("BOT_USERNAME")
	if botUsername == "" {
		log.Panic("BOT_USERNAME environment variable is not set")
	}

	db, err := OpenDB()
	if err != nil {
		log.Panic(err)
	}

	svc := newCasinoController(botToken, botUsername, db)

	tgBot, err := bot.New(
		botToken,
		bot.WithMessageTextHandler("/stats", bot.MatchTypeExact, svc.statsHandler),
		bot.WithMessageTextHandler("/balance", bot.MatchTypeExact, svc.balanceHandler),
		bot.WithDefaultHandler(svc.diceHandler),
		bot.WithWorkers(1),
	)
	if err != nil {
		log.Panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tgBot.Start(ctx)
}

type casinoController struct {
	token    string
	username string
	db       *DB
}

func newCasinoController(token string, username string, db *DB) *casinoController {
	return &casinoController{
		token:    token,
		username: username,
		db:       db,
	}
}

func (c *casinoController) statsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	stats, err := c.db.GetStatsByGroup(update.Message.Chat.ID)
	if err != nil {
		log.Printf("error getting users: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Error getting stats.",
		})
		return
	}

	if len(stats) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "No stats yet.",
		})
		return
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Score != stats[j].Score {
			return stats[i].Score > stats[j].Score
		}
		if stats[i].TotalGames != stats[j].TotalGames {
			return stats[i].TotalGames < stats[j].TotalGames
		}
		return stats[i].LastPlayedAt.After(stats[j].LastPlayedAt)
	})

	var msg string
	for i := 0; i < len(stats); i++ {
		u := stats[i]
		name := u.Username
		if name == "" {
			name = fmt.Sprintf("User_%d", u.UserID)
		}
		msg += fmt.Sprintf("%d. %s - %d pts (7️⃣:%d 🍫:%d 🍒:%d 🍋:%d 🎰:%d)\n",
			i+1, name, u.Score, u.SevenWins, u.BarWins, u.CherryWins, u.LemonWins, u.TotalGames)
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   msg,
	})
}

func (c *casinoController) balanceHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	balances, err := c.db.GetBalancesByGroup(update.Message.Chat.ID)
	if err != nil {
		log.Printf("error getting balances: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Error getting balances.",
		})
		return
	}

	if len(balances) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "No balances yet.",
		})
		return
	}

	sort.Slice(balances, func(i, j int) bool {
		return balances[i].Amount > balances[j].Amount
	})

	var msg string
	for i := 0; i < len(balances); i++ {
		bal := balances[i]
		stats, err := c.db.GetOrCreateStats(bal.UserID, bal.GroupID, "")
		if err != nil {
			continue
		}
		name := stats.Username
		if name == "" {
			name = fmt.Sprintf("User_%d", bal.UserID)
		}
		msg += fmt.Sprintf("%d. %s - %d$\n", i+1, name, bal.Amount)
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   msg,
	})
}

func (c *casinoController) diceHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	v, ok := c.parseSlotMachineMessage(update)
	if !ok {
		return
	}

	userID := update.Message.From.ID
	username := update.Message.From.Username
	groupID := update.Message.Chat.ID

	if _, err := c.db.GetOrCreateStats(userID, groupID, username); err != nil {
		log.Printf("error getting user: %v", err)
		return
	}
	if _, err := c.db.GetOrCreateBalance(userID, groupID); err != nil {
		log.Printf("error getting balance: %v", err)
		return
	}

	delta := StatsDelta{TotalGames: 1, Score: 0}
	lastPlayedAt := time.Unix(int64(update.Message.Date), 0)
	defer func() {
		if err := c.db.Transaction(func(tx *gorm.DB) error {
			if err := c.db.UpdateStats(tx, userID, groupID, lastPlayedAt, delta); err != nil {
				return err
			}
			if err := c.db.UpdateBalance(tx, userID, groupID, delta.Score); err != nil {
				return err
			}
			return nil
		}); err != nil {
			log.Printf("error saving: %v", err)
		}
	}()

	left, center, right := v.left(), v.center(), v.right()
	if left != center || center != right {
		return
	}

	switch left {
	case barSlotFace:
		delta.BarWins = 1
		delta.Score = 50
	case cherrySlotFace:
		delta.CherryWins = 1
		delta.Score = 10
	case lemonSlotFace:
		delta.LemonWins = 1
		delta.Score = 20
	case sevenSlotFace:
		delta.SevenWins = 1
		delta.Score = 100
	default:
		log.Printf("unexpected main.slotFace: %#v", left)
	}
}

func (*casinoController) parseSlotMachineMessage(update *models.Update) (slotMachineValue, bool) {
	msg := update.Message
	if msg == nil {
		return 0, false
	}

	dice := msg.Dice
	if dice == nil {
		return 0, false
	}

	if dice.Emoji != "🎰" {
		return 0, false
	}

	return slotMachineValue(dice.Value), true
}
