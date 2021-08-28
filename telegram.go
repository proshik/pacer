package main

import (
	"bytes"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"log"
	"strconv"
	"strings"
	"time"
)

type TgBot struct {
	tg *tgbotapi.BotAPI
}

// incoming command chanells
var startC = make(chan tgbotapi.Update)
var timeC = make(chan tgbotapi.Update)
var paceC = make(chan tgbotapi.Update)

// send message
var messages = make(chan tgbotapi.Chattable)

func NewTelegramBot(token string) (*TgBot, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	return &TgBot{bot}, nil
}

func (bot *TgBot) ReadUpdates() {
	// create timeout value
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	//read updates from telegram server
	updates, err := bot.tg.GetUpdatesChan(u)
	if err != nil {
		log.Println(err)
	}

	// handle commands from channels
	go func() {
		for {
			select {
			case u := <-startC:
				messages <- handleStartCmd(&u)
			case u := <-timeC:
				messages <- handleTimeCmd(&u)
			case u := <-paceC:
				messages <- handlePaceCmd(&u)
			}
		}
	}()
	// Отправка сообщений пользователям.
	// Отдельно от предыдущего блока т.к. в select нельзя обрабатывать каналы команд из которох читается(*С) и куда записыватеся(messages)
	go func() {
		for res := range messages {
			_, err := bot.tg.Send(res)
			if err != nil {
				log.Println(err)
			}
		}
	}()

	for update := range updates {
		bot.doUpdate(update)
	}

}

func (bot *TgBot) doUpdate(update tgbotapi.Update) {
	//log.Printf("%+v\n", update)
	if update.Message.IsCommand() {
		// send the chat action message
		go func() {
			_, err := bot.tg.Send(tgbotapi.NewChatAction(update.Message.Chat.ID, tgbotapi.ChatTyping))
			if err != nil {
				log.Println(err)
			}
		}()

		switch update.Message.Command() {
		case "start":
			startC <- update
		case "time":
			timeC <- update
		case "pace":
			paceC <- update
		default:
			// show access commands
			startC <- update
		}
	} else {
		startC <- update
	}
}

func handleStartCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	buf := bytes.NewBufferString("Calculate Your Running Pace")

	// descriptions of commands
	buf.WriteString("\n")
	buf.WriteString("Please, enter of the following commands:\n\n")
	buf.WriteString("[/time]() - calculate time, e.g. 4m50s 21095, where first - pace, second - distance\n")
	buf.WriteString("[/pace]() - calculate pace, e.g. 21097 1h38m48s, where first - distance, second - time\n")

	// create message
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, buf.String())
	msg.ParseMode = "markdown"
	return msg
}

func handleTimeCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	arguments := update.Message.CommandArguments()
	if arguments == "" {
		return buildMsg(update, "empty arguments")
	}

	data := strings.Split(arguments, " ")
	if len(data) < 2 {
		return buildMsg(update, "should be 2 argument pace, dist separated by a space")
	}

	paceDuration, err := time.ParseDuration(data[0])
	if err != nil {
		return buildMsg(update, "invalid pace value")
	}

	dist, err := strconv.Atoi(data[1])
	if err != nil {
		return buildMsg(update, "invalid dist value")
	}

	resultTime := Time(dist, int(paceDuration.Seconds()))

	// to Duration value
	result := time.Duration(resultTime) * time.Second

	return buildMsg(update, result.String())
}

func handlePaceCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	return buildMsg(update, "not implemented yet")
}

func buildMsg(update *tgbotapi.Update, text string) tgbotapi.Chattable {
	return tgbotapi.NewMessage(update.Message.Chat.ID, text)
}
