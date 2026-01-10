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
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Panic("BOT_TOKEN environment variable is not set")
	}

	botUsername := os.Getenv("BOT_USERNAME")
	if botUsername == "" {
		log.Panic("BOT_USERNAME environment variable is not set")
	}

	svc := newCasinoController(botToken, botUsername)

	if err := initDB(); err != nil {
		log.Panic(err)
	}

	tgBot, err := bot.New(
		botToken,
		bot.WithMessageTextHandler("/web", bot.MatchTypeExact, svc.webHandler),
		bot.WithMessageTextHandler("/stats", bot.MatchTypeExact, svc.statsHandler),
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
}

func newCasinoController(token string, username string) *casinoController {
	return &casinoController{
		token:    token,
		username: username,
	}
}

func (c *casinoController) webHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Welcome! Click the button below to launch the mini app.",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text: "🎰 Open Mini App",
						URL:  "https://t.me/normans_bot_casino?startapp",
					},
				},
			},
		},
	})
}

func (*casinoController) statsHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
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
		if users[i].Score != users[j].Score {
			return users[i].Score > users[j].Score
		}
		if users[i].TotalGames != users[j].TotalGames {
			return users[i].TotalGames < users[j].TotalGames
		}
		return users[i].LastPlayedAt.After(users[j].LastPlayedAt)
	})

	var msg string
	for i := 0; i < len(users); i++ {
		u := users[i]
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

func (c *casinoController) diceHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	v, ok := c.parseSlotMachineMessage(update)
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
		userStat.Score += 50
	case cherrySlotFace:
		userStat.CherryWins += 1
		userStat.Score += 10
	case lemonSlotFace:
		userStat.LemonWins += 1
		userStat.Score += 20
	case sevenSlotFace:
		userStat.SevenWins += 1
		userStat.Score += 100
	default:
		log.Printf("unexpected main.slotFace: %#v", left)
	}

	if err := saveStats(userStat); err != nil {
		log.Printf("error saving user: %v", err)
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
