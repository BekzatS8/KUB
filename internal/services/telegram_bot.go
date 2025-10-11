// internal/services/telegram_bot.go

package services

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
)

// The service no longer holds a reference to the handlers
type TelegramBotService struct {
	Bot *tgbotapi.BotAPI // <-- Make the Bot field public
}

// The handlers are no longer needed here
func NewTelegramBotService(token string) (*TelegramBotService, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	bot.Debug = true // Optional: for debugging
	return &TelegramBotService{Bot: bot}, nil
}

// This function now returns the channel of updates for main() to process.
func (s *TelegramBotService) GetUpdatesChannel() tgbotapi.UpdatesChannel {
	log.Printf("Authorized on account %s", s.Bot.Self.UserName)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	return s.Bot.GetUpdatesChan(u)
}
