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

	url := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook?=%s", token, webHookUrl)

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

		fmt.Printf("%+v\n", update)

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
