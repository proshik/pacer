package main

import (
	"encoding/json"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

// 1. забираю из переменных значения порта, токена, хоста, признак локальная тачка или нет.
// 2. если локальная тачка, то создаю бота через polling, иначе webhook.

// 0. по командам, или кнопкам (снизу): тайм, пейс сделать возможность посчитать по заданнм после параметрам.
//Отправляет команду, зате подсказки: ввведи расстояние в киломлетрах или с точностью до 100м через точку или запятую,
//введи пейс через пробел или двоеточие, где группа цифр это часы, минуты, секунды. Если групп
//цифр всего 2, тогда это минуты и часы, если одна то это секунды. например: 5 15, 03:35:00, 03:52, 1 39 00
// 1. делаю клавиатуру, для возмности задания
//- (дистанция + пейс) = тайм
//- (дистанция + тайм) = пейс
// 2. используя библиотеку и по отдельной кнопке генерирую картинку (svg) с рерзультатом по заданным параметрам

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT must be set")
	}

	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_TOKEN must be set")
	}

	host := os.Getenv("HOST")
	if host == "" {
		log.Fatal("{ must be set")
	}

	fmt.Printf("starting: host=%s, port=%s", host, port)

	pattern := fmt.Sprintf("/%s", token)

	webHookUrl := fmt.Sprintf("https://%s/%s", host, token)

	url := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook?url=%s", token, webHookUrl)

	log.Printf("url: %s\n", url)

	_, err := http.Get(url)
	if err != nil {
		panic(err)
	}

	//go NewTelegramBot(host, port, token)

	serveMux := http.NewServeMux()

	//updates := make(chan tgbotapi.Update)

	serveMux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("YESSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSs")
		bytes, _ := ioutil.ReadAll(r.Body)

		var update tgbotapi.Update
		_ = json.Unmarshal(bytes, &update)

		b, err := json.MarshalIndent(update, "", "  ")
		if err != nil {
			fmt.Println(err)
		}

		fmt.Printf("%s\n", b)

		//updates <- update
	})

	//go func() {
	//	for update := range updates {
	//		log.Printf("%+v\n", update)
	//	}
	//}()

	serveMux.HandleFunc("/time", timeHandler)
	serveMux.HandleFunc("/pace", paceHandler)

	log.Println("Listening ... ")
	err = http.ListenAndServe(":"+port, serveMux)
	if err != nil {
		panic(err)
	}
}
