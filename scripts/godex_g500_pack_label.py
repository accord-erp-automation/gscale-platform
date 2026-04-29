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
import subprocess
import time
import sys
from pathlib import Path
from urllib.parse import quote_plus

from PIL import Image, ImageChops, ImageDraw, ImageFont

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

DEFAULT_QR_BASE_URL = "https://scan.wspace.sbs/L/"
TEXT_GRAPHIC_NAME = "TEXTLBL"
QR_GRAPHIC_NAME = "QRLBL"
NOTO_SANS_REGULAR = Path("/usr/share/fonts/noto/NotoSans-Regular.ttf")
NOTO_SANS_BOLD = Path("/usr/share/fonts/noto/NotoSans-Bold.ttf")


def load_font(path: Path, size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(str(path), size=size)


def text_width(draw: ImageDraw.ImageDraw, text: str, font: ImageFont.FreeTypeFont) -> int:
    if not text:
        return 0
    box = draw.textbbox((0, 0), text, font=font)
    return box[2] - box[0]


def encode_scan_payload(
    company_name: str,
    product_name: str,
    kg_text: str,
    brutto_text: str,
    epc: str,
) -> str:
    compact_payload = "/".join(
        quote_plus(value, safe="")
        for value in (company_name, product_name, kg_text, brutto_text, epc)
    )
    return f"{DEFAULT_QR_BASE_URL}{compact_payload}"


def render_qr_graphic(payload: str, box_dots: int) -> bytes:
    proc = subprocess.run(
        [
            "qrencode",
            "-t",
            "PNG",
            "-l",
            "L",
            "-m",
            "4",
            "-s",
            "1",
            "-o",
            "-",
            payload,
        ],
        check=True,
        capture_output=True,
    )
    source = Image.open(io.BytesIO(proc.stdout)).convert("1")
    if source.width != box_dots or source.height != box_dots:
        resample = getattr(Image, "Resampling", Image).NEAREST
        source = source.resize((box_dots, box_dots), resample)
    output = io.BytesIO()
    source.save(output, format="BMP")
    return output.getvalue()


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
    first_line_width: int,
    rest_line_width: int,
) -> list[str]:
    prefix = sanitize_label_text(prefix)
    text = sanitize_label_text(text)
    if not text:
        return [prefix] if prefix else [""]

    prefix_render = f"{prefix} " if prefix else ""
    prefix_width = text_width(draw, prefix_render, font)

    if not prefix:
        return wrap_text_pixels(draw, text, font, first_line_width)

    if prefix_width >= first_line_width:
        return [prefix] + wrap_text_pixels(draw, text, font, rest_line_width)

    body_words = text.split()
    if not body_words:
        return [prefix.rstrip()]

    lines: list[str] = []
    current_words: list[str] = []
    current_width = first_line_width - prefix_width
    line_index = 0

    def line_limit_for(index: int) -> int:
        # Keep the first product line plus the next 5 lines on the same width,
        # then let any further overflow use the narrower continuation width.
        return first_line_width if index < 6 else rest_line_width

    for word in body_words:
        candidate = word if not current_words else f"{' '.join(current_words)} {word}"
        if text_width(draw, candidate, font) <= current_width:
            current_words.append(word)
            continue

        if not lines:
            lines.append((prefix_render + " ".join(current_words)).rstrip())
        else:
            lines.append(" ".join(current_words))

        line_index += 1
        current_words = [word]
        current_width = line_limit_for(line_index)

    if current_words:
        if not lines:
            lines.append((prefix_render + " ".join(current_words)).rstrip())
        else:
            lines.append(" ".join(current_words))

    return lines


def measure_pack_text(
    company_name: str,
    product_name: str,
    kg_text: str,
    brutto_text: str,
    epc: str,
    product_first_line_width_dots: int,
    product_rest_line_width_dots: int,
) -> tuple[str, list[str], str, str, str]:
    scratch = Image.new("1", (1, 1), 1)
    draw = ImageDraw.Draw(scratch)
    bold_21 = load_font(NOTO_SANS_BOLD, 21)

    company_text = f"COMPANY: {company_name}"
    product_lines = wrap_prefixed_text_pixels(
        draw,
        "MAHSULOT NOMI:",
        product_name,
        bold_21,
        product_first_line_width_dots,
        product_rest_line_width_dots,
    )
    netto_text = f"NETTO: {kg_text} KG".upper()
    brutto_text = f"BRUTTO: {brutto_text} KG".upper()
    epc_text = f"EPC: {epc}"
    return company_text, product_lines, netto_text, brutto_text, epc_text


