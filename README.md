# GSCALE-ZEBRA

Scale + Zebra RFID + Telegram Bot + ERPNext integratsiyasi uchun amaliy ishlab chiqilgan ko'p-modulli (Go workspace) tizim.
Ushbu lokal nusxa hozir `gscale-zebra` remote'iga ulangan holda tekshirilmoqda.

## Annotatsiya
Ushbu loyiha ishlab chiqarish yoki ombor sharoitida real tarozi o'qishini RFID markirovka va ERP hujjatlashtirish bilan bir zanjirga birlashtiradi. Tizimning asosiy g'oyasi: fizik o'lchovdan (qty) boshlab, ERPNext'da `Material Receipt` draft yaratish, so'ng aynan shu EPC bilan Zebra orqali chop etish va yakunda hujjatni submit qilishgacha bo'lgan oqimni holatga asoslangan ishonchli pipeline ko'rinishida ishlatish.

Loyiha institut amaliy ishi uchun mos ravishda quyidagilarni namoyish etadi:
- real vaqtli signal oqimini qayta ishlash;
- barqarorlikni aniqlash (stable-detection) asosida trigger mexanizmi;
- ko'p jarayonli tizimda umumiy holatni atomar yangilash;
- tashqi servislar (Telegram, ERP API, Zebra USB) bilan integratsiya;
- Linux xizmat (systemd) sifatida ekspluatatsiya.

