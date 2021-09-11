package gorun

import (
	"bytes"
	"errors"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"gorun/pkg/calculator"
	"log"
	"strconv"
	"strings"
	"time"
)

type TgBot struct {
	Tg         *tgbotapi.BotAPI
	Calculator *calculator.Service
}

// incoming command channels
var startC = make(chan tgbotapi.Update)
var timeC = make(chan tgbotapi.Update)
var paceC = make(chan tgbotapi.Update)

// send message
var messages = make(chan tgbotapi.Chattable)

func NewTelegramBot(token string, debugMode bool, calculator *calculator.Service) (*TgBot, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = debugMode

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

	return &TgBot{bot, calculator}, nil
}

func (bot *TgBot) ReadUpdates() {
	// create timeout value
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	//read updates from telegram server
	updates, err := bot.Tg.GetUpdatesChan(u)
	if err != nil {
		log.Println(err)
	}

	for update := range updates {
		bot.DoUpdate(update)
	}
}

func (bot *TgBot) DoUpdate(update tgbotapi.Update) {
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
	arguments, err := extractArguments(update)
	if err != nil {
		return buildMsg(update, err.Error())
	}

	if len(arguments) != 2 {
		return buildMsg(update, "should be 2 arguments: (pace, dist) separated by a space")
	}

	paceDuration, err := time.ParseDuration(arguments[0])
	if err != nil {
		return buildMsg(update, "invalid pace value: "+arguments[1])
	}

	dist, err := strconv.Atoi(arguments[1])
	if err != nil {
		return buildMsg(update, "invalid dist value: "+arguments[1])
	}

	resultTime := calculator.Time(dist, int(paceDuration.Seconds()))

	result := time.Duration(resultTime) * time.Second

	return buildMsg(update, result.String())
}

func handlePaceCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	arguments, err := extractArguments(update)
	if err != nil {
		return buildMsg(update, err.Error())
	}

	if len(arguments) != 2 {
		return buildMsg(update, "should be 2 arguments: (pace, dist) separated by a space")
	}

	dist, err := strconv.Atoi(arguments[0])
	if err != nil {
		return buildMsg(update, "invalid dist value: "+arguments[0])
	}

	timeDuration, err := time.ParseDuration(arguments[1])
	if err != nil {
		return buildMsg(update, "invalid time value: "+arguments[1])
	}

	resultPace := calculator.Pace(dist, int(timeDuration.Seconds()))

	result := time.Duration(resultPace) * time.Second

	return buildMsg(update, result.String())
}

func extractArguments(update *tgbotapi.Update) ([]string, error) {
	arguments := update.Message.CommandArguments()
	if arguments == "" {
		return nil, errors.New("empty arguments")
	}

	data := strings.Split(arguments, " ")

	return data, nil
}

func buildMsg(update *tgbotapi.Update, text string) tgbotapi.Chattable {
	return tgbotapi.NewMessage(update.Message.Chat.ID, text)
}
