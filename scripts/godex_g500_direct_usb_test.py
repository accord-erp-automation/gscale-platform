#!/usr/bin/env python3
"""
Direct USB test for GoDEX G500.

This follows the documented working path in
docs/godex-g500-direct-print-notes.md:
- talk to the printer directly over USB
- use EZPL
- recover from the red status state
- print a minimal label
"""

from __future__ import annotations

import argparse
import secrets
import sys
import time
import textwrap
import unicodedata

import usb.core
import usb.util


VID = 0x195F
PID = 0x0001


RECOVERY_SEQUENCE = [
    "~S,ESG",
    "^AD",
    "^XSET,IMMEDIATE,1",
    "^XSET,ACTIVERESPONSE,1",
    "~Z",
    "~S,CANCEL",
    "~S,SENSOR",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Direct USB EZPL test for GoDEX G500"
    )
    parser.add_argument(
        "text",
        nargs="?",
        default="TEST",
        help="Label text to print, or QR payload when --qr is set",
    )
    parser.add_argument(
        "--qr",
        action="store_true",
        help="Print a QR code instead of plain text",
    )
    parser.add_argument(
        "--pack-label",
        action="store_true",
        help="Print product/company/batch label with QR",
    )
    parser.add_argument(
        "--status-only",
        action="store_true",
        help="Only read printer status and exit",
    )
    parser.add_argument(
        "--skip-recover",
        action="store_true",
        help="Do not run the recovery sequence before printing",
    )
    parser.add_argument(
        "--calibrate",
        action="store_true",
        help="Run recovery + sensor calibration and exit without printing",
    )
    parser.add_argument(
        "--label-length-mm",
        type=int,
        default=25,
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
        "--center",
        action="store_true",
        help="Place the printed content near the center of the label",
    )
    parser.add_argument(
        "--qr-box-mm",
        type=int,
        default=35,
        help="Approximate QR bounding box size in mm when centering QR output",
    )
    parser.add_argument(
        "--qr-mul",
        type=int,
        default=8,
        help="EZPL QR module size multiplier",
    )
    parser.add_argument(
        "--content-x-mm",
        type=float,
        default=-1,
        help="Optional absolute X position in mm for content",
    )
    parser.add_argument(
        "--content-y-mm",
        type=float,
        default=-1,
        help="Optional absolute Y position in mm for content",
    )
    parser.add_argument(
        "--company-name",
        default="TEST COMPANY",
        help="Company name for --pack-label",
    )
    parser.add_argument(
        "--product-name",
        "--item-name",
        dest="product_name",
        default="TEST PRODUCT",
        help="Product name for --pack-label",
    )
    parser.add_argument(
        "--qty-text",
        default="1.000 kg",
        help="Quantity text for --pack-label",
    )
    parser.add_argument(
        "--barcode",
        default="",
        help="Barcode data for --pack-label",
    )
    parser.add_argument(
        "--batch-code",
        default="",
        help="24-character batch code for --pack-label",
    )
    parser.add_argument(
        "--qr-payload",
        default="",
        help="QR payload for --pack-label (defaults to batch code)",
    )
    parser.add_argument(
        "--safe-margin-mm",
        type=float,
        default=4.0,
        help="Inner margin to keep pack-label content inside the printable area",
    )
    return parser.parse_args()


def random_qr_payload() -> str:
    # 36 hex chars matches the length used in the manual example closely.
    return secrets.token_hex(18).upper()


def random_batch_code() -> str:
    return secrets.token_hex(12).upper()


def sanitize_label_text(v: str) -> str:
    v = unicodedata.normalize("NFKC", v)
    v = v.replace("\r", " ").replace("\n", " ")
    v = v.replace("^", " ").replace("~", " ")
    return " ".join(v.split()).strip()


def split_product_lines(text: str, max_lines: int = 3) -> list[str]:
    text = sanitize_label_text(text)
    if not text:
        return [""]
    return textwrap.wrap(text, width=30, break_long_words=False)[:max_lines]


def normalize_kg_value(text: str) -> str:
    value = sanitize_label_text(text)
    lowered = value.lower()
    if lowered.startswith("kg:"):
        value = value.split(":", 1)[1].strip()
    elif lowered.endswith("kg"):
        value = value[:-2].strip()
    return value


