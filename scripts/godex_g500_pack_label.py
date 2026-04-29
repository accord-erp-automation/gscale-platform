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
import time
import sys
from pathlib import Path

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
    wrap_text_for_width,
)


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
) -> list[str]:
    company_name = sanitize_label_text(company_name)
    product_name = sanitize_label_text(product_name)
    kg_text = normalize_kg_value(kg_text)
    netto_text = f"NETTO: {kg_text} KG".upper()
    epc = sanitize_label_text(epc).upper()
    company_name = company_name.upper()
    product_name = product_name.upper()

    label_width_dots = mm_to_dots(label_width_mm, dpi)
    label_length_dots = mm_to_dots(label_length_mm, dpi)
    safe_margin_dots = mm_to_dots(safe_margin_mm, dpi)
    left_x = max(0, safe_margin_dots - mm_to_dots(2.0, dpi))
    gap_dots = mm_to_dots(3.0, dpi)
    line_step = mm_to_dots(5.0, dpi)

    qr_box_dots = mm_to_dots(qr_box_mm, dpi)
    qr_x = label_width_dots - qr_box_dots - mm_to_dots(5.0, dpi)
    qr_x = max(left_x, qr_x)

    text_width_dots = max(1, qr_x - left_x - gap_dots)
    product_lines = wrap_text_for_width(product_name, text_width_dots, dpi, x_mul=1)

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
    qr_mul = 7
    bold_dx = 1
    bold_dy = 1
    epc_font = "AB"
    product_font = "AB"
    netto_dx = 1
    netto_dy = 1

    commands = [
        "~S,ESG",
        "^AD",
        "^XSET,IMMEDIATE,1",
        "^XSET,ACTIVERESPONSE,1",
        "^XSET,CODEPAGE,16",
        f"^Q{label_length_mm},{label_gap_mm}",
        f"^W{label_width_mm}",
        "^H10",
        "^P1",
        "^L",
        f"{epc_font},{left_x},{epc_y},1,1,0,0,EPC: {epc}",
        f"BA,{left_x},{barcode_y},1,2,42,0,0,{epc}",
        f"AC,{left_x},{company_y},1,1,0,0,COMPANY: {company_name}",
        f"AC,{left_x + bold_dx},{company_y + bold_dy},1,1,0,0,COMPANY: {company_name}",
        f"{product_font},{left_x},{item_y},1,1,0,0,MAHSULOT NOMI: {product_lines[0]}",
        f"{product_font},{left_x + bold_dx},{item_y + bold_dy},1,1,0,0,MAHSULOT NOMI: {product_lines[0]}",
        f"AC,{left_x},{qty_y},1,1,0,0,{netto_text}",
        f"AC,{left_x + netto_dx},{qty_y + netto_dy},1,1,0,0,{netto_text}",
    ]

    for idx, line in enumerate(product_lines[1:], start=1):
        commands.append(f"{product_font},{left_x},{item_y + (idx * line_step)},1,1,0,0,{line}")

    commands.extend(
        [
            f"W{qr_x},{qr_y},2,1,L,8,{qr_mul},{len(epc)},0",
            epc,
            "E",
        ]
    )
    return commands


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
) -> str | None:
    for command in build_pack_label(
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
    ):
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
    )
    print(f"final_status: {final_status or '(empty)'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
