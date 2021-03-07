package main

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"log"
)

func NewTelegramBot(host string, port string, token string) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	webHookConfig := tgbotapi.NewWebhook(fmt.Sprintf("https://%s/%s", host, token))

	_, err = bot.SetWebhook(webHookConfig)
	if err != nil {
		log.Fatal(err)
	}

	//info, err := bot.GetWebhookInfo()
	//if err != nil {
	//	log.Fatal(err)
	//}

	//if info.LastErrorDate != 0 {
	//	log.Printf("Telegram callback failed: %s", info.LastErrorMessage)
	//}
	updates := bot.ListenForWebhook("/" + bot.Token)

	for update := range updates {
		log.Printf("%+v\n", update)
	}
}
