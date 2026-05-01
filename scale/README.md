# Scale monitor (headless + Zebra + GoDEX) 📟

`scale` is the device-facing runtime that reads the live weight stream, keeps
printer state in the bridge snapshot, and executes pending `print_request`
commands.

## Hardware Support

`scale` is built and tested for Linux hosts only when real hardware is involved.
The printer-specific code uses Linux USB/serial access and Linux build tags.

Supported printer families:

- Zebra RFID / label printers that speak ZPL and the RFID commands used by this
  repository.
- Verified Zebra families in the current docs and code path: `ZT411 RFID`,
  `ZT421 RFID`.
- GoDEX `G500 / G530` family through the GoDEX USB backend.

Supported host architectures:

- `amd64`
- `arm64`

Practical host recommendation:

- low-power Linux mini-PC or industrial PC
- stable USB host controller
- 1 to 2 CPU cores
- about 1 GB RAM
- no GPU requirement

Printer power:

- the printer is powered by its own OEM supply
- the host PC does not power the printer
- Zebra `ZT411 RFID` official specs list an auto-switching `100-240V` power
  supply

## What It Does

1. Serial scale reading is auto-detected from `/dev/serial/by-id/*`,
   `ttyUSB*`, or `ttyACM*`.
2. If serial is unavailable, HTTP bridge fallback can be used.
3. Each reading updates the shared bridge snapshot.
4. When `batch.active=true`, the ERP-first workflow is allowed to progress.
5. Pending `print_request` entries are executed by the configured printer
   backend.

## Print Backends

### Zebra

- Uses the Zebra runtime path.
- Suitable for RFID encode + label print flows.
- Expects a Zebra printer that understands ZPL and the RFID/SGD commands used
  by the encoder path.

### GoDEX

- Uses the GoDEX runtime path.
- Suitable for label-only or pack-label flows.
- The current code path targets the GoDEX `G500` USB device family.
- Direct USB communication uses the GoDEX `195f:0001` device path.

## Print Request Rules

- `print_request.status = pending` means the worker can pick it up.
- If the same EPC was already printed, the request is marked `done`.
- If the selected backend is disabled, the request becomes `error`.
- Encode work progresses `processing -> done/error`.
- `print_request.mode = rfid` is the default.
- `print_request.mode = label` skips RFID write and still prints the label.

## Batch Gate

- default state file: `/tmp/gscale-zebra/bridge_state.json`
- flag: `--bridge-state-file /tmp/gscale-zebra/bridge_state.json`

Bot or mobile batch control toggles:

- `Material Receipt` -> `batch.active=true`
- `Batch Stop` -> `batch.active=false`

## Startup

```bash
cd /home/wikki/storage/local.git/gscale-platform/scale
go run .
```

The runtime is headless, so there is no TUI.

## Systemd

From the repo root:

```bash
make autostart-install
```

This installs and enables the systemd services for the scale worker and bot.

## Key Flags

- `--device` - serial device path, example `/dev/ttyUSB0`
- `--baud` - main serial baud rate, default `9600`
- `--baud-list` - probe baud list, default `9600,19200,38400,57600,115200`
- `--probe-timeout` - port probe timeout, default `800ms`
- `--unit` - default unit, default `kg`
- `--bridge-url` - fallback endpoint, default `http://127.0.0.1:18000/api/v1/scale`
- `--bridge-interval` - fallback poll interval, default `120ms`
- `--no-bridge` - disable HTTP fallback
- `--zebra-device` - Zebra printer path, example `/dev/usb/lp0`
- `--zebra-interval` - Zebra monitor interval, default `900ms`
- `--no-zebra` - disable Zebra monitor and printer actions
- `--printer` - print backend, `zebra` or `godex`
- `--godex-company` - GoDEX label company name, default `Accord`
- `--godex-brutto` - GoDEX label brutto text, default `5kg`
- `--bot-dir` - bot module path, default `../bot`
- `--no-bot` - disable bot auto-start
- `--bridge-state-file` - shared snapshot file

## Log Files

Worker logs are written to `../logs/scale/`.
Each restart starts a new session and refreshes the log set.
