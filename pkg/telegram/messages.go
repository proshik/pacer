package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"gorun/pkg/plan"
)

// texts holds every word a reply carries, per language. The plan sentence
// itself comes from pkg/card, which the picture and its caption share.
type texts struct {
	language       string // the card and the plan sentence use the same language
	greeting       string
	unknownCommand string // %s is the command without its slash
	emptyArguments string
	cardFailed     string

	// why a request could not be read, and how to write it
	noDistance    string
	noTime        string
	noPace        string
	ambiguous     string
	implausible   string
	notUnderstood string // followed by the words that were not understood
	usageTime     string
	usagePace     string
	usageCard     string

	// buttons
	openApp       string
	openPlan      string
	splitsButton  string
	predictButton string
	cardButton    string
	backButton    string

	// the splits and prediction views
	splitsLabel    string
	predictLabel   string
	distanceHeader string
	riegel         string
	cameron        string
	finish         string
	km             string
	decimal        string
	half           string
	marathon       string

	// descriptions in the command menu next to the input box
	startCommand string
	timeCommand  string
	paceCommand  string
	cardCommand  string
}

var englishTexts = texts{
	language: "en",
	greeting: "Running pace calculator\n\n" +
		"Send a distance and a time, like “marathon 3:30” or “10k 4:50”, and I’ll work out the plan.\n\n" +
		"/pace 21.1 1:38:48 — pace from distance and time\n" +
		"/time 4:50 10k — time from pace and distance\n" +
		"/card marathon 3:30 — the plan as a picture",
	unknownCommand: "Unknown command: /%s\n\nAvailable commands: /start, /time, /pace, /card",
	emptyArguments: "This command needs arguments.",
	cardFailed:     "Couldn't draw the card",
	noDistance:     "I don't see a distance.",
	noTime:         "I don't see a time.",
	noPace:         "I don't see a pace.",
	ambiguous:      "One distance and one time, please.",
	implausible:    "That doesn't look like a running pace, check the numbers.",
	notUnderstood:  "Didn't understand: ",
	usageTime:      "Send the pace and the distance, for example: /time 4:50 10k",
	usagePace:      "Send the distance and the time, for example: /pace 21.1 1:38:48",
	usageCard:      "Send the distance and the time, for example: /card marathon 3:30",
	openApp:        "Open Pacer",
	openPlan:       "Open in Pacer",
	splitsButton:   "Splits",
	predictButton:  "Prediction",
	cardButton:     "Card",
	backButton:     "← Plan",
	splitsLabel:    "Splits at an even pace:",
	predictLabel:   "What this result predicts:",
	distanceHeader: "distance",
	riegel:         "Riegel",
	cameron:        "Cameron",
	finish:         "finish",
	km:             "km",
	decimal:        ".",
	half:           "Half marathon",
	marathon:       "Marathon",
	startCommand:   "How to use the bot",
	timeCommand:    "Time from pace and distance",
	paceCommand:    "Pace from distance and time",
	cardCommand:    "The plan as a picture",
}

