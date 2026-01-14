package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var updateFlag = flag.Bool("update", false, "update test files with actual responses")

// loadTestFile reads a test scenario from a .txt file
func loadTestFile(filename string) (string, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// saveTestFile writes a test scenario to a .txt file
func saveTestFile(filename string, content string) error {
	return os.WriteFile(filename, []byte(content), 0644)
}

// findTestFiles finds all .txt files in testdata directory
func findTestFiles(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// updateTestFile updates the expected responses in a test file
func updateTestFile(filename string, scenarios []TestScenario, actualResponses []string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	responseIndex := 0

	i := 0
	for i < len(lines) {
		line := lines[i]
		newLines = append(newLines, line)

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "> @") && trimmed != "> @bot" {
			// Skip to command
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				newLines = append(newLines, lines[i])
				i++
			}
			if i < len(lines) {
				newLines = append(newLines, lines[i])
				i++
			}

			// Skip to bot response marker
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				newLines = append(newLines, lines[i])
				i++
			}
			if i < len(lines) {
				newLines = append(newLines, lines[i]) // Keep "> @bot" line
				i++
			}

			// Replace old expected response with new one
			if responseIndex < len(actualResponses) {
				newLines = append(newLines, actualResponses[responseIndex])
				responseIndex++

				// Skip old expected response lines
				for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "> @") {
					i++
				}
			}
		} else {
			i++
		}
	}

	return os.WriteFile(filename, []byte(strings.Join(newLines, "\n")), 0644)
}

// MockBot simulates *bot.Bot for testing
type MockBot struct {
	messages []string
	diceValues []int
}

func NewMockBot() *MockBot {
	return &MockBot{
		messages: []string{},
		diceValues: []int{},
	}
}

func (m *MockBot) SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	msg := &models.Message{
		ID:   len(m.messages) + 1,
		Text: params.Text,
	}
	m.messages = append(m.messages, params.Text)
	return msg, nil
}

func (m *MockBot) SendDice(ctx context.Context, params *bot.SendDiceParams) (*models.Message, error) {
	msg := &models.Message{
		ID: len(m.messages) + 1,
		Dice: &models.Dice{
			Emoji: params.Emoji,
			Value: m.NextDiceValue(),
		},
	}
	return msg, nil
}

func (m *MockBot) DeleteMessage(ctx context.Context, params *bot.DeleteMessageParams) (bool, error) {
	// Mock implementation - just return true (deleted successfully)
	return true, nil
}

func (m *MockBot) NextDiceValue() int {
	if len(m.diceValues) == 0 {
		return 1
	}
	val := m.diceValues[0]
	m.diceValues = m.diceValues[1:]
	return val
}

func (m *MockBot) SetDiceValues(values []int) {
	m.diceValues = values
}

func (m *MockBot) GetMessages() []string {
	return m.messages
}

func (m *MockBot) GetLastMessage() string {
	if len(m.messages) == 0 {
		return ""
	}
	return m.messages[len(m.messages)-1]
}

func (m *MockBot) ClearMessages() {
	m.messages = []string{}
	m.diceValues = []int{}
}

// parseTestInput parses the test format and returns test scenarios
type TestScenario struct {
	Username string
	UserID   int64
	Command  string
	Expected string
}

func parseTestInput(input string) []TestScenario {
	scenarios := []TestScenario{}
	lines := strings.Split(input, "\n")

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines
		if line == "" {
			i++
			continue
		}

		// Parse username line: "> @username (id=N)" or "> @bot"
		if strings.HasPrefix(line, "> @") {
			usernameLine := strings.TrimPrefix(line, "> @")
			username := usernameLine
			userID := int64(0)

			// Check if it's @bot (no ID needed)
			if strings.HasPrefix(usernameLine, "bot") {
				username = "bot"
				userID = 0
			} else if strings.Contains(usernameLine, "(id=") {
				// Parse explicit ID
				parts := strings.Split(usernameLine, "(id=")
				username = strings.TrimSpace(parts[0])
				idStr := strings.TrimSuffix(parts[1], ")")
				_, err := fmt.Sscanf(idStr, "%d", &userID)
				if err != nil || userID == 0 {
					panic(fmt.Sprintf("invalid user ID format in line: %s", line))
				}
			} else {
				// User ID is required for non-bot users
				panic(fmt.Sprintf("user ID required for @%s - use format: @%s (id=N)", usernameLine, usernameLine))
			}

			i++

			// Skip empty lines between username and command
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}

			// Parse command
			if i >= len(lines) {
				break
			}
			command := strings.TrimSpace(lines[i])
			i++

			// Skip empty lines between command and bot response
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}

			// Parse expected response: "> @bot"
			if i >= len(lines) {
				break
			}
			botLine := strings.TrimSpace(lines[i])
			if !strings.HasPrefix(botLine, "> @bot") {
				break
			}
			i++

			// Parse expected output (multi-line until next "> @")
			expected := ""
			for i < len(lines) {
				nextLine := lines[i]
				if strings.HasPrefix(nextLine, "> @") {
					break
				}
				trimmed := strings.TrimSpace(nextLine)
				if trimmed != "" {
					if expected != "" {
						expected += "\n"
					}
					expected += trimmed
				}
				i++
			}

			scenarios = append(scenarios, TestScenario{
				Username: username,
				UserID:   userID,
				Command:  command,
				Expected: expected,
			})
		} else {
			i++
		}
	}

	return scenarios
}