def wrap_text_for_width(
    text: str,
    width_dots: int,
    dpi: int,
    x_mul: int = 1,
    pitch_dots: int = 14,
    min_chars: int = 8,
) -> list[str]:
    text = sanitize_label_text(text)
    if not text:
        return [""]
    char_width = max(1, pitch_dots * max(1, x_mul))
    width_chars = max(min_chars, width_dots // char_width)
    wrapped = textwrap.wrap(
        text,
        width=width_chars,
        break_long_words=False,
        break_on_hyphens=False,
    )
    if not wrapped:
        return [text]
    if any(len(line) > width_chars for line in wrapped):
        wrapped = textwrap.wrap(
            text,
            width=width_chars,
            break_long_words=True,
            break_on_hyphens=False,
        )
    return wrapped or [text]


def mm_to_dots(mm: float, dpi: int) -> int:
    return int(round(mm * dpi / 25.4))


def label_center_dots(label_length_mm: int, label_width_mm: int, dpi: int) -> tuple[int, int]:
    return (
        mm_to_dots(label_width_mm / 2.0, dpi),
        mm_to_dots(label_length_mm / 2.0, dpi),
    )


def find_printer():
    dev = usb.core.find(idVendor=VID, idProduct=PID)
    if dev is None:
        raise RuntimeError("GoDEX G500 not found")

    try:
        if dev.is_kernel_driver_active(0):
            try:
                dev.detach_kernel_driver(0)
            except Exception:
                pass
    except Exception:
        pass

    last_err = None
    for _ in range(10):
        try:
            dev.set_configuration()
            break
        except usb.core.USBError as err:
            last_err = err
            if getattr(err, "errno", None) not in (16, 19):
                raise
            time.sleep(0.4)
            try:
                if dev.is_kernel_driver_active(0):
                    try:
                        dev.detach_kernel_driver(0)
                    except Exception:
                        pass
            except Exception:
                pass
    else:
        raise RuntimeError(f"USB configuration busy: {last_err}")

    intf = dev.get_active_configuration()[(0, 0)]

    ep_out = usb.util.find_descriptor(
        intf,
        custom_match=lambda e: usb.util.endpoint_direction(e.bEndpointAddress)
        == usb.util.ENDPOINT_OUT,
    )
    ep_in = usb.util.find_descriptor(
        intf,
        custom_match=lambda e: usb.util.endpoint_direction(e.bEndpointAddress)
        == usb.util.ENDPOINT_IN,
    )

    if ep_out is None or ep_in is None:
        raise RuntimeError("USB endpoints not found")

    return dev, ep_out, ep_in


def wait_for_printer(timeout: float = 10.0):
    deadline = time.time() + timeout
    last_err = None
    while time.time() < deadline:
        try:
            return find_printer()
        except Exception as err:
            last_err = err
            time.sleep(0.5)
    raise RuntimeError(f"GoDEX G500 not ready: {last_err}")


def send(dev, ep_out, ep_in, command, read=False, pause=0.12):
    if isinstance(command, str):
        command = command.encode("cp1251", "replace")
    if not command.endswith(b"\r\n"):
        command += b"\r\n"

    dev.write(ep_out.bEndpointAddress, command, timeout=2000)
    time.sleep(pause)

    if not read:
        return ""

    try:
        data = dev.read(ep_in.bEndpointAddress, 512, timeout=1200)
        return bytes(data).decode("latin1", "replace").strip()
    except Exception:
        return ""


def write_raw(dev, ep_out, payload: bytes, chunk_size: int = 4096) -> None:
    for offset in range(0, len(payload), chunk_size):
        chunk = payload[offset : offset + chunk_size]
        dev.write(ep_out.bEndpointAddress, chunk, timeout=2000)


def recover(dev, ep_out, ep_in):
    for command in RECOVERY_SEQUENCE:
        send(dev, ep_out, ep_in, command, pause=0.3)
    return send(dev, ep_out, ep_in, "~S,STATUS", read=True)


def calibrate(dev, ep_out, ep_in):
    attempts = [
        [
            "~S,ESG",
            "^AD",
            "^XSET,IMMEDIATE,1",
            "^XSET,ACTIVERESPONSE,1",
            "~S,CANCEL",
            "~S,SENSOR",
        ],
        [
            "~S,ESG",
            "^AD",
            "^XSET,IMMEDIATE,1",
            "^XSET,ACTIVERESPONSE,1",
            "~S,CANCEL",
            "~S,SENSOR",
            "~V",
        ],
        [
            "~S,ESG",
            "^AD",
            "^XSET,IMMEDIATE,1",
            "^XSET,ACTIVERESPONSE,1",
            "~Z",
            "~S,CANCEL",
            "~S,SENSOR",
        ],
    ]

    for idx, commands in enumerate(attempts, start=1):
        print(f"calibrate_step: {idx}")
        for command in commands:
            try:
                send(dev, ep_out, ep_in, command, pause=0.35)
            except usb.core.USBError as err:
                if "No such device" not in str(err):
                    raise
                print("reconnect: waiting for printer to re-enumerate after reset")
                dev, ep_out, ep_in = wait_for_printer()
        time.sleep(1.5)
        for _ in range(5):
            try:
                status = send(dev, ep_out, ep_in, "~S,STATUS", read=True)
            except usb.core.USBError as err:
                if "No such device" in str(err):
                    print("reconnect: waiting for printer to re-enumerate after reset")
                    dev, ep_out, ep_in = wait_for_printer()
                    continue
                raise
            if status:
                print(f"calibrate_status: {status}")
                if status.startswith("00,"):
                    return status
            time.sleep(0.4)

    return send(dev, ep_out, ep_in, "~S,STATUS", read=True)


def build_label(
    text: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    center: bool,
    content_x_mm: float,
    content_y_mm: float,
) -> list[str]:
    x = 20
    y = 20
    if content_x_mm >= 0 and content_y_mm >= 0:
        x = mm_to_dots(content_x_mm, dpi)
        y = mm_to_dots(content_y_mm, dpi)
    if center:
        x, y = label_center_dots(label_length_mm, label_width_mm, dpi)
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
        f"AC,{x},{y},1,1,0,0,{text}",
        "E",
    ]


