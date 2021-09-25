package rest

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/NYTimes/gziphandler"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"gorun/pkg/calculator"
	"gorun/pkg/telegram"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func NewHandler(
	debugMode bool,
	tgToken string,
	t *telegram.Service,
	c *calculator.Service,
	a embed.FS,
) *http.ServeMux {
	serveMux := http.NewServeMux()

	if debugMode {
		// debug endpoints
		serveMux.HandleFunc("/time", calculateTime(c))
		serveMux.HandleFunc("/pace", calculatePace(c))
	} else {
		// handle telegram web hook messages
		serveMux.HandleFunc(fmt.Sprintf("/%s", tgToken), handleWebHook(t))
	}

	stripped, err := fs.Sub(a, "assets")
	if err != nil {
		log.Fatalln(err)
	}

	assetsDir := gziphandler.GzipHandler(http.FileServer(http.FS(stripped)))
	serveMux.Handle("/", assetsDir)

	return serveMux
}

// calculateTime http://localhost:8080/time?pace=4m50s&dist=21095
func calculateTime(c *calculator.Service) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
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

		result := c.Time(dist, paceDuration)

		_, _ = w.Write([]byte(result.String()))
	}
}

// calculatePace http://localhost:8080/pace?dist=21097&time=1h38m48s
func calculatePace(c *calculator.Service) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
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

		result := c.Pace(dist, timeDuration)

		_, _ = w.Write([]byte(result.String()))
	}
}

func handleWebHook(t *telegram.Service) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := ioutil.ReadAll(r.Body)

		defer r.Body.Close()

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

		t.DoUpdate(update)
	}
}
