# GoDEX G500 / G530

`godex` is the Go implementation of the GoDEX pack-label flow that used to live
in the legacy Python printer scripts.

## What It Supports

- GoDEX `G500 / G530` family printers
- direct USB bulk transfer to GoDEX `195f:0001`
- EZPL command generation
- host-side QR rendering
- host-side Noto Sans text rendering into a monochrome BMP graphic
- GoDEX graphic download with `~EB` and placement with `Y`

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

## Notes

- QR payloads are URL-shaped and are generated host-side.
- Human-readable label text is rendered on the host because direct printer text
  rendering was not safe for the Uzbek glyphs used in production labels.
- This package is the source of truth for the GoDEX production path; no Python
  printer script is required anymore.
