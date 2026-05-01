# GScale Worker Context

This file is a living handoff for the current GoDEX G500/G530 label flow and
the stateless scan landing page.

## What is working now

- `godex/` contains the production Go implementation of the GoDEX pack-label
  printer flow.
- The QR on the label points to a Cloudflare Worker on `scan.wspace.sbs`.
- The QR payload is URL-shaped and camera-friendly:
  - `HTTPS://SCAN.WSPACE.SBS/L/COMPANY/PRODUCT/KG/BRUTTO/EPC`
- The scan page is stateless:
  - no database
  - no persistent storage
  - it only renders data already encoded in the QR URL
- Human-readable label text is rendered as a monochrome BMP graphic on the
  host side in Go and sent to the printer as a downloaded graphic, because
  printer text rendering was not safe for the Uzbek glyphs used in production.

## Current Live Pieces

- Worker code: `cloudflare/scan_label_worker/worker.js`
- Worker config: `cloudflare/scan_label_worker/wrangler.toml`
- Go printer code: `godex/`
- Local scan renderer used during testing: `tools/scan_label_server.py`

## Cloudflare Routing

- `scan.wspace.sbs/*` is routed to the Worker.
- The Worker is deployed through Cloudflare Wrangler.

## Important Implementation Details

- The QR payload is deliberately kept uppercase and alphanumeric-safe so
  mobile cameras are more likely to recognize it as a URL.
- The Worker reads the path and renders a plain white page with:
  - `COMPANY`
  - `MAHSULOT NOMI`
  - `NETTO`
  - `BRUTTO`
  - `EPC`
- Print text is rendered on the host with Noto Sans fonts and downloaded to
  the printer as a bitmap graphic.
- No Go backend changes are needed for the scan worker itself.

## Verified Print Path

Example command that was tested successfully:

```bash
cd /home/wikki/storage/local.git/gscale-platform/godex
GOWORK=off go run ./cmd/godex-g500 \
  --pack-label \
  --company-name Accord \
  --product-name "Zo‘r pista 100gr ko‘k" \
  --kg 89 \
  --epc 30A5FEA7709854D93C2B7593
```

Observed printer status:

- `status: 00,00000`
- `final_status: 50,00001`

## Notes for Future Edits

- Keep printer-flow changes in `godex/` and `scale/` unless the user asks for a
  wider refactor.
- If QR scanning starts resolving as search instead of opening the page, first
  revisit the QR payload shape and the Worker route, not the printer layout.

## Current Mental Model

1. Printer code generates the label.
2. QR encodes a URL path with all data in it.
3. Cloudflare Worker receives the request.
4. Worker renders the visible label page.
5. Nothing is stored server-side.
