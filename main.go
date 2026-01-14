package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
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
		bot.WithMessageTextHandler("/stats", bot.MatchTypeExact, svc.wrapHandler(svc.statsHandler)),
		bot.WithMessageTextHandler("/balance", bot.MatchTypeExact, svc.wrapHandler(svc.balanceHandler)),
		bot.WithMessageTextHandler("/duel", bot.MatchTypePrefix, svc.wrapHandler(svc.duelHandler)),
		bot.WithMessageTextHandler("/acceptDuel", bot.MatchTypeExact, svc.wrapHandler(svc.acceptDuelHandler)),
		bot.WithMessageTextHandler("/declineDuel", bot.MatchTypeExact, svc.wrapHandler(svc.declineDuelHandler)),
		bot.WithMessageTextHandler("/cancelDuel", bot.MatchTypeExact, svc.wrapHandler(svc.cancelDuelHandler)),
		bot.WithDefaultHandler(svc.wrapHandler(svc.defaultHandler)),
		bot.WithWorkers(1),
	)
	if err != nil {
		log.Panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tgBot.Start(ctx)
}

// BotInterface defines the methods we need from the bot
type BotInterface interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	SendDice(ctx context.Context, params *bot.SendDiceParams) (*models.Message, error)
	DeleteMessage(ctx context.Context, params *bot.DeleteMessageParams) (bool, error)
}

type casinoController struct {
	token        string
	username     string
	db           *DB
	pendingDuels   map[int64]*PendingDuel // groupID -> PendingDuel
	pendingDuelsMu sync.RWMutex
}

func newCasinoController(token string, username string, db *DB) *casinoController {
	return &casinoController{
		token:        token,
		username:     username,
		db:           db,
		pendingDuels: make(map[int64]*PendingDuel),
	}
}

// wrapHandler converts a handler using BotInterface to use *bot.Bot
func (c *casinoController) wrapHandler(handler func(context.Context, BotInterface, *models.Update)) func(context.Context, *bot.Bot, *models.Update) {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		handler(ctx, b, update)
	}
}

func (c *casinoController) statsHandler(ctx context.Context, b BotInterface, update *models.Update) {
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

func (c *casinoController) balanceHandler(ctx context.Context, b BotInterface, update *models.Update) {
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

func (c *casinoController) defaultHandler(ctx context.Context, b BotInterface, update *models.Update) {
	// Debug: print raw JSON
	if jsonBytes, err := json.Marshal(update); err == nil {
		fmt.Printf("Raw update: %s\n", string(jsonBytes))
	}

	// Handle slot machine dice
	if v, ok := c.parseSlotMachineMessage(update); ok {
		c.handleSlotMachine(ctx, b, update, v)
		return
	}
}

func (c *casinoController) duelHandler(ctx context.Context, b BotInterface, update *models.Update) {
	if update.Message == nil {
		return
	}

	groupID := update.Message.Chat.ID
	initiatorID := update.Message.From.ID
	initiatorName := update.Message.From.Username

	// Parse target username
	args := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/duel"))
	if args == "" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: groupID,
			Text:   "Usage: /duel <username>",
		})
		return
	}

	targetUsername := strings.TrimPrefix(args, "@")

	// Find target user in the group by checking balances
	balances, err := c.db.GetBalancesByGroup(groupID)
	if err != nil {
		log.Printf("error getting users: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: groupID,
			Text:   "Error getting users.",
		})
		return
	}

	var targetID int64
	var targetFound bool
	var targetBalance int64
	for _, bal := range balances {
		// Get username from stats
		stat, err := c.db.GetOrCreateStats(bal.UserID, groupID, "")
		if err != nil {
			continue
		}
		if stat.Username == targetUsername {
			targetID = bal.UserID
			targetBalance = bal.Amount
			targetFound = true
			break
		}
	}

	if !targetFound || targetBalance <= 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: groupID,
			Text:   "The target is too poor to be challenged",
		})
		return
	}

	// Check if target is too poor
	if targetBalance <= 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: groupID,
			Text:   "The target is too poor to be challenged",
		})
		return
	}

	if targetID == initiatorID {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: groupID,
			Text:   "You cannot duel yourself!",
		})
		return
	}

	// Check if there's already a pending duel in this group
	c.pendingDuelsMu.Lock()
	defer c.pendingDuelsMu.Unlock()

	if existingDuel, exists := c.pendingDuels[groupID]; exists {
		// Check if existing duel has expired
		if time.Now().Before(existingDuel.ExpiresAt) {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: groupID,
				Text:   "There is already a pending duel in this group.",
			})
			return
		}
		// Remove expired duel
		delete(c.pendingDuels, groupID)
	}

	// Store pending duel
	c.pendingDuels[groupID] = &PendingDuel{
		InitiatorID:   initiatorID,
		TargetID:      targetID,
		GroupID:       groupID,
		TargetName:    targetUsername,
		InitiatorName: initiatorName,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}

	// Get initiator balance for display
	initiatorBalance, err := c.db.GetOrCreateBalance(initiatorID, groupID)
	if err != nil {
		log.Printf("error getting initiator balance: %v", err)
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: groupID,
		Text: fmt.Sprintf("@%s (%d$) has challenged @%s (%d$) to a duel!\n\nRules: 🎲 Even = @%s wins, Odd = @%s wins\n\n@%s, type /acceptDuel to accept or /declineDuel to decline.",
			initiatorName, initiatorBalance.Amount, targetUsername, targetBalance,
			initiatorName, targetUsername, targetUsername),
	})
}

