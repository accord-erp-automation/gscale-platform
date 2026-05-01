# Batch Labels

`batch` is the Go implementation of the batch/archive label flow. It was
cloned from the GoDEX printer stack so the archive path stays isolated from the
main `godex` pack-label package.

## What It Supports

- GoDEX `G500 / G530` family printers
- direct USB bulk transfer to GoDEX `195f:0001`
- EZPL command generation
- host-side QR rendering
- host-side Noto Sans text rendering into a monochrome BMP graphic
- EZPL command generation for archive batch labels
- archive batch labels that reuse the pack-label coordinate style and only
  drop the company name, EPC barcode, and pack barcode
- archive batch labels with item name, brutto, netto, date, and QR history

## Compatibility Notes

- Verified: GoDEX `G500`, `G530`
- Likely-compatible with testing: nearby `G500+ / G530+` models and other
  GoDEX printers that expose the same EZPL + USB transport shape
- Not promised without test: GoDEX models that need a different transport,
  vendor ID, or command dialect

## Platform Requirements

- Linux only for real USB printer access
- supported CPU architectures: `amd64`, `arm64`
- printer power: use the printer’s own OEM adapter / PSU
- host power: a low-power Linux PC or SBC is enough; the printer does the heavy
  work

The Go implementation does not hardcode a wattage requirement. In practice, the
host only needs stable USB access and enough CPU to build the label graphics.

## Build

```bash
cd /home/wikki/storage/local.git/gscale-platform/batch
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

## Current Defaults

- label length: `50mm`
- label width: `50mm`
- gap: `3mm`
- dpi: `203`
- safe margin: `4mm`
- QR box: `18mm`
- text graphic name: `TEXTLBL`
- QR graphic name: `QRLBL`
- `--brutto` defaults to `5`

## Label Stock

The archive batch layout now defaults to the same `60 × 80 mm` stock used by
the existing archive flow, so the printed content lands on the same physical
label and does not drift into the label seam.

If you need to fine-tune stock for a specific roll, pass
`--label-length-mm 80 --label-width-mm 60` explicitly. The engine already
supports both values through the CLI flags.

## Notes

- QR payloads are URL-shaped and are generated host-side.
- Human-readable label text is rendered on the host because direct printer text
  rendering was not safe for the Uzbek glyphs used in production labels.
- This package is the source of truth for the batch/archive printer path; no
  Python printer script is required anymore.
