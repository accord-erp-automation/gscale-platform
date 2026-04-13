# Bot (Telegram + ERPNext) 🤖

`bot` moduli Telegram orqali batch workflow'ni boshqaradi va ERPNext'ga draft yaratadi.

## Ishga tushirish

```bash
cd /home/wikki/local.git/gscale-zebra/bot
cp .env.example .env
# .env ichiga real token va ERP credentials yozing
go run ./cmd/bot
```

## Qo'llab-quvvatlanadigan buyruqlar

- `/start` - ERPNext ulanishini tekshiradi va botni tayyor holatga olib keladi.
- `/batch` - batch oqimini boshlash uchun item/ombor tanlash jarayonini ochadi.
- `/log` - `logs/bot` va `logs/scale` fayllarini Telegram chatga yuboradi.
- `/epc` - bot ishga tushganidan beri draftlarda ishlatilgan EPC ro'yxatini `.txt` fayl qilib yuboradi.
- `/calibrate` - Zebra calibration yuboradi (`~JC` va default holatda save). Format: `/calibrate [--device /dev/usb/lp0] [--no-save] [--dry-run]`

## Batch workflow (hozirgi amaliy oqim) ✅

1. `/batch` beriladi.
2. `Item tanlash` inline tugmasi orqali ERP item tanlanadi.
3. `Ombor tanlash` inline tugmasi orqali warehouse tanlanadi.
4. Bot `Material Receipt` tugmasini ko'rsatadi.
5. `Material Receipt` bosilganda batch session ishga tushadi.
6. Scale'dan `stable + musbat qty` keladi.
7. Bot shu cycle uchun EPC generatsiya qiladi va ERPNext draft yaratadi.
8. Bot bridge state'ga `print_request` yozadi.
9. Scale worker print natijasini qaytaradi.
10. Print muvaffaqiyatli bo'lsa draft submit qilinadi, aks holda delete qilinadi.

## Batch boshqaruv tugmalari

- `Item almashtirish` - joriy batchni pause qiladi va yangi item tanlashga qaytaradi.
- `Batch Start` - tanlangan item/ombor bilan batchni qayta boshlaydi.
- `Batch Stop` - batchni to'xtatadi (`batch.active=false`).

## Bridge state

Shared snapshot fayl:

- default: `/tmp/gscale-zebra/bridge_state.json`
- config: `BRIDGE_STATE_FILE`

Bot `batch` va `print_request` bo'limlarini shu faylga yozadi, `scale` esa shu holatga qarab print command'larni ijro qiladi.

## Konfiguratsiya (`.env`)

Majburiy:

- `TELEGRAM_BOT_TOKEN`

Core runtime config alohida `../config/core.env` ichidan olinadi:

- `ERP_URL`
- `ERP_API_KEY`
- `ERP_API_SECRET`
- `ERP_READ_URL`
- `BRIDGE_STATE_FILE`

Ixtiyoriy/asosiy:

- `CORE_ENV_PATH` (agar `../config/core.env` o'rniga boshqa core config yo'lini ishlatmoqchi bo'lsangiz)

## Loglar

Ish jarayonida worker loglari `../logs/bot/` ichiga yoziladi.
Har restartda `logs/bot/` tozalanib, yangi sessiya boshlanadi.

## Eslatma

Inline qidiruv ishlashi uchun `@BotFather -> /setinline` yoqilgan bo'lishi kerak.
