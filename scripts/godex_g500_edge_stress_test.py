#!/usr/bin/env python3
"""
Edge placement stress test for GoDEX G500.

This prints two long single words near the left and right edges of a label so
we can inspect how much horizontal room the printer and current layout leave.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

# Make the repository root importable when this script is run directly.
ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from scripts.godex_g500_direct_usb_test import (
    find_printer,
    mm_to_dots,
    recover,
    sanitize_label_text,
    send,
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="GoDEX G500 edge placement stress test"
    )
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
        "--left-text",
        default="WWWWWWWWWWWWWWWWWWWW",
        help="Long word to place near the left edge",
    )
    parser.add_argument(
        "--right-text",
        default="MMMMMMMMMMMMMMMMMMMM",
        help="Long word to place near the right edge",
    )
    parser.add_argument(
        "--top-y-mm",
        type=float,
        default=8,
        help="Vertical position in mm for both words",
    )
    parser.add_argument(
        "--left-margin-mm",
        type=float,
        default=2,
        help="Left margin in mm",
    )
    parser.add_argument(
        "--right-margin-mm",
        type=float,
        default=2,
        help="Right margin in mm",
    )
    parser.add_argument(
        "--x-mul",
        type=int,
        default=3,
        help="Text width multiplier",
    )
    parser.add_argument(
        "--y-mul",
        type=int,
        default=1,
        help="Text height multiplier",
    )
    parser.add_argument(
        "--edge-pitch-dots",
        type=int,
        default=14,
        help="Estimated width in dots of one character at x_mul=1",
    )
    parser.add_argument(
        "--skip-recover",
        action="store_true",
        help="Do not run recovery even if printer is not ready",
    )
    parser.add_argument(
        "--status-only",
        action="store_true",
        help="Only read printer status and exit",
    )
    return parser.parse_args()


def build_edge_label(
    left_text: str,
    right_text: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    top_y_mm: float,
    left_margin_mm: float,
    right_margin_mm: float,
    x_mul: int,
    y_mul: int,
    edge_pitch_dots: int,
) -> list[str]:
    left_text = sanitize_label_text(left_text)
    right_text = sanitize_label_text(right_text)

    label_width_dots = mm_to_dots(label_width_mm, dpi)
    left_x = mm_to_dots(left_margin_mm, dpi)
    top_y = mm_to_dots(top_y_mm, dpi)
    right_margin = max(1, mm_to_dots(right_margin_mm, dpi))
    right_text_width = max(edge_pitch_dots * x_mul * len(right_text), edge_pitch_dots * x_mul)
    right_x = max(left_x, label_width_dots - right_text_width - right_margin)

    return [
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
        f"AC,{left_x},{top_y},{x_mul},{y_mul},0,0,{left_text}",
        f"AC,{right_x},{top_y},{x_mul},{y_mul},0,0,{right_text}",
        "E",
    ]


def main() -> int:
    args = parse_args()
    dev, ep_out, ep_in = find_printer()

    status = send(dev, ep_out, ep_in, "~S,STATUS", read=True)
    print(f"status: {status or '(empty)'}")

    if args.status_only:
        return 0

    if status and not status.startswith("00,") and not args.skip_recover:
        print("recover: running recovery sequence")
        status = recover(dev, ep_out, ep_in)
        print(f"recovered: {status or '(empty)'}")

    for command in build_edge_label(
        args.left_text,
        args.right_text,
        args.label_length_mm,
        args.label_gap_mm,
        args.label_width_mm,
        args.dpi,
        args.top_y_mm,
        args.left_margin_mm,
        args.right_margin_mm,
        args.x_mul,
        args.y_mul,
        args.edge_pitch_dots,
    ):
        send(dev, ep_out, ep_in, command)

    final_status = send(dev, ep_out, ep_in, "~S,STATUS", read=True)
    print(f"final_status: {final_status or '(empty)'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
