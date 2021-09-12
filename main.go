package main

import (
	"embed"
	"fmt"
	gorun "gorun/pkg"
	calculator2 "gorun/pkg/calculator"
	"gorun/pkg/http/rest"
	"io/fs"
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

//go:embed assets
var assets embed.FS

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

	calculator := calculator2.NewService()

	bot, err := gorun.NewTelegramBot(token, debugMode, calculator)
	if err != nil {
		panic(err)
	}

	handler := rest.NewHandler(bot, calculator)

	/************* DI END ***************/

	serveMux := http.NewServeMux()
	// debugMode methods
	if debugMode {
		serveMux.HandleFunc("/time", handler.TimeHandler)
		serveMux.HandleFunc("/pace", handler.PaceHandler)
	}

	stripped, err := fs.Sub(assets, "assets")
	if err != nil {
		log.Fatalln(err)
	}

	assetsDir := http.FileServer(http.FS(stripped))
	serveMux.Handle("/", assetsDir)

	if host != "localhost" {
		// setting webhook
		webHookUrl := fmt.Sprintf("https://%s/%s", host, token)
		setWebHookUrl :=
			fmt.Sprintf("%s/bot%s/%s?url=%s", gorun.TgApiUrl, token, gorun.TgMethodSetWebHook, webHookUrl)

		log.Printf("url: %s\n", setWebHookUrl)

		_, err := http.Get(setWebHookUrl)
		if err != nil {
			panic(err)
		}

		// listen messages handler
		serveMux.HandleFunc(fmt.Sprintf("/%s", token), handler.TgWebHookHandler)
	}

	fmt.Printf("The server is on tap now on port: %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), serveMux))
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