// runScenario executes a single test scenario
func runScenario(t *testing.T, scenario TestScenario, svc *casinoController) {
	ctx := context.Background()
	mockBot := NewMockBot()

	// Create a mock update based on the scenario
	update := &models.Update{
		ID: 1,
		Message: &models.Message{
			ID:   1,
			From: &models.User{
				ID:       scenario.UserID,
				Username: scenario.Username,
			},
			Chat: models.Chat{
				ID:   1,
				Type: models.ChatTypeGroup,
			},
			Text: scenario.Command,
		},
	}

	// Parse command to determine which handler to call
	command := strings.TrimSpace(strings.Fields(scenario.Command)[0])

	switch {
	case command == "/stats":
		svc.statsHandler(ctx, mockBot, update)
	case command == "/balance":
		svc.balanceHandler(ctx, mockBot, update)
	case strings.HasPrefix(command, "/duel"):
		svc.duelHandler(ctx, mockBot, update)
	case command == "/acceptDuel":
		svc.acceptDuelHandler(ctx, mockBot, update)
	case command == "/declineDuel":
		svc.declineDuelHandler(ctx, mockBot, update)
	case command == "/cancelDuel":
		svc.cancelDuelHandler(ctx, mockBot, update)
	case strings.HasPrefix(command, "/slots"):
		// Handle slot machine
		update.Message.Dice = &models.Dice{
			Emoji: "🎰",
			Value: 1, // Default value, can be overridden
		}
		svc.defaultHandler(ctx, mockBot, update)
	default:
		svc.defaultHandler(ctx, mockBot, update)
	}

	// Check the result
	actual := mockBot.GetLastMessage()
	if actual != scenario.Expected {
		t.Errorf("Command: %s\nExpected: %q\nActual:   %q", scenario.Command, scenario.Expected, actual)
	}
}

// TestTxtFiles runs all tests from testdata/*.txt files
func TestTxtFiles(t *testing.T) {
	flag.Parse()

	// Find all test files
	files, err := findTestFiles("testdata/*.txt")
	if err != nil {
		t.Fatalf("failed to find test files: %v", err)
	}

	if len(files) == 0 {
		t.Skip("no test files found in testdata/")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			// Open in-memory database for each test
			db, err := OpenDB()
			if err != nil {
				t.Fatalf("failed to open test database: %v", err)
			}

			svc := newCasinoController("test-token", "testbot", db)

			// Load test file
			input, err := loadTestFile(file)
			if err != nil {
				t.Fatalf("failed to load test file %s: %v", file, err)
			}

			scenarios := parseTestInput(input)
			if len(scenarios) == 0 {
				t.Fatal("no scenarios parsed")
			}

			// Run all scenarios in sequence with shared state
			mockBot := NewMockBot()
			ctx := context.Background()
			var actualResponses []string

			for _, scenario := range scenarios {
				// Re-create the update for each scenario
				update := &models.Update{
					ID: 1,
					Message: &models.Message{
						ID:   1,
						From: &models.User{
							ID:       scenario.UserID,
							Username: scenario.Username,
						},
						Chat: models.Chat{
							ID:   1,
							Type: models.ChatTypeGroup,
						},
						Text: scenario.Command,
					},
				}

				command := strings.TrimSpace(strings.Fields(scenario.Command)[0])

				switch {
				case command == "/stats":
					svc.statsHandler(ctx, mockBot, update)
				case command == "/balance":
					svc.balanceHandler(ctx, mockBot, update)
				case strings.HasPrefix(command, "/duel"):
					svc.duelHandler(ctx, mockBot, update)
				case command == "/acceptDuel":
					svc.acceptDuelHandler(ctx, mockBot, update)
				case command == "/declineDuel":
					svc.declineDuelHandler(ctx, mockBot, update)
				case command == "/cancelDuel":
					svc.cancelDuelHandler(ctx, mockBot, update)
				default:
					svc.defaultHandler(ctx, mockBot, update)
				}

				actual := mockBot.GetLastMessage()
				actualResponses = append(actualResponses, actual)

				// If update flag is set, don't check expectations
				if !*updateFlag && actual != scenario.Expected {
					t.Errorf("Command: %s\nExpected: %q\nActual:   %q", scenario.Command, scenario.Expected, actual)
				}
			}

			// Update test file if flag is set
			if *updateFlag {
				if err := updateTestFile(file, scenarios, actualResponses); err != nil {
					t.Fatalf("failed to update test file %s: %v", file, err)
				}
				t.Logf("Updated test file: %s", file)
			}
		})
	}
}

