package bot

import (
	"log"
	"os"
	"turcompany/internal/handlers"
	"turcompany/internal/services"
)

func bot() {
	// 1. Load Config
	token := os.Getenv("TELEGRAM_APITOKEN")
	if token == "" {
		log.Panic("TELEGRAM_APITOKEN environment variable not set")
	}

	// 2. Initialize Services and Handlers
	tourSvc := services.NewTourService()                // <-- Create the tour service
	tgHandlers := handlers.NewTelegramHandlers(tourSvc) // <-- Pass the service instance here

	// Now, initialize the bot service
	botService, err := services.NewTelegramBotService(token)
	if err != nil {
		log.Panic(err)
	}

	// 3. Get the updates channel from the service
	updates := botService.GetUpdatesChannel()

	// 4. Run the main loop here, in main(), not in the service
	log.Println("Bot is running...")
	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Route the update to the correct handler
		if update.Message.IsCommand() {
			// Pass the bot instance from the service to the handler
			tgHandlers.HandleCommand(botService.Bot, update.Message)
		} else {
			// tgHandlers.HandleMessage(botService.Bot, update.Message)
		}
	}
}
