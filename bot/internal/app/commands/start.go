package commands

import (
	"context"
	"fmt"
	"strings"

	"bot/internal/telegram"
)

func HandleStart(ctx context.Context, deps Deps, msg telegram.Message) (int64, error) {
	user, err := deps.Control.CheckConnection(ctx)
	if err != nil {
		return 0, deps.TG.SendMessage(ctx, msg.Chat.ID, "ERPNext ulanishi xato: "+err.Error())
	}

	info := buildStartInfo(user)

	return deps.TG.SendMessageWithInlineKeyboardAndReturnID(ctx, msg.Chat.ID, info, nil)
}

func buildStartInfo(user string) string {
	user = strings.TrimSpace(user)
	if user == "" {
		user = "-"
	}

	return strings.Join([]string{
		fmt.Sprintf("✅ ERPNext ga ulandi: %s", user),
		"",
		"🤖 Men scale + zebra workflow yordamchi botiman.",
		"",
		"📌 Asosiy buyruqlar:",
		"• /batch - batch oqimini boshlash",
		"• /log - bot va scale log fayllarini olish",
		"• /epc - session davomida draftlarda ishlatilgan EPC ro'yxatini .txt olish",
		"• /calibrate - zebra calibration yuborish",
		"",
		"🧾 Nechta draft EPC bilan ketganini ko'rish:",
		"1) /epc yuboring",
		"2) Bot .txt fayl yuboradi",
		"3) Fayl captionida umumiy son chiqadi (masalan: 12 ta)",
		"",
		"🚀 Boshlash uchun /batch ni bosing.",
	}, "\n")
}
