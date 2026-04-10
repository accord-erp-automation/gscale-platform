package app

import (
	"context"
	"fmt"
	"strings"

	"bot/internal/app/commands"
	"bot/internal/telegram"
	"core/workflow"
)

func (a *App) handleCallbackQuery(ctx context.Context, q telegram.CallbackQuery) error {
	data := strings.TrimSpace(q.Data)
	switch data {
	case commands.StockEntryCallbackMaterialReceipt:
		return a.handleMaterialReceiptCallback(ctx, q)
	case commands.StockEntryCallbackBatchChangeItem:
		return a.handleBatchChangeItemCallback(ctx, q)
	case commands.StockEntryCallbackBatchStart:
		return a.handleBatchStartCallback(ctx, q)
	case commands.StockEntryCallbackBatchStop:
		return a.handleBatchStopCallback(ctx, q)
	default:
		return a.tg.AnswerCallbackQuery(ctx, q.ID, "")
	}
}

func (a *App) handleMaterialReceiptCallback(ctx context.Context, q telegram.CallbackQuery) error {
	return a.tg.AnswerCallbackQuery(ctx, q.ID, "Material Receipt batchni boshlamaydi. Batch Start ni bosing.")
}

func (a *App) handleBatchChangeItemCallback(ctx context.Context, q telegram.CallbackQuery) error {
	if q.Message == nil || q.Message.Chat.ID == 0 {
		return a.tg.AnswerCallbackQuery(ctx, q.ID, "Item almashtirish")
	}

	chatID := q.Message.Chat.ID
	_ = a.control.Stop(chatID)
	a.setBatchChangePending(chatID, q.Message.MessageID)

	pausedText := formatPausedStatus(q.Message.Text)
	if err := a.tg.EditMessageText(ctx, chatID, q.Message.MessageID, pausedText, commands.BuildBatchControlKeyboard()); err != nil && !isMessageNotModifiedError(err) {
		a.logCallback.Printf("edit paused status warning: %v", err)
	}

	promptID, err := commands.HandleBatch(ctx, a.deps(), telegram.Message{Chat: telegram.Chat{ID: chatID}})
	if err != nil {
		if cbErr := a.tg.AnswerCallbackQuery(ctx, q.ID, "Pause qilindi, lekin item tanlashda xato"); cbErr != nil {
			return cbErr
		}
		return err
	}
	a.trackBatchPromptMessage(ctx, chatID, promptID)
	a.deleteTrackedWarehousePromptMessage(ctx, chatID)

	return a.tg.AnswerCallbackQuery(ctx, q.ID, "Batch pause. Yangi item tanlang")
}

func (a *App) handleBatchStopCallback(ctx context.Context, q telegram.CallbackQuery) error {
	if q.Message == nil || q.Message.Chat.ID == 0 {
		return a.tg.AnswerCallbackQuery(ctx, q.ID, "Batch to'xtatildi")
	}

	chatID := q.Message.Chat.ID
	a.clearBatchChangePending(chatID)
	stopped := a.control.Stop(chatID)
	if stopped {
		if err := a.tg.AnswerCallbackQuery(ctx, q.ID, "Batch to'xtatildi"); err != nil {
			return err
		}
		stoppedText := formatStoppedStatus(q.Message.Text)
		if err := a.tg.EditMessageText(ctx, chatID, q.Message.MessageID, stoppedText, commands.BuildBatchControlKeyboard()); err != nil && !isMessageNotModifiedError(err) {
			a.logCallback.Printf("edit stopped status warning: %v", err)
		}
		return nil
	}

	return a.tg.AnswerCallbackQuery(ctx, q.ID, "Batch allaqachon to'xtagan")
}

func (a *App) handleBatchStartCallback(ctx context.Context, q telegram.CallbackQuery) error {
	if q.Message == nil || q.Message.Chat.ID == 0 {
		return a.tg.AnswerCallbackQuery(ctx, q.ID, "Batch boshlandi")
	}

	chatID := q.Message.Chat.ID
	sel, ok := a.getSelection(chatID)
	if !ok {
		if err := a.tg.AnswerCallbackQuery(ctx, q.ID, "Avval item va ombor tanlang"); err != nil {
			return err
		}
		return a.tg.SendMessage(ctx, chatID, "Avval /batch orqali item va ombor tanlang.")
	}
	if a.control.HasActiveBatch(chatID) {
		return a.tg.AnswerCallbackQuery(ctx, q.ID, "Batch allaqachon ishlayapti")
	}
	if _, ok := a.control.OtherActiveBatchOwner(chatID); ok {
		return a.tg.AnswerCallbackQuery(ctx, q.ID, "Batch boshqa chatda ishlayapti")
	}

	a.clearBatchChangePending(chatID)
	_ = a.startMaterialReceiptBatch(ctx, chatID, sel, q.Message.MessageID, "Batch qayta boshlandi: scale qty kutilmoqda...")
	return a.tg.AnswerCallbackQuery(ctx, q.ID, "Batch qayta boshlandi")
}

