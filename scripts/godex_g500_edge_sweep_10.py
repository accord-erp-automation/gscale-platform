#!/usr/bin/env python3
"""
10-step edge sweep for GoDEX G500.

This prints the same long left word and sweeps the right word across the
label so we can see which placement reaches the edge best.
"""

from __future__ import annotations

import argparse
import sys
import time
from pathlib import Path

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
    parser = argparse.ArgumentParser(description="GoDEX G500 10-step edge sweep")
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
        help="Long word to keep near the left edge",
    )
    parser.add_argument(
        "--right-text",
        default="WWWWWWWWWWWWWWWWWWWW",
        help="Long word to sweep across the right side",
    )
    parser.add_argument(
        "--top-y-mm",
        type=float,
        default=8,
        help="Vertical position in mm for both words",
    )
    parser.add_argument(
        "--left-x-mm",
        type=float,
        default=1.5,
        help="Left word anchor in mm",
    )
    parser.add_argument(
        "--right-start-mm",
        type=float,
        default=16,
        help="Starting X position for the right word in mm",
    )
    parser.add_argument(
        "--right-step-mm",
        type=float,
        default=2,
        help="Step between each right-word placement in mm",
    )
    parser.add_argument(
        "--x-mul-start",
        type=int,
        default=1,
        help="Starting text width multiplier",
    )
    parser.add_argument(
        "--x-mul-step",
        type=int,
        default=1,
        help="Multiplier increment per variant",
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


def build_label(
    left_text: str,
    right_text: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    top_y_mm: float,
    left_x_mm: float,
    right_x_mm: float,
    x_mul: int,
    y_mul: int,
    edge_pitch_dots: int,
) -> list[str]:
    left_text = sanitize_label_text(left_text)
    right_text = sanitize_label_text(right_text)
    left_x = mm_to_dots(left_x_mm, dpi)
    right_margin = mm_to_dots(right_x_mm, dpi)
    top_y = mm_to_dots(top_y_mm, dpi)
    label_width_dots = mm_to_dots(label_width_mm, dpi)
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

    for idx in range(10):
        right_x_mm = args.right_start_mm + (idx * args.right_step_mm)
        x_mul = max(1, args.x_mul_start + (idx * args.x_mul_step))
        print(f"variant {idx + 1}: right_x_mm={right_x_mm:.2f}, x_mul={x_mul}")
        for command in build_label(
            args.left_text,
            args.right_text,
            args.label_length_mm,
            args.label_gap_mm,
            args.label_width_mm,
            args.dpi,
            args.top_y_mm,
            args.left_x_mm,
            right_x_mm,
            x_mul,
            args.y_mul,
            args.edge_pitch_dots,
        ):
            send(dev, ep_out, ep_in, command)
        time.sleep(0.7)
        variant_status = send(dev, ep_out, ep_in, "~S,STATUS", read=True)
        print(f"variant_{idx + 1}_status: {variant_status or '(empty)'}")

    final_status = send(dev, ep_out, ep_in, "~S,STATUS", read=True)
    print(f"final_status: {final_status or '(empty)'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