def build_qr_label(
    payload: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    center: bool,
    qr_box_mm: int,
    qr_mul: int,
    content_x_mm: float,
    content_y_mm: float,
) -> list[str]:
    # EZPL QR syntax from the GoDEX manual:
    # Wx,y,mode,type,ec,mask,mul,len,rotation
    # The payload follows as a separate line.
    #
    # Mode 2 = alphanumeric, type 2 = enhanced QR, ec=L, mask=8 auto.
    x = 10
    y = 10
    if content_x_mm >= 0 and content_y_mm >= 0:
        x = mm_to_dots(content_x_mm, dpi)
        y = mm_to_dots(content_y_mm, dpi)
    if center:
        cx, cy = label_center_dots(label_length_mm, label_width_mm, dpi)
        box = max(1, mm_to_dots(qr_box_mm, dpi))
        x = max(0, cx - box // 2)
        y = max(0, cy - box // 2)
    # `mul` controls QR module size.
    mul = max(1, qr_mul)
    ec = "L"
    mask = 8
    mode = 2
    qtype = 1
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
        f"W{x},{y},{mode},{qtype},{ec},{mask},{mul},{len(payload)},0",
        payload,
        "E",
    ]


def build_pack_label(
    company_name: str,
    product_name: str,
    qty_text: str,
    barcode: str,
    batch_code: str,
    qr_payload: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    safe_margin_mm: float,
) -> list[str]:
    company_name = sanitize_label_text(company_name)
    product_name = sanitize_label_text(product_name)
    qty_text = normalize_kg_value(qty_text)
    barcode = sanitize_label_text(barcode)
    batch_code = sanitize_label_text(batch_code)
    qr_payload = sanitize_label_text(qr_payload or batch_code)

    label_width_dots = mm_to_dots(label_width_mm, dpi)
    label_length_dots = mm_to_dots(label_length_mm, dpi)
    safe_margin_dots = mm_to_dots(safe_margin_mm, dpi)
    left_x = safe_margin_dots
    gap_dots = mm_to_dots(3.0, dpi)
    line_step = mm_to_dots(5.0, dpi)
    company_y = safe_margin_dots
    item_y = company_y + line_step
    qr_box_mm = max(16.0, min(20.0, label_width_mm * 0.30))
    qr_box_dots = mm_to_dots(qr_box_mm, dpi)
    qr_x = max(left_x, label_width_dots - safe_margin_dots - qr_box_dots)
    qr_x = min(label_width_dots - qr_box_dots, qr_x + mm_to_dots(1.0, dpi))
    barcode_y = max(item_y + line_step * 3, label_length_dots - safe_margin_dots - mm_to_dots(12.0, dpi))
    qr_mul = 5
    text_width_dots = max(1, qr_x - left_x - gap_dots)
    product_lines = wrap_text_for_width(product_name, text_width_dots, dpi, x_mul=1)
    qty_y = item_y + (len(product_lines) * line_step)
    qr_y = max(safe_margin_dots + line_step * 2, qty_y + line_step)
    barcode_text_y = barcode_y + mm_to_dots(8.0, dpi)
    barcode_text_x_mul = 2
    barcode_text_width_dots = max(1, len(barcode) * 14 * barcode_text_x_mul)
    barcode_text_x = max(
        left_x,
        left_x + ((label_width_dots - left_x - safe_margin_dots) - barcode_text_width_dots) // 2,
    )

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
        f"AC,{left_x},{company_y},1,1,0,0,company name: {company_name}",
        f"AC,{left_x + 1},{company_y + 1},1,1,0,0,company name: {company_name}",
        f"AC,{left_x},{item_y},1,1,0,0,item name: {product_lines[0]}",
        f"AC,{left_x},{qty_y},1,1,0,0,kg: {qty_text}",
    ]
    for idx, line in enumerate(product_lines[1:], start=1):
        commands.append(f"AC,{left_x},{item_y + (idx * line_step)},1,1,0,0,{line}")
    commands.extend([
        f"BA,{left_x},{barcode_y},1,2,42,0,0,{barcode}",
        f"AC,{barcode_text_x},{barcode_text_y},{barcode_text_x_mul},1,0,0,{barcode}",
        f"W{qr_x},{qr_y},2,1,L,8,{qr_mul},{len(qr_payload)},0",
        qr_payload,
    ])
    if batch_code:
        batch_y = min(label_length_dots - safe_margin_dots, barcode_text_y + line_step)
        commands.append(f"AC,{left_x},{batch_y},1,1,0,0,{batch_code}")
    commands.append("E")
    return commands


