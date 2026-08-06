package bot

import (
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"scribo/i18n"
)

// commandMenu is the list Telegram shows when the user types "/".
//
// It advertises exactly the names the greeting advertises — the language's own
// spelling — because a menu offering /last while the greeting says /son would
// reintroduce the very mismatch the aliases exist to remove. The other spelling
// keeps working; it is simply not listed.
func commandMenu() []tgbotapi.BotCommand {
	return []tgbotapi.BotCommand{
		{
			Command:     i18n.T("command.start.name"),
			Description: i18n.T("command.start.description"),
		},
		{
			Command:     i18n.T("command.last.name"),
			Description: i18n.T("command.last.description"),
		},
	}
}

// registerCommands publishes the menu to Telegram.
//
// Deliberately without a language_code: that field keys off the *user's*
// Telegram client language, while Scribo's language is SCRIBO_LANG, one setting
// for the whole process (K-11). Registering per-language lists would hand a
// Turkish-client user a Turkish menu from an English-configured bot and put the
// menu back out of step with every reply it sends.
//
// A failure is logged and swallowed. The menu is a convenience; refusing to
// start a working bot because Telegram would not take a cosmetic list would be
// the wrong trade.
func (b *BotRunner) registerCommands() {
	if _, err := b.api.Request(tgbotapi.NewSetMyCommands(commandMenu()...)); err != nil {
		slog.Warn("⚠️ could not register the command menu", "error", b.cfg.Redact(err.Error()))
		return
	}
	slog.Info("📋 command menu registered", "language", i18n.Language())
}