## Mundarija
- [1. Muammo va maqsad](#1-muammo-va-maqsad)
- [2. Tizim arxitekturasi](#2-tizim-arxitekturasi)
- [3. Modullar tavsifi](#3-modullar-tavsifi)
- [4. Ma'lumotlar modeli (bridge state)](#4-malumotlar-modeli-bridge-state)
- [5. Asosiy algoritmlar](#5-asosiy-algoritmlar)
- [6. Ishlash oqimi](#6-ishlash-oqimi)
- [7. O'rnatish va ishga tushirish](#7-ornatish-va-ishga-tushirish)
- [8. Konfiguratsiya](#8-konfiguratsiya)
- [9. Buyruqlar va boshqaruv](#9-buyruqlar-va-boshqaruv)
- [10. Loglash, monitoring va xatoliklar](#10-loglash-monitoring-va-xatoliklar)
- [11. Test va verifikatsiya](#11-test-va-verifikatsiya)
- [12. Cheklovlar va rivojlantirish yo'nalishlari](#12-cheklovlar-va-rivojlantirish-yonalishlari)

## 1. Muammo va maqsad
### Muammo
Amaliy jarayonda quyidagi ajralgan bosqichlar mavjud bo'ladi:
1. Tarozida mahsulot og'irligini o'qish.
2. RFID tegga EPC yozish va uni tekshirish.
3. ERP tizimida operatsion hujjat (draft) yaratish.

Bu bosqichlar alohida-alohida bajarilganda inson xatosi, vaqt yo'qotilishi va tracing qiyinligi ortadi.

### Maqsad
Bitta integrallashgan tizim yaratish:
- Scale o'qish barqaror bo'lganda yangi cycle'ni aniqlash;
- Zebra natijasini status bilan kuzatish;
- Batch holatiga qarab ERP draft yaratishni boshqarish;
- Barcha komponentlar holatini bitta umumiy state faylga jamlash.

## 2. Tizim arxitekturasi
Loyiha `go.work` asosida 5 asosiy moduldan tashkil topgan:

- `scale`: real-time worker, scale va Zebra monitor orchestration.
- `bot`: Telegram bot, ERP integratsiyasi, batch session boshqaruvi.
- `bridge`: umumiy state (`JSON`) uchun atomar store.
- `core`: barqaror qty asosida EPC trigger logikasi.
- `zebra`: diagnostika va servis komandalar uchun CLI utilita.

### Yuksak darajadagi oqim
```text
[Scale serial/bridge] -> [scale worker] -> [bridge_state.json] <- [bot worker]
                                 |                      |
                                 v                      v
                           [Zebra USB I/O]         [ERPNext API]
                                 |
                                 v
                            [EPC / VERIFY]
```

### Dizayn qarorlari
- Modullarni ajratish: testlash va ekspluatatsiyani soddalashtiradi.
- Shared-state fayl yondashuvi: komponentlar orasida bo'sh bog'lanish (loose coupling).
- File-lock + atomic rename: race condition va yarim yozilgan fayl xavfini pasaytiradi.

## 3. Modullar tavsifi
### `scale` moduli
Asosiy vazifalar:
- serial portni auto-detect qilish (`/dev/serial/by-id/*`, `ttyUSB*`, `ttyACM*`);
- scale frame parsing (`kg/g/lb/oz`, minus formatlar, stable/unstable markerlar);
- serial ishlamasa HTTP bridge fallback o'qish;
- Zebra holatini polling qilish;
- headless worker rejimi orqali operator oqimini yuritish;
- bridge state'ga `scale` va `zebra` snapshot yozish;
- bridge state'dagi `print_request` buyruqlarini kuzatish va printerga ijro qilish.

### `bot` moduli
Asosiy vazifalar:
- Telegram komandalarni qabul qilish: `/start`, `/batch`, `/log`, `/epc`;
- item/warehouse inline-qidiruv orqali ERP'dan tanlash;
- batch session ochish/yopish (`Material Receipt` tugmasi orqali);
- bridge state'dan stable qty ni kutish;
- ERPNext'da `Stock Entry` (`Material Receipt`) draft yaratish;
- `print_request` orqali worker'ga chop etish buyruq yuborish;
- print natijasiga qarab draft'ni submit yoki delete qilish;
- ish jarayonida ishlatilgan EPC'lar tarixini saqlash va `.txt` ko'rinishida yuborish (`/epc`).

### `bridge` moduli
Asosiy vazifalar:
- `bridge_state.json` ni yagona manba sifatida saqlash;
- lock-fayl bilan eksklyuziv yozish;
- `tmp` faylga yozib `rename` qilish orqali atomar update.

### `core` moduli
Asosiy vazifalar:
- umumiy EPC generatsiya logikasini saqlash;
- har bir yangi sikl uchun unikal 24-hex EPC hosil qilish.

### `zebra` moduli
Asosiy vazifalar:
- printer topish, status va SGD sozlamalarini tekshirish;
- RFID encode/read testlari;
- kalibratsiya va self-check.

## 4. Ma'lumotlar modeli (bridge state)
Default fayl:
- `/tmp/gscale-zebra/bridge_state.json`

Snapshot 4 asosiy bo'limdan iborat:
- `scale`: source, port, weight, unit, stable, error, updated_at
- `zebra`: connected, device state, media state, last_epc, verify, action, error, updated_at
- `batch`: active, chat_id, item_code, item_name, warehouse, updated_at
- `print_request`: epc, qty, unit, item_code, item_name, status, error, requested_at, updated_at

Namuna:
```json
{
  "scale": {
    "source": "serial",
    "port": "/dev/ttyUSB0",
    "weight": 1.25,
    "unit": "kg",
    "stable": true,
    "updated_at": "2026-02-20T10:10:10.123Z"
  },
  "zebra": {
    "connected": true,
    "device_path": "/dev/usb/lp0",
    "last_epc": "3034257BF7194E406994036B",
    "verify": "MATCH",
    "action": "encode",
    "updated_at": "2026-02-20T10:10:10.456Z"
  },
  "batch": {
    "active": true,
    "chat_id": 123456789,
    "item_code": "ITEM-001",
    "item_name": "Green Tea",
    "warehouse": "Stores - A",
    "updated_at": "2026-02-20T10:10:09.999Z"
  },
  "updated_at": "2026-02-20T10:10:10.500Z"
}
```

## 5. Asosiy algoritmlar
### 5.1 Stable qty cycle detection
Parametrlar:
- `StableFor = 1s`
- `Epsilon = 0.005`
- `MinWeight = 0.0`

Qoidalar:
1. `weight <= 0` yoki invalid qiymat bo'lsa holat reset bo'lishi mumkin, lekin `0` yangi cycle uchun majburiy shart emas.
2. Oqim bir nuqtada `StableFor` davomida `|w - candidate| <= Epsilon` bo'lsa stable nuqta qabul qilinadi.
3. Yangi cycle faqat oldingi stable holatdan keyin haqiqiy `movement/unstable` faza kuzatilganda ochiladi.
4. Keyingi stable nuqta oldingisidan katta, kichik yoki hatto deyarli teng bo'lishi mumkin.
5. Ma'no jihatdan model `stable -> movement -> next stable` ko'rinishida ishlaydi.

### 5.2 EPC generatsiya
24 belgilik uppercase hex format:
- prefiks: `30`
- vaqtga bog'liq qism: `unix nano`dan hosil qilingan 56-bit bo'lak
- tail: vaqt + seq + process salt aralashmasi

Natija:
- tez-tez triggerlarda ham collision ehtimoli juda past;
- restartdan keyin ham salt sababli farqlanish saqlanadi.

### 5.3 Zebra encode va verify
`scale` ichida encode oqimi:
1. RFID ultra settings qo'llanadi (`rfid.enable`, power, tries, va boshqalar).
2. ZPL stream bilan EPC yozish (`^RFW,H,,,A`).
3. `rfid.error.response` sampling orqali `WRITTEN/NO TAG/ERROR/UNKNOWN` infer.
4. Kerak bo'lsa readback bilan qo'shimcha tekshiruv.
5. `verify` va `last_epc` bridge state'ga yoziladi.

`verify` muvaffaqiyat qiymatlari:
- `MATCH`, `OK`, `WRITTEN`

### 5.4 Botdagi draft yaratish mezoni
Bot batch loop'i:
1. bridge state'dan stable musbat qty kutadi.
2. shu cycle uchun EPC generatsiya qiladi.
3. EPC bilan ERP `Material Receipt` draft yaratadi.
4. `print_request` orqali worker'ga aynan shu EPC bilan chop etish buyruq yuboradi.
5. print muvaffaqiyatli bo'lsa draft submit qilinadi.
6. print muvaffaqiyatsiz bo'lsa draft delete qilinadi.

Eslatma:
- duplicate barcode aniqlansa, final printdan oldin yangi candidate EPC bilan qayta urinish qilinadi.

## 6. Ishlash oqimi
### 6.1 End-to-end batch ketma-ketligi
```text
Operator -> Telegram: /batch
Bot -> ERP: item/warehouse qidiruv
Operator -> Bot: Material Receipt
Bot -> bridge_state: batch.active=true
Scale -> bridge_state: live qty/stable
Bot <- bridge_state: stable qty
Bot -> ERP: Stock Entry (Material Receipt) draft
Bot -> bridge_state: print_request pending
Scale <- bridge_state: print_request
Scale -> Zebra: EPC encode/print
Scale -> bridge_state: print_request done/error + zebra status
Bot <- bridge_state: print result
Bot -> ERP: submit (success) / delete (error)
Bot -> Telegram: status update
```

### 6.2 Qo'shimcha servis komandalar
- `/log`: `logs/bot` va `logs/scale` fayllarini Telegramga document qilib yuboradi.
- `/epc`: joriy bot session davomida draftlarda ishlatilgan barcha EPC'larni `.txt` fayl qilib yuboradi.

## 7. O'rnatish va ishga tushirish
### 7.1 Talablar
- Linux (Ubuntu/Arch sinovdan o'tgan)
- Go `1.25`
- USB serial scale qurilmasi
- Zebra USB LP printer (`/dev/usb/lp*`)
- Telegram Bot token
- ERPNext API key/secret

### 7.2 Development rejimi
Repo root'da:
```bash
make build
make test
```

Scale + auto bot:
```bash
make run SCALE_DEVICE=/dev/ttyUSB0 ZEBRA_DEVICE=/dev/usb/lp0
```

Faqat scale:
```bash
make run-scale SCALE_DEVICE=/dev/ttyUSB0 ZEBRA_DEVICE=/dev/usb/lp0
```

Faqat bot:
```bash
cd bot
cp .env.example .env
# tokenni to'ldiring
cp config/core.env.example config/core.env
# core ERP credentials ni config/core.env ichida to'ldiring
go run ./cmd/bot
```

### 7.3 Systemd autostart (repo rejimi)
```bash
make run
# yoki
make autostart-install
make autostart-status
```

`make run` endi persistent service rejimi: `scale + mobileapi` ni systemd orqali
install qiladi, start qiladi va reboot'dan keyin ham avtomatik ko'taradi.

Bot kerak bo'lsa:

```bash
make autostart-install-bot
```

Foreground rejim kerak bo'lsa:

```bash
make run-foreground
```

### 7.4 Release paket
```bash
make release
# yoki
make release-all
```

Natija `dist/` ichida Linux tarball ko'rinishida hosil bo'ladi.

## 8. Konfiguratsiya
### 8.1 Bot (`bot/.env`)
Majburiy:
- `TELEGRAM_BOT_TOKEN`

### 8.2 Core (`config/core.env`)
Majburiy:
- `ERP_URL`
- `ERP_API_KEY`
- `ERP_API_SECRET`

Ixtiyoriy:
- `ERP_READ_URL`
- `BRIDGE_STATE_FILE` (default: `/tmp/gscale-zebra/bridge_state.json`)

### 8.3 Scale (`flags`)
Asosiy flaglar:
- `--device`, `--baud`, `--baud-list`
- `--bridge-url`, `--bridge-interval`, `--no-bridge`
- `--zebra-device`, `--zebra-interval`, `--no-zebra`
- `--bot-dir`, `--no-bot`
- `--bridge-state-file`

### 8.4 Deploy env (systemd)
`deploy/config/scale.env.example`:
- `SCALE_DEVICE`
- `ZEBRA_DEVICE`
- `BRIDGE_STATE_FILE`

`deploy/config/bot.env.example`:
- `TELEGRAM_BOT_TOKEN`

`deploy/config/core.env.example`:
- `ERP_URL`
- `ERP_READ_URL`
- `ERP_API_KEY`
- `ERP_API_SECRET`
- `BRIDGE_STATE_FILE`

`deploy/config/mobileapi.env.example`:
- `MOBILE_API_ADDR`
- `MOBILE_API_CANDIDATE_PORTS`
- `MOBILE_API_SERVER_NAME`

Bu fayl ixtiyoriy. `mobileapi` default qiymatlar bilan ham ishga tushadi.

## 9. Buyruqlar va boshqaruv
### 9.1 Make targetlar
- `make run`: persistent systemd stack (scale + mobileapi)
- `make run-foreground`: scale worker + mobileapi sidecar (foreground, botsiz)
- `make run-scale`: faqat scale
- `make run-bot`: faqat bot
- `make test`: barcha modul testlari
- `make autostart-install|status|restart|stop` - scale + mobileapi systemd stack
- `make autostart-install-bot` - botni ham systemd stackka qo'shadi

### 9.2 Bot komandalar
- `/start`: ERP ulanish tekshiruvi
- `/batch`: batch tanlash va ishga tushirish oqimi
- `/log`: workflow log fayllarini yuborish
- `/epc`: session bo'yicha EPC ro'yxatini `.txt` yuborish

### 9.3 Scale worker
Scale endi terminal TUI’siz ishlaydi; operator interfeysi mobile app yoki bot orqali boshqariladi.

### 9.4 Zebra utilita
```bash
cd zebra
go run . help
```
Asosiy komandalar:
- `list`, `status`, `settings`, `setvar`, `raw-getvar`
- `print-test`, `epc-test`, `read-epc`, `calibrate`, `self-check`

## 10. Loglash, monitoring va xatoliklar
Log papkalar:
- `logs/scale/`
- `logs/bot/`

Muhim xususiyat:
- har process restart bo'lganda o'z log papkasini tozalab, yangi sessiya ochadi.

### Tipik xatoliklar
1. `device busy`:
- odatda printer portini boshqa process band qilgan bo'ladi.

2. `serial device topilmadi`:
- `SCALE_DEVICE` ni aniq berish yoki udev orqali portni tekshirish kerak.

3. `ERP auth/http xato`:
- `.env` dagi URL, key, secret qiymatlarini tekshirish zarur.

4. `epc timeout`:
- Zebra kechikishi yoki state update timing sabab bo'lishi mumkin.

## 11. Test va verifikatsiya
Loyihada unit testlar mavjud:
- `core`: stable detector va EPC uniqueness
- `bridge`: store update/read atomarligi
- `scale`: parser, frame parsing, zebra stream building
- `bot`: command parsing, ERP payload, log discovery, EPC history

Joriy holatda barcha testlar o'tadi:
```bash
make test
```

## 12. Cheklovlar va rivojlantirish yo'nalishlari
### Amaldagi cheklovlar
- asosiy target platforma Linux;
- `Receipt` flow callback hozir placeholder holatda;
- EPC history (`/epc`) hozircha process-memory'da (restartda tozalanadi);
- draft yaratishda `verify` muvaffaqiyatsiz bo'lsa ham EPC bo'lsa draft yaratiladi.

### Tavsiya etilgan keyingi ishlar
1. `verify` bo'yicha qat'iy policy qo'shish (`strict/lenient` mode).
2. `/epc` tarixini persistent storage'ga (SQLite/append-only log) ko'chirish.
3. Bridge state uchun schema-versioning va migratsiya.
4. Metrics (Prometheus) va health endpoint qo'shish.
5. E2E integration test (mock ERP + mock bridge + replay traces).

---

Agar bu README'ni diplom/amaliy ish formatiga to'liq moslashtirish kerak bo'lsa, keyingi bosqichda men siz uchun qo'shimcha `docs/` paketini ham tayyorlab bera olaman:
- `docs/architecture.md`
- `docs/algorithm.md`
- `docs/experiment-results.md`
- `docs/appendix-api-contracts.md`