func (c *casinoController) acceptDuelHandler(ctx context.Context, b BotInterface, update *models.Update) {
	if update.Message == nil {
		return
	}

	groupID := update.Message.Chat.ID
	targetID := update.Message.From.ID
	messageID := update.Message.ID

	c.pendingDuelsMu.Lock()
	defer c.pendingDuelsMu.Unlock()

	pendingDuel, exists := c.pendingDuels[groupID]
	if !exists {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          groupID,
			Text:            "No pending duel in this group.",
			ReplyParameters: &models.ReplyParameters{MessageID: messageID},
		})
		return
	}

	if pendingDuel.TargetID != targetID {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          groupID,
			Text:            "This duel is not for you!",
			ReplyParameters: &models.ReplyParameters{MessageID: messageID},
		})
		return
	}

	// Get balances
	initiatorBalance, err := c.db.GetOrCreateBalance(pendingDuel.InitiatorID, groupID)
	if err != nil {
		log.Printf("error getting initiator balance: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          groupID,
			Text:            "Error getting balances.",
			ReplyParameters: &models.ReplyParameters{MessageID: messageID},
		})
		return
	}

	targetBalance, err := c.db.GetOrCreateBalance(targetID, groupID)
	if err != nil {
		log.Printf("error getting target balance: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          groupID,
			Text:            "Error getting balances.",
			ReplyParameters: &models.ReplyParameters{MessageID: messageID},
		})
		return
	}

	if initiatorBalance.Amount <= 0 && targetBalance.Amount <= 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          groupID,
			Text:            "Both players have no balance to duel for!",
			ReplyParameters: &models.ReplyParameters{MessageID: messageID},
		})
		delete(c.pendingDuels, groupID)
		return
	}

	// Send dice roll
	diceMsg, err := b.SendDice(ctx, &bot.SendDiceParams{
		ChatID: groupID,
		Emoji:  "🎲",
	})
	if err != nil {
		log.Printf("error sending dice: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          groupID,
			Text:            "Error sending dice roll.",
			ReplyParameters: &models.ReplyParameters{MessageID: messageID},
		})
		return
	}

	// Process the dice result immediately
	if diceMsg.Dice == nil {
		log.Printf("dice message has no dice field")
		return
	}

	diceValue := diceMsg.Dice.Value

	// Get current balances
	initiatorAmount := initiatorBalance.Amount
	targetAmount := targetBalance.Amount

	// Dice roll: even = initiator wins, odd = target wins
	var winnerName string

	if diceValue%2 == 0 {
		// Even - initiator wins
		winnerName = fmt.Sprintf("User_%d", pendingDuel.InitiatorID)
	} else {
		// Odd - target wins
		winnerName = pendingDuel.TargetName
	}

	// Transfer balances atomically - winner takes loser's entire balance
	err = c.db.Transaction(func(tx *gorm.DB) error {
		if diceValue%2 == 0 {
			// Even - initiator wins, take target's balance
			if targetAmount > 0 {
				if err := c.db.TransferBalance(tx, pendingDuel.TargetID, pendingDuel.InitiatorID, groupID, targetAmount); err != nil {
					return err
				}
			}
		} else {
			// Odd - target wins, take initiator's balance
			if initiatorAmount > 0 {
				if err := c.db.TransferBalance(tx, pendingDuel.InitiatorID, pendingDuel.TargetID, groupID, initiatorAmount); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("error transferring balance: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          groupID,
			Text:            "Error transferring balance.",
			ReplyParameters: &models.ReplyParameters{MessageID: messageID},
		})
		return
	}

	resultType := "odd"
	if diceValue%2 == 0 {
		resultType = "even"
	}

	// Calculate amount won (loser's balance) and loser's name
	var amountWon int64
	var loserName string
	if diceValue%2 == 0 {
		amountWon = targetAmount
		loserName = pendingDuel.TargetName
	} else {
		amountWon = initiatorAmount
		loserName = pendingDuel.InitiatorName
	}

	// Wait for dice animation to play out
	time.Sleep(5 * time.Second)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: groupID,
		Text: fmt.Sprintf("🎲 %d (%s)!\n\n@%s wins %d$ from @%s!",
			diceValue, resultType, winnerName, amountWon, loserName),
	})
}

