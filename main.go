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
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Panic("TELEGRAM_BOT_TOKEN environment variable is not set")
	}

	if err := initDB(); err != nil {
		log.Panic(err)
	}

	b, err := bot.New(
		token,
		bot.WithMessageTextHandler("/stats", bot.MatchTypeExact, statsHandler),
		bot.WithDefaultHandler(handler),
		bot.WithWorkers(1),
	)
	if err != nil {
		log.Panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.Start(ctx)
}

func statsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	users, err := getUsersByGroup(update.Message.Chat.ID)
	if err != nil {
		log.Printf("error getting users: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Error getting stats.",
		})
		return
	}

	if len(users) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "No stats yet.",
		})
		return
	}

	sort.Slice(users, func(i, j int) bool {
		scoreI, scoreJ := users[i].Score(), users[j].Score()
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		if users[i].TotalGames != users[j].TotalGames {
			return users[i].TotalGames < users[j].TotalGames
		}
		return users[i].LastPlayedAt.After(users[j].LastPlayedAt)
	})

	topN := 5
	if len(users) < topN {
		topN = len(users)
	}

	var msg string
	for i := 0; i < topN; i++ {
		u := users[i]
		name := u.Username
		if name == "" {
			name = fmt.Sprintf("User_%d", u.UserID)
		}
		msg += fmt.Sprintf("%d. %s - %d pts (7️⃣:%d 🍫:%d 🍒:%d 🍋:%d 🎰:%d)\n",
			i+1, name, u.Score(), u.SevenWins, u.BarWins, u.CherryWins, u.LemonWins, u.TotalGames)
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   msg,
	})
}

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	v, ok := parseSlotMachineMessage(update)
	if !ok {
		return
	}

	userID := update.Message.From.ID
	groupID := update.Message.Chat.ID
	username := update.Message.From.Username

	userStat, err := getOrCreateStats(userID, groupID, username)
	if err != nil {
		log.Printf("error getting user: %v", err)
		return
	}

	userStat.TotalGames += 1
	userStat.LastPlayedAt = time.Unix(int64(update.Message.Date), 0)
	defer func() {
		if err := saveStats(userStat); err != nil {
			log.Printf("error saving user: %v", err)
		}
	}()

	left, center, right := v.left(), v.center(), v.right()
	if left != center || center != right {
		return
	}

	switch left {
	case barSlotFace:
		userStat.BarWins += 1
	case cherrySlotFace:
		userStat.CherryWins += 1
	case lemonSlotFace:
		userStat.LemonWins += 1
	case sevenSlotFace:
		userStat.SevenWins += 1
	default:
		log.Printf("unexpected main.slotFace: %#v", left)
	}

	if err := saveStats(userStat); err != nil {
		log.Printf("error saving user: %v", err)
	}
}

func parseSlotMachineMessage(update *models.Update) (slotMachineValue, bool) {
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
