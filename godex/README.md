# GoDEX G500

Go implementation of the working Python GoDEX G500 print path from
`scripts/godex_g500_direct_usb_test.py` and `scripts/godex_g500_pack_label.py`.

The production path here intentionally keeps the same core technology choices:

- direct USB bulk transfer to GoDEX `195f:0001`
- EZPL commands
- recovery/status commands such as `~S,STATUS`, `~S,SENSOR`, and `^AD`
- host-side QR rendering
- host-side Noto Sans text rendering into a monochrome BMP graphic
- GoDEX graphic download with `~EB` and placement with `Y`

## Build

```bash
cd /home/wikki/storage/local.git/gscale-platform/godex
GOWORK=off go test ./...
GOWORK=off go build ./cmd/godex-g500
```

## Status

```bash
sudo ./godex-g500 --status-only
```

## Pack Label

```bash
sudo ./godex-g500 \
  --pack-label \
  --company-name Accord \
  --product-name "Zo'r pista 100gr kok" \
  --kg 89 \
  --epc 30A5FEA7709854D93C2B7593
```

The Go CLI mirrors the Python script defaults:

- label length: `50mm`
- label width: `50mm`
- gap: `3mm`
- dpi: `203`
- safe margin: `4mm`
- QR box: `18mm`
- text graphic name: `TEXTLBL`
- QR graphic name: `QRLBL`

The `--brutto` flag defaults to `5`, matching the current Python
`godex_g500_pack_label.py` behavior.
