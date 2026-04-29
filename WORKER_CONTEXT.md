# GScale Worker Context

This file is a living handoff for the current GoDEX G500 label flow and the
Cloudflare-backed scan page.

## What is working now

- `scripts/godex_g500_pack_label.py` prints the pack label directly to the
  GoDEX G500 printer over USB.
- The QR on the label now points to a Cloudflare Worker on
  `scan.wspace.sbs`.
- The QR payload is URL-shaped and camera-friendly:
  - `HTTPS://SCAN.WSPACE.SBS/L/COMPANY/PRODUCT/KG/EPC`
- The scan page is stateless:
  - no database
  - no persistent storage
  - it only renders data already encoded in the QR URL
- Human-readable label text is now rendered as a monochrome BMP graphic on the
  host side and sent to the printer as a downloaded graphic, because the
  printer text path was corrupting Uzbek Cyrillic / special glyphs.

## Current live pieces

- Worker code: `scripts/scan_label_worker/worker.js`
- Worker config: `scripts/scan_label_worker/wrangler.toml`
- Print script: `scripts/godex_g500_pack_label.py`
- Local scan renderer used during testing:
  - `scripts/scan_label_server.py`

## Cloudflare routing

- `scan.wspace.sbs/*` is routed to the Worker.
- The Worker is deployed through Cloudflare Wrangler.
- Live checks returned `HTTP 200` on:
  - `https://scan.wspace.sbs/`
  - `https://scan.wspace.sbs/L/ACCORD/ZARQAND+PRYANIKI+KLUBNIKA+230+GR/89/30A5FEA7709854D93C2B7593`

## Important implementation details

- The QR payload is deliberately kept uppercase and alphanumeric-safe so
  mobile cameras are more likely to recognize it as a URL.
- The Worker reads the path and renders a plain white page with:
  - `COMPANY`
  - `MAHSULOT NOMI`
  - `NETTO`
  - `EPC`
- Print text is no longer sent through GoDEX font commands. The Python host
  renders the text block with Pillow/Noto Sans, downloads it as a BMP graphic
  (`~EB`) and places it with `Y`.
- This was the fix for broken Uzbek glyphs like `ў`, `ғ`, `қ`, and `ҳ`.
- No Go backend changes are needed for this flow.

## Verified print command

Example command that was tested successfully:

```bash
.venv/bin/python3 scripts/godex_g500_pack_label.py \
  --company-name Accord \
  --product-name "Zo‘r pista 100gr ko‘k" \
  --kg 89 \
  --epc 30A5FEA7709854D93C2B7593
```

Observed printer status:

- `status: 00,00000`
- `final_status: 50,00001`

## Notes for future edits

- Keep changes confined to `scripts/` unless the user explicitly asks to touch
  other parts of the repo.
- Do not remove or rewrite unrelated worktree changes.
- If QR scanning starts resolving as search instead of opening the page, first
  revisit the QR payload shape and the Worker route, not the printer layout.

## Current mental model

1. Printer script generates the label.
2. QR encodes a URL path with all data in it.
3. Cloudflare Worker receives the request.
4. Worker renders the visible label page.
5. Nothing is stored server-side.
