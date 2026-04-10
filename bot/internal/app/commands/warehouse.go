package commands

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"bot/internal/telegram"
	"core/batchcontrol"
)

const warehouseInlinePrefix = "wh:"

const warehouseDefaultQuery = "*"

type warehouseQueryRequest struct {
	ItemCode string
	Query    string
}

func ExtractSelectedItem(text string) (string, string, bool) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return "", "", false
	}

	var code string
	var name string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "item:") {
			code = strings.TrimSpace(line[len("item:"):])
			continue
		}
		if strings.HasPrefix(lower, "nomi:") {
			name = strings.TrimSpace(line[len("nomi:"):])
		}
	}

	if code == "" {
		return "", "", false
	}
	if name == "" {
		name = code
	}
	return code, name, true
}

func ExtractSelectedItemCode(text string) (string, bool) {
	code, _, ok := ExtractSelectedItem(text)
	return code, ok
}

func HandleItemSelected(ctx context.Context, deps Deps, chatID int64, itemCode, itemName string) (int64, error) {
	itemCode = strings.TrimSpace(itemCode)
	itemName = strings.TrimSpace(itemName)
	if itemCode == "" {
		return 0, nil
	}
	if itemName == "" {
		itemName = itemCode
	}

	keyboard := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "Ombor tanlash", SwitchInlineQueryCurrentChat: buildWarehouseInlineSeed(itemCode)},
			},
		},
	}

	text := fmt.Sprintf("Item tanlandi: %s\nKod: %s\nEndi pastdagi tugmani bosib omborni tanlang.", itemName, itemCode)
	messageID, err := deps.TG.SendMessageWithInlineKeyboardAndReturnID(ctx, chatID, text, keyboard)
	if err == nil {
		return messageID, nil
	}

	if isInlineButtonUnsupported(err) {
		return 0, deps.TG.SendMessage(ctx, chatID, "Inline mode o'chirilgan. BotFather'da /setinline ni yoqing, keyin /batch ni qayta bering.")
	}

	return 0, err
}

func HandleWarehouseInlineQuery(ctx context.Context, deps Deps, q telegram.InlineQuery, request warehouseQueryRequest) error {
	stocks, err := deps.Control.SearchItemWarehouses(ctx, request.ItemCode, request.Query, 50)
	if err != nil {
		results := []telegram.InlineQueryResultArticle{
			{
				Type:        "article",
				ID:          "erp-wh-error",
				Title:       "Omborlarni olishda xato",
				Description: "ERP'dan ombor ro'yxati kelmadi",
				InputMessageContent: telegram.InputTextMessageContent{
					MessageText: "Ombor ro'yxatini olib bo'lmadi. Keyinroq qayta urinib ko'ring.",
				},
			},
		}
		return deps.TG.AnswerInlineQuery(ctx, q.ID, results, 1)
	}

	results := buildWarehouseResults(request.ItemCode, stocks)
	if len(results) == 0 {
		results = []telegram.InlineQueryResultArticle{
			{
				Type:        "article",
				ID:          "wh-empty",
				Title:       "Ombor topilmadi",
				Description: "Bu item uchun ombor yo'q yoki qoldiq 0",
				InputMessageContent: telegram.InputTextMessageContent{
					MessageText: "Ombor topilmadi",
				},
			},
		}
	}

	return deps.TG.AnswerInlineQuery(ctx, q.ID, results, 1)
}

func parseWarehouseInlineQuery(raw string) (warehouseQueryRequest, bool) {
	q := strings.TrimSpace(raw)
	if !strings.HasPrefix(q, warehouseInlinePrefix) {
		return warehouseQueryRequest{}, false
	}

	payload := strings.TrimSpace(strings.TrimPrefix(q, warehouseInlinePrefix))
	parts := strings.SplitN(payload, ":", 2)
	if len(parts) != 2 {
		return warehouseQueryRequest{}, false
	}

	encodedCode := strings.TrimSpace(parts[0])
	if encodedCode == "" {
		return warehouseQueryRequest{}, false
	}

	decoded, err := base64.RawURLEncoding.DecodeString(encodedCode)
	if err != nil {
		return warehouseQueryRequest{}, false
	}

	itemCode := strings.TrimSpace(string(decoded))
	if itemCode == "" {
		return warehouseQueryRequest{}, false
	}

	query := strings.TrimSpace(parts[1])
	if query == warehouseDefaultQuery {
		query = ""
	}

	return warehouseQueryRequest{ItemCode: itemCode, Query: query}, true
}

func buildWarehouseInlineSeed(itemCode string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(itemCode)))
	return warehouseInlinePrefix + encoded + ":" + warehouseDefaultQuery
}

func buildWarehouseResults(itemCode string, stocks []batchcontrol.WarehouseStock) []telegram.InlineQueryResultArticle {
	results := make([]telegram.InlineQueryResultArticle, 0, len(stocks))
	for i, stock := range stocks {
		warehouse := strings.TrimSpace(stock.Warehouse)
		if warehouse == "" {
			continue
		}
		qtyLabel := strconv.FormatFloat(stock.ActualQty, 'f', 3, 64)

		results = append(results, telegram.InlineQueryResultArticle{
			Type:        "article",
			ID:          fmt.Sprintf("wh-%d-%s", i+1, warehouse),
			Title:       warehouse,
			Description: "Qoldiq: " + qtyLabel,
			InputMessageContent: telegram.InputTextMessageContent{
				MessageText: fmt.Sprintf("Item: %s\nOmbor: %s\nQoldiq: %s", itemCode, warehouse, qtyLabel),
			},
		})
	}
	return results
}