def print_text(
    dev,
    ep_out,
    ep_in,
    text: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    center: bool,
    content_x_mm: float,
    content_y_mm: float,
):
    for command in build_label(
        text,
        label_length_mm,
        label_gap_mm,
        label_width_mm,
        dpi,
        center,
        content_x_mm,
        content_y_mm,
    ):
        send(dev, ep_out, ep_in, command)

    time.sleep(1.0)
    return send(dev, ep_out, ep_in, "~S,STATUS", read=True)


def print_qr(
    dev,
    ep_out,
    ep_in,
    payload: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    center: bool,
    qr_box_mm: int,
    qr_mul: int,
    content_x_mm: float,
    content_y_mm: float,
):
    for command in build_qr_label(
        payload,
        label_length_mm,
        label_gap_mm,
        label_width_mm,
        dpi,
        center,
        qr_box_mm,
        qr_mul,
        content_x_mm,
        content_y_mm,
    ):
        send(dev, ep_out, ep_in, command)

    time.sleep(1.0)
    return send(dev, ep_out, ep_in, "~S,STATUS", read=True)


def print_pack(
    dev,
    ep_out,
    ep_in,
    company_name: str,
    product_name: str,
    qty_text: str,
    barcode: str,
    batch_code: str,
    qr_payload: str,
    label_length_mm: int,
    label_gap_mm: int,
    label_width_mm: int,
    dpi: int,
    safe_margin_mm: float,
):
    for command in build_pack_label(
        company_name,
        product_name,
        qty_text,
        barcode,
        batch_code,
        qr_payload,
        label_length_mm,
        label_gap_mm,
        label_width_mm,
        dpi,
        safe_margin_mm,
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

    if args.calibrate:
        print("calibrate: running sensor calibration")
        final_status = calibrate(dev, ep_out, ep_in)
        print(f"final_status: {final_status or '(empty)'}")
        return 0

    if status and not status.startswith("00,") and not args.skip_recover:
        print("recover: running recovery sequence")
        status = recover(dev, ep_out, ep_in)
        print(f"recovered: {status or '(empty)'}")

    if args.qr:
        payload = args.text if args.text != "TEST" else random_qr_payload()
        print(f"qr_payload: {payload}")
        final_status = print_qr(
            dev,
            ep_out,
            ep_in,
            payload,
            args.label_length_mm,
            args.label_gap_mm,
            args.label_width_mm,
            args.dpi,
            args.center,
            args.qr_box_mm,
            args.qr_mul,
            args.content_x_mm,
            args.content_y_mm,
        )
    elif args.pack_label:
        batch_code = sanitize_label_text(args.batch_code)
        qr_payload = sanitize_label_text(args.qr_payload or args.barcode or batch_code)
        print(f"batch_code: {batch_code}")
        print(f"qr_payload: {qr_payload}")
        final_status = print_pack(
            dev,
            ep_out,
            ep_in,
            args.company_name,
            args.product_name,
            args.qty_text,
            args.barcode or qr_payload,
            batch_code,
            qr_payload,
            args.label_length_mm,
            args.label_gap_mm,
            args.label_width_mm,
            args.dpi,
            args.safe_margin_mm,
        )
    else:
        final_status = print_text(
            dev,
            ep_out,
            ep_in,
            args.text,
            args.label_length_mm,
            args.label_gap_mm,
            args.label_width_mm,
            args.dpi,
            args.center,
            args.content_x_mm,
            args.content_y_mm,
        )
    print(f"final_status: {final_status or '(empty)'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
