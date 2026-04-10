package commands

import (
	"context"
	"fmt"
	"strings"

	"bot/internal/telegram"
	"core/batchcontrol"
)

const inlineDefaultQuery = "*"

func HandleBatch(ctx context.Context, deps Deps, msg telegram.Message) (int64, error) {
	keyboard := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "Item tanlash", SwitchInlineQueryCurrentChat: inlineDefaultQuery},
			},
		},
	}

	messageID, err := deps.TG.SendMessageWithInlineKeyboardAndReturnID(ctx, msg.Chat.ID, "Item tanlang:", keyboard)
	if err == nil {
		return messageID, nil
	}

	if isInlineButtonUnsupported(err) {
		return 0, deps.TG.SendMessage(ctx, msg.Chat.ID, "Inline mode o'chirilgan. BotFather'da /setinline ni yoqing, keyin /batch ni qayta bering.")
	}

	return 0, err
}

func HandleInlineQuery(ctx context.Context, deps Deps, q telegram.InlineQuery) error {
	if request, ok := parseWarehouseInlineQuery(q.Query); ok {
		return HandleWarehouseInlineQuery(ctx, deps, q, request)
	}
	return HandleBatchInlineQuery(ctx, deps, q)
}

func HandleBatchInlineQuery(ctx context.Context, deps Deps, q telegram.InlineQuery) error {
	query := normalizeInlineQuery(q.Query)
	items, err := deps.Control.SearchItems(ctx, query, 50)
	if err != nil {
		results := []telegram.InlineQueryResultArticle{
			{
				Type:        "article",
				ID:          "erp-error",
				Title:       "ERP bilan ulanish xatosi",
				Description: "Item ro'yxatini olib bo'lmadi",
				InputMessageContent: telegram.InputTextMessageContent{
					MessageText: "ERP bilan ulanish xatosi. Keyinroq qayta urinib ko'ring.",
				},
			},
		}
		return deps.TG.AnswerInlineQuery(ctx, q.ID, results, 1)
	}

	results := buildItemResults(items)
	if len(results) == 0 {
		results = []telegram.InlineQueryResultArticle{
			{
				Type:        "article",
				ID:          "empty",
				Title:       "Item topilmadi",
				Description: "ERPNext'da item topilmadi",
				InputMessageContent: telegram.InputTextMessageContent{
					MessageText: "Item topilmadi",
				},
			},
		}
	}

	return deps.TG.AnswerInlineQuery(ctx, q.ID, results, 1)
}

func buildItemResults(items []batchcontrol.Item) []telegram.InlineQueryResultArticle {
	results := make([]telegram.InlineQueryResultArticle, 0, len(items))
	for i, it := range items {
		code := strings.TrimSpace(it.ItemCode)
		if code == "" {
			continue
		}
		name := strings.TrimSpace(it.ItemName)
		if name == "" {
			name = code
		}

		results = append(results, telegram.InlineQueryResultArticle{
			Type:        "article",
			ID:          fmt.Sprintf("%d-%s", i+1, code),
			Title:       name,
			Description: fmt.Sprintf("Kod: %s", code),
			InputMessageContent: telegram.InputTextMessageContent{
				MessageText: fmt.Sprintf("Item: %s\nNomi: %s", code, name),
			},
		})
	}
	return results
}

func normalizeInlineQuery(query string) string {
	q := strings.TrimSpace(query)
	if q == inlineDefaultQuery {
		return ""
	}
	return q
}

func isInlineButtonUnsupported(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "button_type_invalid") || strings.Contains(s, "inline mode")
}
