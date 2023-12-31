package main

import (
	"log"
	"os"

	"github.com/BiRabittoh/escarbot/telegram"
	"github.com/BiRabittoh/escarbot/webui"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file provided.")
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("Please set up your BOT_TOKEN in .env!")
	}

	channelId := os.Getenv("CHANNEL_ID")
	if channelId == "" {
		log.Fatal("Please set up your CHANNEL_ID in .env!")
	}

	groupId := os.Getenv("GROUP_ID")
	if groupId == "" {
		log.Fatal("Please set up your GROUP_ID in .env!")
	}

	adminId := os.Getenv("ADMIN_ID")
	if adminId == "" {
		log.Fatal("Please set up your ADMIN_ID in .env!")
	}

	port := os.Getenv("PORT")
	if port == "" {
		log.Println("PORT not set in .env! Defaulting to 3000.")
		port = "3000"
	}

	bot := telegram.NewBot(botToken, channelId, groupId, adminId)
	ui := webui.NewWebUI(port, bot)
	ui.Poll()
}