var russianTexts = texts{
	language: "ru",
	greeting: "Беговой калькулятор темпа\n\n" +
		"Напишите дистанцию и время — например «марафон 3:30» или «10k 4:50», — и я посчитаю план.\n\n" +
		"/pace 21.1 1:38:48 — темп по дистанции и времени\n" +
		"/time 4:50 10k — время по темпу и дистанции\n" +
		"/card марафон 3:30 — план картинкой",
	unknownCommand: "Неизвестная команда: /%s\n\nДоступные команды: /start, /time, /pace, /card",
	emptyArguments: "Команде нужны аргументы.",
	cardFailed:     "Не получилось нарисовать карточку",
	noDistance:     "Не вижу дистанцию.",
	noTime:         "Не вижу время.",
	noPace:         "Не вижу темп.",
	ambiguous:      "Нужны одна дистанция и одно время.",
	implausible:    "На беговой темп не похоже — проверьте числа.",
	notUnderstood:  "Не понял: ",
	usageTime:      "Нужны темп и дистанция, например: /time 4:50 10k",
	usagePace:      "Нужны дистанция и время, например: /pace 21.1 1:38:48",
	usageCard:      "Нужны дистанция и время, например: /card марафон 3:30",
	openApp:        "Открыть Pacer",
	openPlan:       "Открыть в Pacer",
	splitsButton:   "Раскладка",
	predictButton:  "Прогноз",
	cardButton:     "Карточка",
	backButton:     "← План",
	splitsLabel:    "Раскладка при ровном темпе:",
	predictLabel:   "Прогноз по этому результату:",
	distanceHeader: "дистанция",
	riegel:         "Ригель",
	cameron:        "Кэмерон",
	finish:         "финиш",
	km:             "км",
	decimal:        ",",
	half:           "Полумарафон",
	marathon:       "Марафон",
	startCommand:   "Как пользоваться",
	timeCommand:    "Время по темпу и дистанции",
	paceCommand:    "Темп по дистанции и времени",
	cardCommand:    "Карточка плана картинкой",
}

// problem explains why a request could not be read: what is missing, the
// words that were not understood, and how to write it.
func (t *texts) problem(err error, kind plan.Kind, unknown []string, usage string) string {
	reason := t.implausible
	switch {
	case errors.Is(err, plan.ErrNoDistance):
		reason = t.noDistance
	case errors.Is(err, plan.ErrNoTime) && kind == plan.Pace:
		reason = t.noPace
	case errors.Is(err, plan.ErrNoTime):
		reason = t.noTime
	case errors.Is(err, plan.ErrAmbiguous):
		reason = t.ambiguous
	}

	lines := []string{reason}
	if len(unknown) > 0 {
		lines = append(lines, t.notUnderstood+strings.Join(unknown, ", "))
	}

	return strings.Join(append(lines, usage), "\n")
}

// distanceName names a race the way the page and the card do; the three keep
// their own copies because none of them can import another.
func (t *texts) distanceName(meters int) string {
	switch meters {
	case 21097:
		return t.half
	case 42195:
		return t.marathon
	}

	return t.kilometres(meters)
}

func (t *texts) kilometres(meters int) string {
	number := strconv.Itoa(meters / 1000)
	if rest := meters % 1000; rest != 0 {
		number += t.decimal + strings.TrimRight(fmt.Sprintf("%03d", rest), "0")
	}

	return number + " " + t.km
}

// publishCommands fills the command menu Telegram shows next to the input box.
// One menu per language: the menu without a language is what everyone but a
// Russian speaker sees.
func publishCommands(ctx context.Context, client *bot.Bot) error {
	menus := []struct {
		language string
		texts    *texts
	}{
		{texts: &englishTexts},
		{language: "ru", texts: &russianTexts},
	}

	for _, menu := range menus {
		_, err := client.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			LanguageCode: menu.language,
			Commands: []models.BotCommand{
				{Command: "start", Description: menu.texts.startCommand},
				{Command: "time", Description: menu.texts.timeCommand},
				{Command: "pace", Description: menu.texts.paceCommand},
				{Command: "card", Description: menu.texts.cardCommand},
			},
		})
		if err != nil {
			return fmt.Errorf("set command menu for language %q: %w", menu.language, err)
		}
	}

	return nil
}

// textsFor picks the reply language from the sender's Telegram language_code.
// Messages without a sender are answered in English.
func textsFor(message *models.Message) *texts {
	if message == nil || message.From == nil {
		return &englishTexts
	}

	return textsForLanguage(message.From.LanguageCode)
}

// textsForLanguage reads an IETF tag such as "ru" or "pt-br": Russian goes to
// Russian speakers, English to everyone else.
func textsForLanguage(code string) *texts {
	base, _, _ := strings.Cut(strings.ToLower(code), "-")
	if base == "ru" {
		return &russianTexts
	}

	return &englishTexts
}
