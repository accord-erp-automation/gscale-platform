package app

import (
	"context"
	"time"

	"bot/internal/app/commands"
	"bot/internal/telegram"
)

func (a *App) Run(ctx context.Context) error {
	a.logRun.Printf("bot started, ERP=%s ERP_READ=%s bridge_state=%s", a.cfg.ERPURL, a.cfg.ERPReadURL, a.cfg.BridgeStateFile)
	a.control.StopAll()
	defer a.control.StopAll()
	var offset int64

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := a.tg.GetUpdates(ctx, offset, 55)
		if err != nil {
			a.logRun.Printf("getUpdates error: %v", err)
			time.Sleep(1200 * time.Millisecond)
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			a.handleUpdate(ctx, upd)
		}
	}
}

func (a *App) handleUpdate(ctx context.Context, upd telegram.Update) {
	if upd.InlineQuery != nil {
		if err := commands.HandleInlineQuery(ctx, a.deps(), *upd.InlineQuery); err != nil {
			a.logCallback.Printf("handleInlineQuery error: %v", err)
		}
		return
	}

	if upd.CallbackQuery != nil {
		if err := a.handleCallbackQuery(ctx, *upd.CallbackQuery); err != nil {
			a.logCallback.Printf("handleCallbackQuery error: %v", err)
		}
		return
	}

	if upd.Message == nil {
		return
	}
	if err := a.handleMessage(ctx, *upd.Message); err != nil {
		a.logCallback.Printf("handleMessage error: %v", err)
	}
}