func (a *App) startMaterialReceiptBatch(ctx context.Context, chatID int64, sel SelectedContext, statusMessageID int64, note string) int64 {
	initial := formatBatchStatusText(sel, 0, "", 0, "", "", "", strings.TrimSpace(note))
	statusMessageID = a.upsertBatchStatusMessage(ctx, chatID, statusMessageID, initial)

	a.control.Start(ctx, chatID, toWorkflowSelection(sel), workflow.Hooks{
		Progress: func(progress workflow.Progress) {
			statusMessageID = a.upsertBatchStatusMessage(
				ctx,
				chatID,
				statusMessageID,
				formatBatchWorkflowProgress(progress),
			)
		},
	})
	return statusMessageID
}

func (a *App) upsertBatchStatusMessage(ctx context.Context, chatID, messageID int64, text string) int64 {
	if messageID > 0 {
		err := a.tg.EditMessageText(ctx, chatID, messageID, text, commands.BuildBatchControlKeyboard())
		if err == nil || isMessageNotModifiedError(err) {
			return messageID
		}
		a.logCallback.Printf("edit batch status warning: %v", err)
	}

	newID, err := a.tg.SendMessageWithInlineKeyboardAndReturnID(ctx, chatID, text, commands.BuildBatchControlKeyboard())
	if err != nil {
		a.logCallback.Printf("send batch status warning: %v", err)
		return messageID
	}
	return newID
}

func formatBatchStatusText(sel SelectedContext, draftCount int, draftName string, qty float64, unit, epc, epcVerify, note string) string {
	lines := []string{
		"Batch ishlayapti",
		fmt.Sprintf("Item: %s", formatSelectedItem(sel)),
		fmt.Sprintf("Ombor: %s", strings.TrimSpace(sel.Warehouse)),
		fmt.Sprintf("Draftlar: %d", draftCount),
	}

	if draftCount > 0 {
		u := strings.ToLower(strings.TrimSpace(unit))
		if u == "" {
			u = "kg"
		}
		lines = append(lines, fmt.Sprintf("Oxirgi draft: %s", strings.TrimSpace(draftName)))
		lines = append(lines, fmt.Sprintf("Oxirgi QTY: %.3f %s", qty, u))
		epc = strings.ToUpper(strings.TrimSpace(epc))
		if epc == "" {
			epc = "-"
		}
		lines = append(lines, "Oxirgi EPC: "+epc)
		lines = append(lines, formatRFIDConfirmLine(epc, epcVerify))
	}

	note = strings.TrimSpace(note)
	if note != "" {
		lines = append(lines, "Holat: "+note)
	}

	return strings.Join(lines, "\n")
}

func formatRFIDConfirmLine(epc, verify string) string {
	verify = strings.ToUpper(strings.TrimSpace(verify))
	if verify == "" {
		verify = "UNKNOWN"
	}
	if strings.TrimSpace(epc) == "" {
		return fmt.Sprintf("RFID holat: EPC yo'q (VERIFY=%s)", verify)
	}
	if verify == "PENDING" {
		return fmt.Sprintf("RFID holat: chop etish navbatda (VERIFY=%s)", verify)
	}
	if !isRFIDVerifySuccess(verify) {
		return fmt.Sprintf("RFID holat: yozish tasdiqlanmadi (VERIFY=%s)", verify)
	}
	return fmt.Sprintf("RFID holat: yozish tasdiqlandi (VERIFY=%s)", verify)
}

func isRFIDVerifySuccess(verify string) bool {
	switch strings.ToUpper(strings.TrimSpace(verify)) {
	case "MATCH", "OK", "WRITTEN":
		return true
	default:
		return false
	}
}

func formatSelectedItem(sel SelectedContext) string {
	code := strings.TrimSpace(sel.ItemCode)
	name := strings.TrimSpace(sel.ItemName)
	if name == "" {
		name = code
	}
	if code == "" {
		return "-"
	}
	if strings.EqualFold(name, code) {
		return code
	}
	return name + " (" + code + ")"
}

func formatPausedStatus(current string) string {
	base := strings.TrimSpace(current)
	if base == "" {
		return "Batch pausa qilindi. Yangi item tanlang."
	}
	if strings.Contains(strings.ToUpper(base), "PAUSE") {
		return base
	}
	return base + "\n\nStatus: PAUSE (yangi item tanlanmoqda)"
}

func formatStoppedStatus(current string) string {
	base := strings.TrimSpace(current)
	if base == "" {
		base = "Batch"
	}
	if strings.Contains(strings.ToUpper(base), "TO'XTATILDI") {
		return base
	}
	return base + "\n\nStatus: TO'XTATILDI"
}

func isMessageNotModifiedError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "message is not modified")
}
