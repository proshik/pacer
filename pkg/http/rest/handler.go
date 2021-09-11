package rest

import (
	"encoding/json"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"gorun/pkg"
	"gorun/pkg/calculator"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Handler struct {
	Bot        *gorun.TgBot
	Calculator *calculator.Service
}

func NewHandler(bot *gorun.TgBot, calculator *calculator.Service) *Handler {
	return &Handler{bot, calculator}
}

// TimeHandler http://localhost:8080/time?pace=4m50s&dist=21095
func (h *Handler) TimeHandler(w http.ResponseWriter, r *http.Request) {
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

	result := h.Calculator.Time(dist, paceDuration)

	_, _ = w.Write([]byte(result.String()))
}

//type Handler struct {
//	Bot *gorun.TgBot
//	//Calculator *calculator.Service
//}
//
//func NewHandler(bot *gorun.TgBot,
////calculator *calculator.Service
//) *Handler {
//	return &Handler{bot,
//		//, calculator
//	}
//}
//
//// TimeHandler http://localhost:8080/time?pace=4m50s&dist=21095
//func TimeHandler(s calculator.Service) func(w http.ResponseWriter, r *http.Request) {
//	return func(w http.ResponseWriter, r *http.Request) {
//		query := r.URL.Query()
//
//		// validate query params
//		errs := url.Values{}
//		distValue := query.Get("dist")
//		if distValue == "" {
//			errs.Add("dist", "field is required")
//		}
//
//		paceValue := query.Get("pace")
//		if paceValue == "" {
//			errs.Add("pace", "field is required")
//		}
//
//		dist, err := strconv.Atoi(distValue)
//		if err != nil {
//			errs.Add("dist", "incorrect type should be number")
//		}
//
//		paceDuration, err := time.ParseDuration(paceValue)
//		if err != nil {
//			errs.Add("pace", err.Error())
//		}
//
//		// TODO figure out it
//		if &paceDuration == nil {
//			errs.Add("pace", "parsed value is nil")
//		}
//
//		if len(errs) > 0 {
//			w.Header().Set("Content-type", "application/json")
//			w.WriteHeader(http.StatusBadRequest)
//			err := json.NewEncoder(w).Encode(errs)
//			if err != nil {
//				log.Printf("Error on build json error: %v", err)
//				w.WriteHeader(http.StatusInternalServerError)
//			}
//
//			return
//		}
//
//		result := s.Time(dist, paceDuration)
//
//		_, _ = w.Write([]byte(result.String()))
//	}
//}

// PaceHandler http://localhost:8080/pace?dist=21097&time=1h38m48s
func (h *Handler) PaceHandler(w http.ResponseWriter, r *http.Request) {
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

	result := h.Calculator.Pace(dist, timeDuration)

	_, _ = w.Write([]byte(result.String()))
}

func (h *Handler) TgWebHookHandler(_ http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		return
	}

	var update tgbotapi.Update
	err = json.Unmarshal(data, &update)
	if err != nil {
		log.Println(err)
		return
	}

	h.Bot.DoUpdate(update)
}