def render_text_graphic(
    label_width_dots: int,
    label_length_dots: int,
    left_x: int,
    company_y: int,
    item_y: int,
    qty_y: int,
    brutto_y: int,
    epc_y: int,
    company_text: str,
    product_lines: list[str],
    netto_text: str,
    brutto_text: str,
    epc_text: str,
) -> bytes:
    canvas = Image.new("1", (label_width_dots, label_length_dots), 1)
    draw = ImageDraw.Draw(canvas)

    regular_20 = load_font(NOTO_SANS_REGULAR, 20)
    regular_26 = load_font(NOTO_SANS_REGULAR, 26)
    bold_24 = load_font(NOTO_SANS_BOLD, 24)
    bold_21 = load_font(NOTO_SANS_BOLD, 21)

    epc_bbox = draw.textbbox((0, 0), epc_text, font=regular_20)
    epc_draw_y = epc_y - epc_bbox[1]
    draw.text((left_x, epc_draw_y), epc_text, font=regular_20, fill=0)
    draw.text((left_x, company_y), company_text, font=bold_24, fill=0)

    for idx, line in enumerate(product_lines):
        y = item_y + idx * 28
        draw.text((left_x, y), line, font=bold_21, fill=0)

    draw.text((left_x, qty_y), netto_text, font=regular_26, fill=0)
    draw.text((left_x, brutto_y), brutto_text, font=regular_26, fill=0)

    ink = ImageChops.invert(canvas.convert("L"))
    bbox = ink.getbbox()
    if bbox:
        left, top, right, bottom = bbox
        crop_left = max(0, left - 1)
        crop_top = max(0, top)
        crop_right = min(canvas.width, right + 1)
        crop_bottom = min(canvas.height, bottom + 1)
        canvas = canvas.crop((crop_left, crop_top, crop_right, crop_bottom))

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
        default=18.0,
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
    brutto_text: str,
    epc: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    safe_margin_mm: float,
    qr_box_mm: float,
    qr_mode: str,
) -> tuple[list[str], bytes, bytes]:
    company_name = sanitize_label_text(company_name)
    product_name = sanitize_label_text(product_name)
    kg_text = normalize_kg_value(kg_text)
    brutto_text = normalize_kg_value(brutto_text)
    epc = sanitize_label_text(epc).upper()
    company_name = company_name.upper()
    product_name = product_name.upper()
    qr_mode = sanitize_label_text(qr_mode).lower()
    netto_text = f"NETTO: {kg_text} KG".upper()
    qr_payload = encode_scan_payload(company_name, product_name, kg_text, brutto_text, epc)

    label_width_dots = mm_to_dots(label_width_mm, dpi)
    label_length_dots = mm_to_dots(label_length_mm, dpi)
    safe_margin_dots = mm_to_dots(safe_margin_mm, dpi)
    left_x = max(0, safe_margin_dots - mm_to_dots(2.0, dpi))
    gap_dots = mm_to_dots(3.0, dpi)
    line_step = mm_to_dots(5.0, dpi)

    qr_box_dots = mm_to_dots(qr_box_mm, dpi)
    qr_right_gap_dots = mm_to_dots(6.0, dpi)
    base_qr_x = label_width_dots - qr_box_dots - qr_right_gap_dots
    qr_x = min(label_width_dots - qr_box_dots, max(left_x, base_qr_x))

    product_first_line_width_dots = max(
        1,
        label_width_dots - left_x,
    )
    product_rest_line_width_dots = max(
        1,
        qr_x - left_x - mm_to_dots(5.0, dpi),
    )
    company_text, product_lines, netto_text, brutto_text, epc_text = measure_pack_text(
        company_name,
        product_name,
        kg_text,
        brutto_text,
        epc,
        product_first_line_width_dots,
        product_rest_line_width_dots,
    )

    company_y = safe_margin_dots + (line_step * 2)
    item_y = company_y + line_step
    qty_y = mm_to_dots(33.0, dpi)
    qr_y = max(safe_margin_dots + line_step * 2, qty_y + line_step)
    qr_y = min(
        label_length_dots - safe_margin_dots - mm_to_dots(18.0, dpi),
        qr_y + mm_to_dots(8.0, dpi),
    )
    epc_y = max(0, safe_margin_dots - (line_step * 5))
    text_block_up_dots = mm_to_dots(3.0, dpi)
    header_block_up_dots = mm_to_dots(5.0, dpi)
    company_y = max(0, company_y - header_block_up_dots)
    item_y = max(0, item_y - header_block_up_dots)
    qty_y = max(0, qty_y - text_block_up_dots)
    brutto_y = max(0, qty_y + line_step)
    barcode_y = max(0, epc_y + mm_to_dots(3.0, dpi))
    barcode_x = max(0, left_x - mm_to_dots(2.0, dpi))
    qr_graphic_bytes = render_qr_graphic(qr_payload, qr_box_dots)

    graphic_bytes = render_text_graphic(
        label_width_dots,
        label_length_dots,
        left_x,
        company_y,
        item_y,
        qty_y,
        brutto_y,
        epc_y,
        company_text,
        product_lines,
        netto_text,
        brutto_text,
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
        f"BA,{barcode_x},{barcode_y},1,2,42,0,0,{epc}",
        f"Y{qr_x},{qr_y},{QR_GRAPHIC_NAME}",
        "E",
    ]
    return commands, graphic_bytes, qr_graphic_bytes


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
    brutto_text: str,
    epc: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    safe_margin_mm: float,
    qr_box_mm: float,
    qr_mode: str,
) -> str | None:
    commands, graphic_bytes, qr_graphic_bytes = build_pack_label(
        company_name,
        product_name,
        kg_text,
        brutto_text,
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
    download_graphic(dev, ep_out, ep_in, QR_GRAPHIC_NAME, qr_graphic_bytes)

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
        "5",
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
