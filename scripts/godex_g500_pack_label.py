#!/usr/bin/env python3
"""
Clean GoDEX G500 pack-label printer.

Inputs:
- company name
- product name
- kg
- EPC code

Output:
- company name
- product name
- kg
- QR code generated from EPC
- EPC text line
"""

from __future__ import annotations

import argparse
import io
import time
import sys
from pathlib import Path
from urllib.parse import quote, quote_plus

from PIL import Image, ImageDraw, ImageFont

# Make the repository root importable when this script is run directly.
ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from scripts.godex_g500_direct_usb_test import (
    find_printer,
    mm_to_dots,
    normalize_kg_value,
    recover,
    sanitize_label_text,
    send,
    write_raw,
)

DEFAULT_QR_BASE_URL = "HTTPS://SCAN.WSPACE.SBS/L/"
TEXT_GRAPHIC_NAME = "TEXTLBL"
NOTO_SANS_REGULAR = Path("/usr/share/fonts/noto/NotoSans-Regular.ttf")
NOTO_SANS_BOLD = Path("/usr/share/fonts/noto/NotoSans-Bold.ttf")


def load_font(path: Path, size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(str(path), size=size)


def text_width(draw: ImageDraw.ImageDraw, text: str, font: ImageFont.FreeTypeFont) -> int:
    if not text:
        return 0
    box = draw.textbbox((0, 0), text, font=font)
    return box[2] - box[0]


def wrap_text_pixels(
    draw: ImageDraw.ImageDraw,
    text: str,
    font: ImageFont.FreeTypeFont,
    max_width: int,
) -> list[str]:
    text = sanitize_label_text(text)
    if not text:
        return [""]

    words = text.split()
    lines: list[str] = []
    current = ""

    for word in words:
        candidate = word if not current else f"{current} {word}"
        if text_width(draw, candidate, font) <= max_width:
            current = candidate
            continue

        if current:
            lines.append(current)

        if text_width(draw, word, font) <= max_width:
            current = word
            continue

        chunk = ""
        for ch in word:
            candidate = f"{chunk}{ch}"
            if not chunk or text_width(draw, candidate, font) <= max_width:
                chunk = candidate
            else:
                if chunk:
                    lines.append(chunk)
                chunk = ch
        current = chunk

    if current:
        lines.append(current)

    return lines or [""]


def wrap_prefixed_text_pixels(
    draw: ImageDraw.ImageDraw,
    prefix: str,
    text: str,
    font: ImageFont.FreeTypeFont,
    max_width: int,
) -> tuple[list[str], int]:
    prefix = sanitize_label_text(prefix)
    text = sanitize_label_text(text)
    prefix_render = f"{prefix} "
    prefix_width = text_width(draw, prefix_render, font)
    body_words = text.split()
    if not body_words:
        return [prefix], prefix_width

    if prefix_width >= max_width:
        return [prefix] + wrap_text_pixels(draw, text, font, max_width), prefix_width

    remaining_width = max(1, max_width - prefix_width)
    first_line_words: list[str] = []
    consumed = 0
    for idx, word in enumerate(body_words):
        candidate = word if not first_line_words else f"{' '.join(first_line_words)} {word}"
        if text_width(draw, candidate, font) <= remaining_width:
            first_line_words.append(word)
            consumed = idx + 1
        else:
            break

    lines = [prefix_render + " ".join(first_line_words)] if first_line_words else [prefix.rstrip()]

    remaining_text = " ".join(body_words[consumed:])
    if remaining_text:
        lines.extend(wrap_text_pixels(draw, remaining_text, font, max_width))

    return lines, prefix_width


def measure_pack_text(
    company_name: str,
    product_name: str,
    kg_text: str,
    epc: str,
    product_width_dots: int,
) -> tuple[str, list[str], int, str, str]:
    scratch = Image.new("1", (1, 1), 1)
    draw = ImageDraw.Draw(scratch)
    bold_22 = load_font(NOTO_SANS_BOLD, 22)

    company_text = f"COMPANY: {company_name}"
    product_lines, product_indent_dots = wrap_prefixed_text_pixels(
        draw,
        "MAHSULOT NOMI:",
        product_name,
        bold_22,
        product_width_dots,
    )
    netto_text = f"NETTO: {kg_text} KG".upper()
    epc_text = f"EPC: {epc}"
    return company_text, product_lines, product_indent_dots, netto_text, epc_text


def render_text_graphic(
    label_width_dots: int,
    label_length_dots: int,
    left_x: int,
    company_y: int,
    item_y: int,
    qty_y: int,
    epc_y: int,
    company_text: str,
    product_lines: list[str],
    product_indent_dots: int,
    netto_text: str,
    epc_text: str,
) -> bytes:
    canvas = Image.new("1", (label_width_dots, label_length_dots), 1)
    draw = ImageDraw.Draw(canvas)

    regular_20 = load_font(NOTO_SANS_REGULAR, 20)
    regular_22 = load_font(NOTO_SANS_REGULAR, 22)
    bold_24 = load_font(NOTO_SANS_BOLD, 24)
    bold_22 = load_font(NOTO_SANS_BOLD, 22)

    draw.text((left_x, epc_y), epc_text, font=regular_20, fill=0)
    draw.text((left_x, company_y), company_text, font=bold_24, fill=0)

    for idx, line in enumerate(product_lines):
        y = item_y + idx * 28
        draw.text((left_x, y), line, font=bold_22, fill=0)

    draw.text((left_x, qty_y), netto_text, font=regular_22, fill=0)

    output = io.BytesIO()
    canvas.save(output, format="BMP")
    return output.getvalue()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Clean direct-USB GoDEX G500 pack label printer"
    )
    parser.add_argument("--company-name", required=True, help="Company name")
    parser.add_argument("--product-name", required=True, help="Product name")
    parser.add_argument("--kg", required=True, help="Kg value to print")
    parser.add_argument("--epc", required=True, help="EPC code for QR and text")
    parser.add_argument(
        "--label-length-mm",
        type=int,
        default=50,
        help="EZPL label length in mm used for ^Q",
    )
    parser.add_argument(
        "--label-gap-mm",
        type=int,
        default=3,
        help="EZPL gap length in mm used for ^Q",
    )
    parser.add_argument(
        "--label-width-mm",
        type=int,
        default=50,
        help="EZPL label width in mm used for ^W",
    )
    parser.add_argument(
        "--dpi",
        type=int,
        default=203,
        help="Printer resolution in dpi for mm-to-dot conversion",
    )
    parser.add_argument(
        "--safe-margin-mm",
        type=float,
        default=4.0,
        help="Inner margin to keep content inside the printable area",
    )
    parser.add_argument(
        "--qr-box-mm",
        type=float,
        default=14.0,
        help="Approximate QR bounding box size in mm",
    )
    parser.add_argument(
        "--qr-mode",
        choices=("label", "dataurl", "url"),
        default="url",
        help="QR payload mode: embedded label data, inline text data URL, or scan URL",
    )
    parser.add_argument(
        "--skip-recover",
        action="store_true",
        help="Skip the recovery sequence even if printer is not ready",
    )
    parser.add_argument(
        "--status-only",
        action="store_true",
        help="Only read printer status and exit",
    )
    return parser.parse_args()


