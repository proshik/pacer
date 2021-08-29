package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
)

// 1. забираю из переменных значения порта, токена, хоста, признак локальная тачка или нет.
// 2. если локальная тачка, то создаю бота через polling, иначе webhook.

// 0. по командам, или кнопкам (снизу): тайм, пейс сделать возможность посчитать по заданнм после параметрам.
// Отправляет команду, зате подсказки: ввведи расстояние в киломлетрах или с точностью до 100м через точку или запятую,
// введи пейс через пробел или двоеточие, где группа цифр это часы, минуты, секунды. Если групп
// цифр всего 2, тогда это минуты и часы, если одна то это секунды. например: 5 15, 03:35:00, 03:52, 1 39 00
// 1. делаю клавиатуру, для возмности задания
//- (дистанция + пейс) = тайм
//- (дистанция + тайм) = пейс
// 2. используя библиотеку и по отдельной кнопке генерирую картинку (svg) с рерзультатом по заданным параметрам

const TgApiUrl = "https://api.telegram.org"

const TgMethodSetWebHook = "setWebhook"
const TgMethodDeleteWebHook = "deleteWebhook"

func main() {
	// read environment variables
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT must be set")
	}

	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_TOKEN must be set")
	}

	host := os.Getenv("HOST")
	if host == "" {
		log.Fatal("HOST must be set")
	}

	debug := os.Getenv("DEBUG")
	debugMode := isDebugMode(debug)

	fmt.Printf("starting: host=%s, port=%s", host, port)

	/************* DI ***************/

	calculator := NewCalculator()

	bot, err := NewTelegramBot(token, debugMode, calculator)
	if err != nil {
		panic(err)
	}

	handler := NewHandler(bot, calculator)

	/************* DI END ***************/

	serveMux := http.NewServeMux()
	// debugMode methods
	if debugMode {
		serveMux.HandleFunc("/time", handler.TimeHandler)
		serveMux.HandleFunc("/pace", handler.PaceHandler)
	}

	if host == "localhost" {
		// need to disable web hook and set up polling bot
		deleteWebHookUrl := fmt.Sprintf("%s/bot%s/%s?drop_pending_updates=true", TgApiUrl, token, TgMethodDeleteWebHook)
		_, err := http.Get(deleteWebHookUrl)
		if err != nil {
			panic(err)
		}

		go func() {
			bot.ReadUpdates()
		}()
	} else {
		// setting webhook
		webHookUrl := fmt.Sprintf("https://%s/%s", host, token)
		setWebHookUrl := fmt.Sprintf("%s/bot%s/%s?url=%s", TgApiUrl, token, TgMethodSetWebHook, webHookUrl)

		log.Printf("url: %s\n", setWebHookUrl)

		_, err := http.Get(setWebHookUrl)
		if err != nil {
			panic(err)
		}

		// listen messages handler
		serveMux.HandleFunc(fmt.Sprintf("/%s", token), handler.TgWebHookHandler)
	}

	log.Printf("Listening on port=%s ... ", port)

	err = http.ListenAndServe(fmt.Sprintf(":%s", port), serveMux)
	if err != nil {
		panic(err)
	}
}

func isDebugMode(debug string) bool {
	var debugMode bool
	if debug != "" {
		debugModeValue, err := strconv.ParseBool(debug)
		if err != nil {
			panic(err)
		}
		debugMode = debugModeValue
	} else {
		debugMode = false
	}
	return debugMode
}