func (c *casinoController) declineDuelHandler(ctx context.Context, b BotInterface, update *models.Update) {
	if update.Message == nil {
		return
	}

	groupID := update.Message.Chat.ID
	targetID := update.Message.From.ID

	c.pendingDuelsMu.Lock()
	defer c.pendingDuelsMu.Unlock()

	pendingDuel, exists := c.pendingDuels[groupID]
	if !exists {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: groupID,
			Text:   "No pending duel in this group.",
		})
		return
	}

	if pendingDuel.TargetID != targetID {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: groupID,
			Text:   "This duel is not for you!",
		})
		return
	}

	delete(c.pendingDuels, groupID)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: groupID,
		Text:   fmt.Sprintf("@%s chickened out of the duel!", pendingDuel.TargetName),
	})
}

func (c *casinoController) cancelDuelHandler(ctx context.Context, b BotInterface, update *models.Update) {
	if update.Message == nil {
		return
	}

	groupID := update.Message.Chat.ID
	initiatorID := update.Message.From.ID

	c.pendingDuelsMu.Lock()
	defer c.pendingDuelsMu.Unlock()

	pendingDuel, exists := c.pendingDuels[groupID]
	if !exists {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: groupID,
			Text:   "No pending duel in this group.",
		})
		return
	}

	if pendingDuel.InitiatorID != initiatorID {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: groupID,
			Text:   "You didn't initiate this duel!",
		})
		return
	}

	delete(c.pendingDuels, groupID)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: groupID,
		Text:   fmt.Sprintf("Duel against @%s has been cancelled.", pendingDuel.TargetName),
	})
}

func (c *casinoController) handleSlotMachine(ctx context.Context, b BotInterface, update *models.Update, v slotMachineValue) {
	userID := update.Message.From.ID
	username := update.Message.From.Username
	groupID := update.Message.Chat.ID
	messageID := update.Message.ID

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
		// Non-winning spin - delete the message after 1 minute
		go func() {
			time.Sleep(1 * time.Minute)
			deleteCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := b.DeleteMessage(deleteCtx, &bot.DeleteMessageParams{
				ChatID:    groupID,
				MessageID: messageID,
			}); err != nil {
				log.Printf("error deleting message: %v", err)
			}
		}()
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
