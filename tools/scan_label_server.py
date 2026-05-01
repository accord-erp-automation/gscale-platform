#!/usr/bin/env python3
"""Minimal stateless label view for QR scans.

The QR code stores all label data in the URL query. This service only decodes
that query and renders a plain white page; it does not store anything.
"""

from __future__ import annotations

import argparse
import base64
import html
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, unquote, urlparse


def decode_payload(raw_q: str) -> list[str]:
    raw_q = unquote(raw_q).replace("+", " ").strip()
    if not raw_q:
        return []

    try:
        padding = "=" * ((4 - len(raw_q) % 4) % 4)
        decoded = base64.urlsafe_b64decode((raw_q + padding).encode("utf-8")).decode(
            "utf-8"
        )
        if decoded.strip():
            return decoded.splitlines()
    except Exception:
        pass

    return raw_q.replace("~", "|").split("|")


def render_pack_label(parts: list[str]) -> bytes:
    company = parts[0] if len(parts) > 0 else ""
    product = parts[1] if len(parts) > 1 else ""
    kg = parts[2] if len(parts) > 2 else ""
    brutto = parts[3] if len(parts) > 3 else "5"
    epc = parts[4] if len(parts) > 4 else (parts[3] if len(parts) > 3 else "")

    lines = [
        f"COMPANY: {company}",
        f"MAHSULOT NOMI: {product}",
        f"NETTO: {kg} KG",
        f"BRUTTO: {brutto} KG",
        f"EPC: {epc}",
    ]
    body = "\n".join(html.escape(line) for line in lines)
    return _render_page(body)


def render_archive_label(parts: list[str]) -> bytes:
    item = parts[1] if len(parts) > 1 else ""
    qty = parts[2] if len(parts) > 2 else ""
    batch_time = parts[3] if len(parts) > 3 else ""
    session = parts[4] if len(parts) > 4 else ""

    lines = [
        "BATCH INFO",
        f"ITEM: {item}",
        f"QTY: {qty} KG",
        f"TIME: {batch_time}",
        f"SESSION: {session}",
    ]
    body = "\n".join(html.escape(line) for line in lines)
    return _render_page(body)


def _render_page(body: str) -> bytes:
    page = f"""<!doctype html>
<html lang="uz">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Label</title>
<body style="margin:24px;background:#fff;color:#000;font:20px/1.45 monospace;white-space:pre-wrap">{body}
<script>
const raw = location.hash.slice(1);
if (raw) {{
  const values = raw.split("~").map(v => decodeURIComponent(v.replace(/\\+/g, " ")));
  if (values[0] === "ARCHIVE") {{
    document.body.textContent = [
      "BATCH INFO",
      `ITEM: ${{values[1] || ""}}`,
      `QTY: ${{values[2] || ""}} KG`,
      `TIME: ${{values[3] || ""}}`,
      `SESSION: ${{values[4] || ""}}`,
    ].join("\\n");
  }} else {{
    document.body.textContent = [
      `COMPANY: ${{values[0] || ""}}`,
      `MAHSULOT NOMI: ${{values[1] || ""}}`,
      `NETTO: ${{values[2] || ""}} KG`,
      `BRUTTO: ${{values[3] || "5"}} KG`,
      `EPC: ${{values[4] || values[3] || ""}}`,
    ].join("\\n");
  }}
}}
</script>
</body>
</html>
"""
    return page.encode("utf-8")


class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        parsed = urlparse(self.path)
        query = parse_qs(parsed.query)
        raw_q = query.get("q", query.get("Q", [""]))[0]
        if not raw_q and parsed.path.upper().startswith("/L/"):
            raw_q = "|".join(parsed.path[3:].split("/"))
        elif not raw_q and parsed.path.upper().startswith("/A/"):
            raw_q = parsed.path[3:].strip("/")
        parts = decode_payload(raw_q)
        if parts and parts[0] == "ARCHIVE":
            payload = render_archive_label(parts)
        else:
            payload = render_pack_label(parts)
        self.send_response(200)
        self.send_header("content-type", "text/html; charset=utf-8")
        self.send_header("cache-control", "no-store")
        self.send_header("content-length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format: str, *args: object) -> None:
        return


def main() -> int:
    parser = argparse.ArgumentParser(description="Stateless QR label web view")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=39119)
    args = parser.parse_args()

    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"serving http://{args.host}:{args.port}", flush=True)
    server.serve_forever()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
