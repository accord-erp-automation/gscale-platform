package commands

import (
	"context"

	"bot/internal/telegram"
	"core/batchcontrol"
)

type TelegramService interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	SendMessageWithInlineKeyboard(ctx context.Context, chatID int64, text string, keyboard *telegram.InlineKeyboardMarkup) error
	SendMessageWithInlineKeyboardAndReturnID(ctx context.Context, chatID int64, text string, keyboard *telegram.InlineKeyboardMarkup) (int64, error)
	AnswerInlineQuery(ctx context.Context, inlineQueryID string, results []telegram.InlineQueryResultArticle, cacheSeconds int) error
	AnswerCallbackQuery(ctx context.Context, callbackQueryID, text string) error
}

type ControlService interface {
	CheckConnection(ctx context.Context) (string, error)
	SearchItems(ctx context.Context, query string, limit int) ([]batchcontrol.Item, error)
	SearchItemWarehouses(ctx context.Context, itemCode, query string, limit int) ([]batchcontrol.WarehouseStock, error)
}

type Deps struct {
	TG      TelegramService
	Control ControlService
}
