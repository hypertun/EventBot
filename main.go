package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {
	// Replace "YOUR_BOT_TOKEN" with your actual bot token
	botToken := "7510313727:AAHj81Bx4Iu32B6wFqpM4i28X_Z20ZEHryA"

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
	}

	b, err := bot.New(botToken, opts...)
	if err != nil {
		log.Fatal("error creating bot: ", err)
	}

	b.Start(ctx)
	<-ctx.Done()
	log.Println("Bot stopped")
}

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	log.Printf("[%s] %s", update.Message.From.Username, update.Message.Text)

	chatID := update.Message.Chat.ID
	var text string

	switch update.Message.Text {
	case "/start":
		text = "Hello! I'm a simple echo bot. Send me a message, and I'll repeat it back to you."
	case "/help":
		text = "I'm a simple echo bot. I can repeat your messages. Use /start to get started."
	default:
		text = fmt.Sprintf("You said: %s", update.Message.Text)
	}

	params := &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}

	_, err := b.SendMessage(ctx, params)
	if err != nil {
		log.Println("error sending message:", err)
	}
}