def build_pack_label(
    company_name: str,
    product_name: str,
    kg_text: str,
    epc: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    safe_margin_mm: float,
    qr_box_mm: float,
    qr_mode: str,
) -> tuple[list[str], bytes]:
    company_name = sanitize_label_text(company_name)
    product_name = sanitize_label_text(product_name)
    kg_text = normalize_kg_value(kg_text)
    epc = sanitize_label_text(epc).upper()
    company_name = company_name.upper()
    product_name = product_name.upper()
    qr_mode = sanitize_label_text(qr_mode).lower()

    if qr_mode == "url":
        compact_payload = "/".join(
            quote_plus(value, safe="")
            for value in (company_name, product_name, kg_text, epc)
        )
        qr_payload = f"{DEFAULT_QR_BASE_URL}{compact_payload}"
        qr_input_mode = 2
        qr_model = 2
        qr_error_level = "M"
        qr_data = qr_payload
    elif qr_mode == "dataurl":
        netto_text = f"NETTO: {kg_text} KG".upper()
        plain_text = (
            f"COMPANY: {company_name}\n"
            f"MAHSULOT NOMI: {product_name}\n"
            f"{netto_text}\n"
            f"EPC: {epc}"
        )
        qr_payload = "data:text/plain;charset=utf-8," + quote(plain_text, safe="")
        qr_input_mode = 3
        qr_model = 2
        qr_error_level = "M"
        qr_data = f"{len(qr_payload.encode('utf-8')):04d}{qr_payload}"
    else:
        netto_text = f"NETTO: {kg_text} KG".upper()
        qr_payload = "\n".join(
            [
                f"COMPANY: {company_name}",
                f"MAHSULOT NOMI: {product_name}",
                netto_text,
                f"EPC: {epc}",
            ]
        )
        qr_input_mode = 3
        qr_model = 2
        qr_error_level = "M"
        qr_data = f"{len(qr_payload.encode('utf-8')):04d}{qr_payload}"

    label_width_dots = mm_to_dots(label_width_mm, dpi)
    label_length_dots = mm_to_dots(label_length_mm, dpi)
    safe_margin_dots = mm_to_dots(safe_margin_mm, dpi)
    left_x = max(0, safe_margin_dots - mm_to_dots(2.0, dpi))
    gap_dots = mm_to_dots(3.0, dpi)
    line_step = mm_to_dots(5.0, dpi)

    effective_qr_box_mm = qr_box_mm
    qr_mul = 4
    if qr_mode == "url":
        effective_qr_box_mm = max(qr_box_mm, 16.0)
        qr_mul = 4
    elif qr_mode == "dataurl":
        effective_qr_box_mm = max(qr_box_mm, 24.0)
        qr_mul = 3
    qr_box_dots = mm_to_dots(effective_qr_box_mm, dpi)
    qr_right_gap_dots = mm_to_dots(6.0, dpi)
    base_qr_x = label_width_dots - qr_box_dots - qr_right_gap_dots
    qr_x = min(label_width_dots - qr_box_dots, max(left_x, base_qr_x))

    product_width_dots = max(1, qr_x - left_x - gap_dots)
    company_text, product_lines, product_indent_dots, netto_text, epc_text = measure_pack_text(
        company_name,
        product_name,
        kg_text,
        epc,
        product_width_dots,
    )

    company_y = safe_margin_dots + (line_step * 2)
    item_y = company_y + line_step
    qty_y = item_y + (len(product_lines) * line_step)
    qr_y = max(safe_margin_dots + line_step * 2, qty_y + line_step)
    qr_y = min(
        label_length_dots - safe_margin_dots - mm_to_dots(18.0, dpi),
        qr_y + mm_to_dots(8.0, dpi),
    )
    epc_y = max(0, safe_margin_dots - (line_step * 3))
    barcode_y = epc_y + line_step

    graphic_bytes = render_text_graphic(
        label_width_dots,
        label_length_dots,
        left_x,
        company_y,
        item_y,
        qty_y,
        epc_y,
        company_text,
        product_lines,
        product_indent_dots,
        netto_text,
        epc_text,
    )

    commands: list[str] = [
        "~S,ESG",
        "^AD",
        "^XSET,UNICODE,1",
        "^XSET,IMMEDIATE,1",
        "^XSET,ACTIVERESPONSE,1",
        "^XSET,CODEPAGE,16",
        f"^Q{label_length_mm},{label_gap_mm}",
        f"^W{label_width_mm}",
        "^H10",
        "^P1",
        "^L",
        f"Y0,0,{TEXT_GRAPHIC_NAME}",
        f"BA,{left_x},{barcode_y},1,2,42,0,0,{epc}",
        f"W{qr_x},{qr_y},{qr_input_mode},{qr_model},{qr_error_level},8,{qr_mul},{len(qr_data.encode('utf-8'))},0",
        qr_data,
        "E",
    ]
    return commands, graphic_bytes


