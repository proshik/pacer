package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func NewHandler() {

}

// http://localhost:8080/time?pace=4m50s&dist=21095
func timeHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// validate query params
	errs := url.Values{}
	distValue := query.Get("dist")
	if distValue == "" {
		errs.Add("dist", "field is required")
	}

	paceValue := query.Get("pace")
	if paceValue == "" {
		errs.Add("pace", "field is required")
	}

	dist, err := strconv.Atoi(distValue)
	if err != nil {
		errs.Add("dist", "incorrect type should be number")
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
	resultTime := Time(dist, int(paceDuration.Seconds()))

	// to Duration value
	result := time.Duration(resultTime) * time.Second

	_, _ = w.Write([]byte(result.String()))
}

// http://localhost:8080/pace?dist=21097&time=1h38m48s
func paceHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// validate query params
	errs := url.Values{}
	distValue := query.Get("dist")
	if distValue == "" {
		errs.Add("dist", "field is required")
	}

	timeValue := query.Get("time")
	if timeValue == "" {
		errs.Add("time", "field is required")
	}

	dist, err := strconv.Atoi(distValue)
	if err != nil {
		errs.Add("dist", "incorrect type should be number")
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
	resultPace := Pace(dist, int(timeDuration.Seconds()))

	// to Duration value
	result := time.Duration(resultPace) * time.Second

	_, _ = w.Write([]byte(result.String()))
}
