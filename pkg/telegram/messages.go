package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
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

	// descriptions in the command menu next to the input box
	startCommand string
	timeCommand  string
	paceCommand  string
}

var englishTexts = texts{
	greeting: "Running pace calculator\n" +
		"Send one of these commands:\n\n" +
		"/time — time from pace, for example /time 4m50s 21095: the pace first, then the distance in meters\n" +
		"/pace — pace from time, for example /pace 21097 1h38m48s: the distance in meters first, then the time\n",
	unknownCommand: "Unknown command: /%s\n\nAvailable commands: /start, /time, /pace",
	emptyArguments: "This command needs arguments — /start has examples",
	timeArgCount:   "Two arguments separated by a space: the pace and the distance",
	paceArgCount:   "Two arguments separated by a space: the distance and the time",
	invalidPace:    "Couldn't read the pace, 4m50s is an example: ",
	invalidDist:    "The distance is a whole number of meters, 21095 for example: ",
	invalidTime:    "Couldn't read the time, 1h38m48s is an example: ",
	startCommand:   "How to use the bot",
	timeCommand:    "Time from pace and distance",
	paceCommand:    "Pace from distance and time",
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
	startCommand:   "Как пользоваться",
	timeCommand:    "Время по темпу и дистанции",
	paceCommand:    "Темп по дистанции и времени",
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
			},
		})
		if err != nil {
			return fmt.Errorf("set command menu for language %q: %w", menu.language, err)
		}
	}

	return nil
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
