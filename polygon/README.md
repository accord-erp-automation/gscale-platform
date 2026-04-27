# Polygon

`polygon` real scale yoki Zebra qurilmasiz ishlaydigan test muhiti.

Nimalarni beradi:
- fake `scale` oqimi;
- fake `zebra` holati;
- `bridge_state.json` ga live snapshot yozish;
- pending `print_request` ni `processing -> done/error` ga o'tkazish;
- virtual printerga kelgan buyruq preview va tarixini saqlash;
- HTTP endpointlar orqali state va qo'lda boshqaruv.
- `scenario` orqali `batch-flow`, `idle`, `stress`, `calibration` profillarini tanlash.
- scale va printer simulyatsiyasini alohida yoqib/o'chirish.

Ishga tushirish:

```bash
make run-polygon
```

Faqat scale simulyatsiya qilish, printer real bo'lganda:

```bash
cd polygon
make run NO_PRINTER_SIM=true
```

Yoki modul ichidan:

```bash
cd polygon
make run
```

Asosiy endpointlar:
- `GET /health`
- `GET /api/v1/scale`
- `GET /api/v1/state`
- `GET /api/v1/dev/printer`
- `POST /api/v1/dev/auto`
- `GET|POST /api/v1/dev/scenario`
- `POST /api/v1/dev/weight`
- `POST /api/v1/dev/reset`
- `POST /api/v1/dev/print-mode`

Misollar:

```bash
curl http://127.0.0.1:18000/api/v1/scale
curl http://127.0.0.1:18000/api/v1/dev/printer
curl -X POST http://127.0.0.1:18000/api/v1/dev/weight -d '{"weight":1.25,"stable":true,"unit":"kg"}'
curl -X POST http://127.0.0.1:18000/api/v1/dev/print-mode -d '{"mode":"alternate"}'
curl -X POST http://127.0.0.1:18000/api/v1/dev/scenario -d '{"scenario":"stress","seed":7}'
```

Flaglar:
- `--no-printer-sim=true` - fake Zebra va fake `print_request` completion o'chadi; polygon faqat scale snapshot yozadi.
- `--no-scale-sim=true` - fake scale endpoint/control o'chadi; printer simulyatsiyasi ishlashi mumkin.
