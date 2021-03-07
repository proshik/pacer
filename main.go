package main

import (
	"fmt"
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

	serveMux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		// Read body
		b, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		fmt.Println(b)
	})

	serveMux.HandleFunc("/time", timeHandler)
	serveMux.HandleFunc("/pace", paceHandler)

	log.Println("Listening ... ")
	err = http.ListenAndServe(":"+port, serveMux)
	if err != nil {
		panic(err)
	}
}
