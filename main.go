package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is not set")
	}

	bot, err := bot.New(
		token,
		bot.WithMessageTextHandler("/stats", bot.MatchTypeExact, statsHandler),
		bot.WithDefaultHandler(handler),
		bot.WithWorkers(1),
	)
	if err != nil {
		log.Panic(err)
	}

	// todo: store in sqlite
	stats = map[int64]*userStats{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.Start(ctx)
}

var stats map[int64]*userStats

func statsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if len(stats) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "No stats yet.",
		})

		return
	}

	users := make([]*userStats, 0, len(stats))
	for _, u := range stats {
		users = append(users, u)
	}

	sort.Slice(users, func(i, j int) bool {
		scoreI, scoreJ := users[i].Score(), users[j].Score()
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		if users[i].TotalGames != users[j].TotalGames {
			return users[i].TotalGames < users[j].TotalGames
		}
		return users[i].LastPlayedAt > users[j].LastPlayedAt
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
		msg += fmt.Sprintf("%d. %s - %d pts (🎰:%d, 7️⃣:%d 🫒:%d 🍒:%d 🍋:%d)\n",
			i+1, name, u.Score(), u.TotalGames, u.SevenWins, u.BarWins, u.CherryWins, u.LemonWins)
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
	userStat, ok := stats[userID]
	if !ok {
		userStat = &userStats{UserID: userID, Username: update.Message.From.Username}
		stats[userID] = userStat
	}

	userStat.TotalGames += 1
	userStat.LastPlayedAt = int64(update.Message.Date)

	left, center, right := v.left(), v.center(), v.right()
	if left != center || center != right {
		return // we don't care about losers.
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
