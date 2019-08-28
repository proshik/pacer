package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func main() {
	// http://localhost:8080/time?distance=10000&pace=4m25s
	http.HandleFunc("/time", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		// validate query params
		errs := url.Values{}
		distanceValue := query.Get("distance")
		if distanceValue == "" {
			errs.Add("distance", "field is required")
		}

		paceValue := query.Get("pace")
		if paceValue == "" {
			errs.Add("pace", "field is required")
		}

		distance, err := strconv.Atoi(distanceValue)
		if err != nil {
			errs.Add("distance", "incorrect type should be number")
		}

		paceDuration, err := time.ParseDuration(paceValue)
		if err != nil {
			errs.Add("pace", err.Error())
		}

		// TODO figure out it
		if &paceDuration == nil {
			errs.Add("pace", "parsed value is nil")
		}

		if len(errs) > 0 {
			w.Header().Set("Content-type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			err := json.NewEncoder(w).Encode(errs)
			if err != nil {
				log.Printf("Error on build json error: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
			}

			return
		}

		// invoke method
		resultTime := Time(distance, int(paceDuration.Seconds()))

		// to Duration value
		result := time.Duration(resultTime) * time.Second

		_, _ = w.Write([]byte(result.String()))
	})

	// http://localhost:8080/pace?distance=21097&time=1h38m48s
	http.HandleFunc("/pace", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		// validate query params
		errs := url.Values{}
		distanceValue := query.Get("distance")
		if distanceValue == "" {
			errs.Add("distance", "field is required")
		}

		timeValue := query.Get("time")
		if timeValue == "" {
			errs.Add("time", "field is required")
		}

		distance, err := strconv.Atoi(distanceValue)
		if err != nil {
			errs.Add("distance", "incorrect type should be number")
		}

		timeDuration, err := time.ParseDuration(timeValue)
		if err != nil {
			errs.Add("time", err.Error())
		}

		// TODO figure out it
		if &timeDuration == nil {
			errs.Add("pace", "parsed value is nil")
		}

		if len(errs) > 0 {
			w.Header().Set("Content-type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			err := json.NewEncoder(w).Encode(errs)
			if err != nil {
				log.Printf("Error on build json error: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
			}

			return
		}

		// invoke method
		resultPace := Pace(distance, int(timeDuration.Seconds()))

		// to Duration value
		result := time.Duration(resultPace) * time.Second

		_, _ = w.Write([]byte(result.String()))
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
