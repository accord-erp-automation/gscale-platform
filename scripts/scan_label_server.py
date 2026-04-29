#!/usr/bin/env python3
"""Minimal stateless label view for QR scans.

The QR code stores all label data in the URL query. This service only decodes
that query and renders a plain white page; it does not store anything.
"""

from __future__ import annotations

import argparse
import html
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, unquote, urlparse


def render_label(raw_q: str) -> bytes:
    parts = unquote(raw_q).replace("+", " ").replace("~", "|").split("|")
    company = parts[0] if len(parts) > 0 else ""
    product = parts[1] if len(parts) > 1 else ""
    kg = parts[2] if len(parts) > 2 else ""
    epc = parts[3] if len(parts) > 3 else ""

    lines = [
        f"COMPANY: {company}",
        f"MAHSULOT NOMI: {product}",
        f"NETTO: {kg} KG",
        f"EPC: {epc}",
    ]
    body = "\n".join(html.escape(line) for line in lines)
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
  document.body.textContent = [
    `COMPANY: ${{values[0] || ""}}`,
    `MAHSULOT NOMI: ${{values[1] || ""}}`,
    `NETTO: ${{values[2] || ""}} KG`,
    `EPC: ${{values[3] || ""}}`,
  ].join("\\n");
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
        payload = render_label(raw_q)
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