def download_graphic(dev, ep_out, ep_in, name: str, graphic_bytes: bytes) -> None:
    try:
        send(dev, ep_out, ep_in, f"~MDELG,{name}", pause=0.1)
    except Exception:
        pass
    send(dev, ep_out, ep_in, f"~EB,{name},{len(graphic_bytes)}", pause=0.05)
    write_raw(dev, ep_out, graphic_bytes)
    time.sleep(0.4)


def print_pack(
    dev,
    ep_out,
    ep_in,
    company_name: str,
    product_name: str,
    kg_text: str,
    epc: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    safe_margin_mm: float,
    qr_box_mm: float,
    qr_mode: str,
) -> str | None:
    commands, graphic_bytes = build_pack_label(
        company_name,
        product_name,
        kg_text,
        epc,
        label_length_mm,
        label_gap_mm,
        label_width_mm,
        dpi,
        safe_margin_mm,
        qr_box_mm,
        qr_mode,
    )

    download_graphic(dev, ep_out, ep_in, TEXT_GRAPHIC_NAME, graphic_bytes)

    for command in commands:
        send(dev, ep_out, ep_in, command)

    time.sleep(1.0)
    return send(dev, ep_out, ep_in, "~S,STATUS", read=True)


def main() -> int:
    args = parse_args()
    dev, ep_out, ep_in = find_printer()

    status = send(dev, ep_out, ep_in, "~S,STATUS", read=True)
    print(f"status: {status or '(empty)'}")

    if args.status_only:
        return 0

    if status not in {None, "", "00,00000"} and not args.skip_recover:
        recover(dev, ep_out, ep_in)
        time.sleep(0.5)

    final_status = print_pack(
        dev,
        ep_out,
        ep_in,
        args.company_name,
        args.product_name,
        args.kg,
        args.epc,
        args.label_length_mm,
        args.label_gap_mm,
        args.label_width_mm,
        args.dpi,
        args.safe_margin_mm,
        args.qr_box_mm,
        args.qr_mode,
    )
    print(f"final_status: {final_status or '(empty)'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
