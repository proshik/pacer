package main

import (
	"log"
	"net/http"
)

func main() {
	serveMux := http.NewServeMux()

	serveMux.HandleFunc("/time", timeHandler)
	serveMux.HandleFunc("/pace", paceHandler)

	log.Println("Listening ... ")
	err := http.ListenAndServe(":8080", serveMux)
	if err != nil {
		panic(err)
	}
}
