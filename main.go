package main

import (
	"log"
	"os"
)

func main() {
	//serveMux := http.NewServeMux()

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT must be set")
	}

	token := os.Getenv("TELEGRAM_TOKEN")
	host := os.Getenv("HOST")
	//
	//pattern := fmt.Sprintf("/%s", token)
	//
	//webHookUrl := fmt.Sprintf("https://%s/%s", host, token)
	//
	//url := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook?=%s", token, webHookUrl)
	//
	//fmt.Println(url)

	//_, err := http.Get(url)
	//if err != nil {
	//	panic(err)
	//}

	NewTelegramBot(host, port, token)

	//serveMux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
	//	// Read body
	//	b, err := ioutil.ReadAll(r.Body)
	//	defer r.Body.Close()
	//	if err != nil {
	//		http.Error(w, err.Error(), 500)
	//		return
	//	}
	//
	//	fmt.Println(b)
	//})
	//
	//serveMux.HandleFunc("/time", timeHandler)
	//serveMux.HandleFunc("/pace", paceHandler)
	//
	//log.Println("Listening ... ")
	//err := http.ListenAndServe(":"+port, serveMux)
	//if err != nil {
	//	panic(err)
	//}
}
