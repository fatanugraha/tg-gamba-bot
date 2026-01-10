# Testing with Simulator

This document explains how to write tests for the casino bot using the simulator.

## Overview

The `MockBot` type implements the `BotInterface` and simulates the Telegram bot for testing. It captures all messages sent via `SendMessage` and allows you to control dice roll values.

## Test Format

Tests are stored as `.txt` files in the `testdata/` directory with a simple text format:

```
> @username (id=N)
/command

> @bot
Expected response here
```

- `> @username (id=N)` - The user sending the command with their user ID
- `/command` - The command being sent
- `> @bot` - Marker for the expected bot response (no ID needed)
- Everything after `> @bot` until the next `> @` is the expected response

**Note**: User IDs are **required** for all users except `@bot`. The test will panic if you forget to specify the user ID.

## Creating Test Files

Create a new `.txt` file in the `testdata/` directory:

### Example: testdata/empty_balance.txt

```
> @fata.nugraha (id=1)
/balance

> @bot
No balances yet.
```

### Example: testdata/user_not_found.txt

```
> @fata.nugraha (id=1)
/stats

> @bot
No stats yet.

> @admin (id=2)
/addBalance @fata.nugraha 100

> @bot
User not found.
```

## Running Tests

```bash
# Run all file-based tests
go test -v -run TestTxtFiles

# Run specific test file
go test -v -run TestTxtFiles/empty_balance.txt

# Update test files with actual responses (golden file pattern)
go test -v -run TestTxtFiles -update

# Update specific test file
go test -v -run TestTxtFiles/empty_balance.txt -update

# Run all tests
go test -v
```

## Update Flag

The `-update` flag allows you to update the expected responses in test files automatically. This is useful when:

- First creating a new test file
- Changing bot behavior that affects multiple tests
- Debugging to see what the actual response is

Usage:
```bash
# Update all test files
go test -v -run TestTxtFiles -update

# Update specific test file
go test -v -run TestTxtFiles/my_test.txt -update
```

When `-update` is set:
- Tests will not fail on mismatched expectations
- Actual bot responses are written back to the test files
- Test files are updated with the correct expected values

## MockBot API

### Creating a MockBot

```go
mockBot := NewMockBot()
```

### Setting Dice Values

For tests that involve dice rolls:

```go
mockBot := NewMockBot()
mockBot.SetDiceValues([]int{6, 4, 2}) // Dice will return 6, then 4, then 2
```

### Getting Messages

```go
// Get all messages
allMessages := mockBot.GetMessages()

// Get the last message
lastMsg := mockBot.GetLastMessage()

// Clear messages
mockBot.ClearMessages()
```

## Implementation

### BotInterface

All handlers use `BotInterface` instead of the concrete `*bot.Bot` type:

```go
type BotInterface interface {
    SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
    SendDice(ctx context.Context, params *bot.SendDiceParams) (*models.Message, error)
}
```

This makes it easy to swap in the `MockBot` for testing.

### MockBot Implementation

```go
type MockBot struct {
    messages   []string
    diceValues []int
}

func (m *MockBot) SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
    m.messages = append(m.messages, params.Text)
    return &models.Message{Text: params.Text}, nil
}

func (m *MockBot) SendDice(ctx context.Context, params *bot.SendDiceParams) (*models.Message, error) {
    value := m.NextDiceValue()
    return &models.Message{Dice: &models.Dice{Value: value}}, nil
}
```

## Running Tests

```bash
# Run all tests
go test -v

# Run specific test
go test -v -run TestCase1

# Run with coverage
go test -cover
```

## Notes

- Each test uses an in-memory SQLite database
- Tests are isolated - each test creates a fresh database instance
- The `MockBot` captures messages but doesn't actually send them
- Multi-scenario tests share the same controller and database state
