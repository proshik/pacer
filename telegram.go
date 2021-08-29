package main

import (
	"bytes"
	"encoding/json"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type TgBot struct {
	tg *tgbotapi.BotAPI
}

// incoming command channels
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

	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// handle incoming messages
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

	// handle outgoing messages
	go func() {
		for msg := range messages {
			_, err := bot.Send(msg)
			if err != nil {
				log.Println(err)
			}
		}
	}()

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

	for update := range updates {
		bot.doUpdate(update)
	}
}

func (bot *TgBot) doUpdate(update tgbotapi.Update) {
	//log.Printf("%+v\n", update)
	if update.Message != nil && update.Message.IsCommand() {
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
		if update.Message != nil {
			startC <- update
		} else {
			log.Println("update.Message is nil")
		}
	}
}

func handleStartCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	buf := bytes.NewBufferString("Calculate Your Running Pace")

	// descriptions of commands
	buf.WriteString("\n")
	buf.WriteString("Please, enter of the following commands:\n\n")
	buf.WriteString("[/time]() - calculate time, e.g. */time 4m50s 21095*, where first - pace, second - distance\n")
	buf.WriteString("[/pace]() - calculate pace, e.g. */pace 21097 1h38m48s*, where first - distance, second - time\n")

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
	if len(data) != 2 {
		return buildMsg(update, "should be 2 arguments: (pace, dist) separated by a space")
	}

	paceDuration, err := time.ParseDuration(data[0])
	if err != nil {
		return buildMsg(update, "invalid pace value: "+data[1])
	}

	dist, err := strconv.Atoi(data[1])
	if err != nil {
		return buildMsg(update, "invalid dist value: "+data[1])
	}

	resultTime := Time(dist, int(paceDuration.Seconds()))

	result := time.Duration(resultTime) * time.Second

	return buildMsg(update, result.String())
}

func handlePaceCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	arguments := update.Message.CommandArguments()
	if arguments == "" {
		return buildMsg(update, "empty arguments")
	}

	data := strings.Split(arguments, " ")
	if len(data) != 2 {
		return buildMsg(update, "should be 2 arguments: (pace, dist) separated by a space")
	}

	dist, err := strconv.Atoi(data[0])
	if err != nil {
		return buildMsg(update, "invalid dist value: "+data[0])
	}

	timeDuration, err := time.ParseDuration(data[1])
	if err != nil {
		return buildMsg(update, "invalid time value: "+data[1])
	}

	resultPace := Pace(dist, int(timeDuration.Seconds()))

	result := time.Duration(resultPace) * time.Second

	return buildMsg(update, result.String())
}

func buildMsg(update *tgbotapi.Update, text string) tgbotapi.Chattable {
	return tgbotapi.NewMessage(update.Message.Chat.ID, text)
}

// move to handler.go
func (bot *TgBot) tgWebHookHandler(_ http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		return
	}

	var update tgbotapi.Update
	err = json.Unmarshal(data, &update)
	if err != nil {
		log.Println(err)
		return
	}

	bot.doUpdate(update)
}
