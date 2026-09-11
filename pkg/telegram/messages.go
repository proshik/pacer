package telegram

import (
	"strings"

	"github.com/go-telegram/bot/models"
)

// texts holds every reply that carries words. Results themselves are Go
// durations ("50m0s") and read the same in either language.
type texts struct {
	greeting       string
	unknownCommand string // %s is the command without its slash
	emptyArguments string
	timeArgCount   string
	paceArgCount   string
	invalidPace    string // each invalid* is followed by the offending argument
	invalidDist    string
	invalidTime    string
}

var englishTexts = texts{
	greeting: "Calculate Your Running Pace\n" +
		"Please, enter one of the following commands:\n\n" +
		"/time - calculate time, e.g. /time 4m50s 21095, where first - pace, second - distance\n" +
		"/pace - calculate pace, e.g. /pace 21097 1h38m48s, where first - distance, second - time\n",
	unknownCommand: "Unknown command: /%s\n\nAvailable commands: /start, /time, /pace",
	emptyArguments: "empty arguments",
	timeArgCount:   "should be 2 arguments: (pace, dist) separated by a space",
	paceArgCount:   "should be 2 arguments: (dist, time) separated by a space",
	invalidPace:    "invalid pace value: ",
	invalidDist:    "invalid dist value: ",
	invalidTime:    "invalid time value: ",
}

var russianTexts = texts{
	greeting: "Беговой калькулятор темпа\n" +
		"Отправьте одну из команд:\n\n" +
		"/time — время по темпу, например /time 4m50s 21095: сначала темп, потом дистанция в метрах\n" +
		"/pace — темп по времени, например /pace 21097 1h38m48s: сначала дистанция в метрах, потом время\n",
	unknownCommand: "Неизвестная команда: /%s\n\nДоступные команды: /start, /time, /pace",
	emptyArguments: "Команде нужны аргументы — примеры в /start",
	timeArgCount:   "Нужно два аргумента через пробел: темп и дистанция",
	paceArgCount:   "Нужно два аргумента через пробел: дистанция и время",
	invalidPace:    "Не получилось разобрать темп, пример — 4m50s: ",
	invalidDist:    "Дистанция — целое число метров, например 21095: ",
	invalidTime:    "Не получилось разобрать время, пример — 1h38m48s: ",
}

// textsFor picks the reply language from the sender's Telegram language_code,
// an IETF tag such as "ru" or "pt-br". Russian goes to Russian speakers,
// English to everyone else and to messages without a sender.
func textsFor(message *models.Message) *texts {
	if message == nil || message.From == nil {
		return &englishTexts
	}

	base, _, _ := strings.Cut(strings.ToLower(message.From.LanguageCode), "-")
	if base == "ru" {
		return &russianTexts
	}

	return &englishTexts
}
