package handlers

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	// Make sure you import your services package if you haven't
	"turcompany/internal/services"
)

type TelegramHandlers struct {
	// You will use this later to get tour data
	tourService *services.TourService
}

// FIX: Add the parameter to the function definition here.
func NewTelegramHandlers(tourService *services.TourService) *TelegramHandlers {
	return &TelegramHandlers{tourService: tourService}
}

func (h *TelegramHandlers) HandleCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(message.Chat.ID, "")
	switch message.Command() {
	case "start":
		msg.Text = "Welcome to the Tour Company Bot!"
	case "tours":
		// Here you would call your tourService to get the list of tours
		// tours := h.tourService.GetAllTours()
		// msg.Text = formatTours(tours)
		msg.Text = "Here are our available tours:\n1. Mountain Adventure\n2. City Exploration"
	default:
		msg.Text = "Unknown command."
	}
	bot.Send(msg)
}

func (h *TelegramHandlers) HandleMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	// Handle regular messages
	msg := tgbotapi.NewMessage(message.Chat.ID, "Please use commands like /tours to interact with me.")
	bot.Send(msg)
}
