# Scale monitor (headless + Zebra) 📟

`scale` moduli USB serial tarozi oqimini o'qiydi, Zebra holatini kuzatadi va bridge state orqali kelgan `print_request` buyruqlarini bajaradi.

## Ishga tushirish

```bash
cd /home/wikki/local.git/gscale-zebra/scale
go run .
```

`scale` hozir headless ishlaydi, shuning uchun terminal TUI tugmalari yo'q.

## Boot'da auto-start (systemd) 🚀

Repo root'dan:

```bash
cd /home/wikki/local.git/gscale-zebra
make autostart-install
```

Bu `scale` va `bot` service'larini systemd'ga o'rnatadi, enable qiladi va start beradi.

## Ishlash oqimi

1. Serial port auto-detect qilinadi (`/dev/serial/by-id/*`, `ttyUSB*`, `ttyACM*`).
2. Agar serial ishga tushmasa, HTTP bridge fallback ishlatiladi (`--bridge-url`).
3. Har reading bridge snapshot'ga yoziladi (`scale` + `zebra`).
4. `batch.active=true` bo'lsa ERP-first workflow ishlaydi, aks holda to'xtaydi.
5. Bot tomonidan yozilgan `print_request` topilganda Zebra encode/print command yuboriladi.

## Print request qoidalari (joriy kod)

- `print_request.status = pending` bo'lsa worker uni ko'radi.
- Aynan shu EPC allaqachon encode bo'lgan bo'lsa request `done` qilib yopiladi.
- Zebra o'chirilgan bo'lsa request `error` holatiga o'tadi.
- Encode ishlaganda request `processing` -> `done/error` oqimi bilan yuradi.

## Batch gate (`bridge_state.json`)

- default fayl: `/tmp/gscale-zebra/bridge_state.json`
- flag: `--bridge-state-file /tmp/gscale-zebra/bridge_state.json`

Bot tomonda:

- `Material Receipt` => `batch.active=true`
- `Batch Stop` => `batch.active=false`

Scale bu holatni log va bridge snapshot orqali ko'rsatadi.

## Bot auto-start

Default holatda scale ichidan bot ham ko'tariladi (`go run ./cmd/bot` in `--bot-dir`).

O'chirish:

```bash
go run . --no-bot
```

## Parametrlar

- `--device` (example: `/dev/ttyUSB0`) - serial device'ni qo'lda berish
- `--baud` (default: `9600`) - asosiy baud
- `--baud-list` (default: `9600,19200,38400,57600,115200`) - detect uchun baudlar
- `--probe-timeout` (default: `800ms`) - port probe timeout
- `--unit` (default: `kg`) - default birlik
- `--bridge-url` (default: `http://127.0.0.1:18000/api/v1/scale`) - fallback endpoint
- `--bridge-interval` (default: `120ms`) - fallback poll interval
- `--no-bridge` - HTTP fallback'ni o'chiradi
- `--zebra-device` (example: `/dev/usb/lp0`) - printer path
- `--zebra-interval` (default: `900ms`) - Zebra monitor interval
- `--no-zebra` - Zebra monitor va printer actionlarini o'chiradi
- `--bot-dir` (default: `../bot`) - bot modul yo'li
- `--no-bot` - bot auto-startni o'chiradi
- `--bridge-state-file` - shared snapshot fayli

## Loglar

Worker loglari `../logs/scale/` ichiga yoziladi.
Har restartda `logs/scale/` tozalanib, yangi sessiya boshlanadi.
