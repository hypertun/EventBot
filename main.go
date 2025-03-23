package main

import (
	"EventBot/handler"
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-telegram/bot"
)

func main() {
	// Replace "YOUR_BOT_TOKEN" with your actual bot token
	botToken := "7510313727:AAHj81Bx4Iu32B6wFqpM4i28X_Z20ZEHryA"

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	opts := []bot.Option{
		bot.WithDefaultHandler(handler.OrganiserHandler),
	}

	b, err := bot.New(botToken, opts...)
	if err != nil {
		log.Fatal("error creating bot: ", err)
	}

	b.Start(ctx)
	<-ctx.Done()
	log.Println("Bot stopped")
}
