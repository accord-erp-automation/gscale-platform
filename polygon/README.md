# Polygon

`polygon` real scale yoki Zebra qurilmasiz ishlaydigan test muhiti.

Nimalarni beradi:
- fake `scale` oqimi;
- fake `zebra` holati;
- `bridge_state.json` ga live snapshot yozish;
- pending `print_request` ni `processing -> done/error` ga o'tkazish;
- virtual printerga kelgan buyruq preview va tarixini saqlash;
- HTTP endpointlar orqali state va qo'lda boshqaruv.

Ishga tushirish:

```bash
make run-polygon
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
- `POST /api/v1/dev/weight`
- `POST /api/v1/dev/reset`
- `POST /api/v1/dev/print-mode`

Misollar:

```bash
curl http://127.0.0.1:18000/api/v1/scale
curl http://127.0.0.1:18000/api/v1/dev/printer
curl -X POST http://127.0.0.1:18000/api/v1/dev/weight -d '{"weight":1.25,"stable":true,"unit":"kg"}'
curl -X POST http://127.0.0.1:18000/api/v1/dev/print-mode -d '{"mode":"alternate"}'
```
