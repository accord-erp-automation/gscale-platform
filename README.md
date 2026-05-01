# GScale Platform

## Abstract
`gscale-platform` is the orchestration and runtime repository of a mobile-first warehouse workflow that connects four operational concerns into one transaction chain:

1. weight acquisition from a scale,
2. RFID and label execution for Zebra printers and GoDEX printers,
3. ERPNext draft creation and submission,
4. operator control through a mobile application.

This repository is one member of a three-repository system:

- `gscale-platform`: runtime orchestration, bridge-state coordination, mobile API, scale worker, simulator, and optional Telegram bot.
- `gscale-erp-read`: read-only ERP catalog service for fast item and warehouse lookups.
- `gscale-mobile-app`: Flutter client used by operators on the shop floor.

In practical terms, this repository is the operational center of the system. It owns the state machine that turns a stable weight reading into a print request and an ERP transaction.

## Repository Ecosystem

| Repository | Primary role | Typical deployment location |
| --- | --- | --- |
| [`gscale-platform`](https://github.com/accord-erp-automation/gscale-platform) | Runtime coordination, bridge state, scale worker, mobile API, simulator | Edge device, operator workstation, or test laptop |
| [`gscale-erp-read`](https://github.com/accord-erp-automation/gscale-erp-read) | Read-only ERP catalog service | ERP-side server or any machine with trusted ERP DB access |
| [`gscale-mobile-app`](https://github.com/WIKKIwk/gscale-mobile-app) | Operator-facing mobile user interface | Android phone, tablet, or Flutter desktop/web dev environment |

Each repository is intentionally narrow in scope. Together they form one application boundary.

## System Context

### Problem Statement
Warehouse operations frequently split scale reading, label printing, and ERP entry into separate manual steps. That creates three predictable risks:

- inconsistent quantities between physical and digital records,
- barcode or EPC mismatches,
- operator delay caused by repeated data entry.

The platform addresses this by treating the workflow as a state-driven transaction pipeline rather than a collection of unrelated tools.

### Architectural Position
This repository sits between the mobile client and the physical devices:

```text
gscale-mobile-app
        |
        v
   mobileapi
        |
        +---------------------> gscale-erp-read -----------------> ERP DB
        |
        v
  bridge_state.json
        ^
        |
      scale worker -----------------------> Zebra printer / RFID
        |
        +---------------------> GoDEX G500/G530 USB printer
        |
        +---------------------> real scale or polygon simulator

cloudflare/scan_label_worker ---------> scan.wspace.sbs QR landing page
```

The `mobileapi` process exposes operator workflows. The `scale` process monitors the physical or simulated scale and fulfills print requests. The `bridge` state file acts as the shared transactional coordination surface. The `gscale-erp-read` companion repository supplies item and warehouse read models without exposing write privileges.

## Scope of This Repository

This repository owns the following responsibilities:

- mobile-facing HTTP API for operator actions and status monitoring,
- real-time scale worker runtime,
- print-request lifecycle management,
- bridge-state persistence and synchronization,
- EPC generation and workflow state transitions,
- development simulator for scale and fake printer behavior,
- optional Telegram bot workflow for non-mobile operation.

This repository deliberately does **not** own:

- the mobile UI implementation itself,
- the ERP-side read-only catalog service implementation,
- ERP business configuration and master data governance.

Those concerns belong to the companion repositories listed above.

## Core Modules

### `cmd/mobileapi` and `internal/mobileapi`
This is the application-layer API consumed by `gscale-mobile-app`. It handles:

- ERP setup and validation,
- default warehouse configuration,
- item and warehouse search delegation,
- batch start and stop operations,
- monitor and archive endpoints.

In production terms, this is the operator-facing control plane.

### `scale`
The scale worker is the device-facing execution engine. It:

- reads scale values from serial devices or a bridge endpoint,
- detects stable positive readings,
- consumes `print_request` instructions from bridge state,
- executes Zebra encode and print operations when Zebra is enabled,
- executes GoDEX pack-label operations when the GoDEX backend is selected,
- reports device status back into shared state.

### `polygon`
`polygon` is the development simulator. It provides:

- fake scale cycles,
- fake printer completion,
- controlled scenarios for repeated batch testing,
- a reproducible environment for end-to-end workflow verification.

It exists to test the full workflow without requiring physical hardware.

### `bridge`
The bridge module is the consistency layer. It stores the shared runtime snapshot at:

`/tmp/gscale-zebra/bridge_state.json`

This file is treated as the single coordination surface between mobile API, scale worker, and optional bot workflows.

### `core`
`core` contains reusable workflow logic:

- stable-reading interpretation,
- EPC generation,
- material receipt orchestration,
- print-request waiting and next-cycle waiting semantics.

### `bot`
The Telegram bot remains in the repository as an alternative operator interface. It is no longer the primary control surface when the mobile-first architecture is used, but it remains a valid runtime path.

### `zebra`
`zebra` is a utility and diagnostics module. It is useful for:

- printer discovery,
- calibration,
- RFID test commands,
- direct device troubleshooting.

The actual runtime print path, however, is executed by the `scale` worker.

### `godex`
`godex` owns the GoDEX G500/G530 pack-label flow. It keeps the direct USB
printer path, EZPL command generation, and host-side QR/text rendering in one
place so the production flow no longer depends on legacy Python scripts.

### `cloudflare/scan_label_worker`
This folder contains the stateless QR landing page used by the printed label.
It receives the URL-shaped QR path and renders the human-readable label page.

### `tools`
`tools/release.sh` builds Linux release tarballs and `tools/scan_label_server.py`
is a local stateless QR preview server for development.

## Supported Hardware

| Layer | Supported / verified | Notes |
| --- | --- | --- |
| Backend host | Linux `amd64` and `arm64` | Real USB/serial hardware access is implemented for Linux; other OSes are not the target runtime for physical devices. |
| GoDEX printer backend | GoDEX G500 / G530 family | Direct USB printer path in `godex/`; the current USB VID/PID path targets GoDEX G500-class devices. |
| Zebra printer backend | Zebra ZT411 RFID, ZT421 RFID | The Zebra path expects ZPL-compatible printers with RFID support where RFID encoding is used. |
| Mobile client | Android phone or tablet | Flutter app; desktop/web is acceptable for development, but Android is the field target. |

## Power And Host Requirements

- Printer power comes from the printer’s own OEM power supply. The host PC does not power the printer.
- Zebra ZT411 RFID official specs list an auto-switching `100-240V` power supply.
- The GoDEX G500/G530 family uses its bundled printer-side PSU/adapter; the repository does not hardcode a wattage requirement.
- Practical host recommendation: a low-power Linux mini-PC or industrial PC with USB host support, 1 to 2 CPU cores, and about 1 GB RAM is sufficient for the backend services. No GPU is required.
- The important host requirement is stable USB/serial access, not raw compute.

## Supported Printer Stack

- Zebra printers are used for RFID encode + label workflows through ZPL and RFID commands.
- GoDEX printers are used for label-only or pack-label workflows through EZPL and host-rendered QR/text graphics.
- The mobile app selects the active printer type from the live bridge snapshot when the batch is idle, so operators do not need to guess which printer is attached.

## Runtime Contracts With Companion Repositories

### Contract With `gscale-erp-read`
This repository consumes the ERP read service as a separate process. The contract is:

- item search by query,
- item detail lookup,
- item-to-warehouse shortlist lookup,
- warehouse detail lookup.

This repository assumes that write operations remain here, while catalog reads are delegated outward. See the companion repository:

`https://github.com/accord-erp-automation/gscale-erp-read`

### Contract With `gscale-mobile-app`
This repository acts as the backend that the mobile client discovers and calls. The mobile application depends on:

- `/healthz`
- `/v1/mobile/handshake`
- `/v1/mobile/items`
- `/v1/mobile/items/{item_code}/warehouses`
- `/v1/mobile/warehouses`
- `/v1/mobile/batch/start`
- `/v1/mobile/batch/stop`
- `/v1/mobile/monitor/state`
- `/v1/mobile/archive`

See the companion repository:

`https://github.com/WIKKIwk/gscale-mobile-app`

## Data Model

The bridge-state snapshot is the operational memory of the system. The main sections are:

- `scale`
- `zebra`
- `batch`
- `print_request`

Conceptually:

- `scale` describes the current measurement,
- `zebra` describes the current printer or RFID state,
- `batch` describes the currently active operator transaction,
- `print_request` describes the pending print command produced by workflow logic.

This repository treats the bridge state as a low-friction integration boundary. That decision simplifies decoupling but increases the importance of careful atomic update logic.

## Execution Modes

### Production-Oriented Runtime
Use the normal runtime when real hardware should be involved:

```bash
make run
```

or explicit service installation:

```bash
make autostart-install
```

This path is intended for persistent systemd-style execution.

### Development Runtime
Development mode launches a clean local stack:

```bash
make run-dev
```

By design, this mode:

- starts `polygon`,
- starts `mobileapi`,
- starts the `scale` worker with `--no-zebra`,
- uses simulator-driven print completion instead of real printer I/O.

If a quieter simulator is needed, auto scale cycles can be disabled:

```bash
make run-dev POLYGON_AUTO=false
```

### Simulator-Only Runtime

```bash
make run-polygon
```

### Scale-Only Runtime

```bash
make run-scale SCALE_DEVICE=/dev/ttyUSB0 ZEBRA_DEVICE=/dev/usb/lp0
```

## Configuration

The central configuration file for ERP-facing operations is:

`config/core.env`

Important keys:

- `ERP_URL`
- `ERP_READ_URL`
- `ERP_API_KEY`
- `ERP_API_SECRET`
- `BRIDGE_STATE_FILE`
- `WAREHOUSE_MODE`
- `DEFAULT_WAREHOUSE`

The mobile API also supports a runtime-local setup file, used heavily in development:

- `MOBILE_API_SETUP_FILE`

## Development Notes

### Why `run-dev` Must Start Clean
The development stack can behave unpredictably if stale `polygon`, `mobileapi`, or `scale` processes remain alive. For that reason, `run-dev` now performs an aggressive cleanup pass before starting a new stack. This prevents duplicated fake scale cycles and duplicated print-request handling.

`run-dev` uses the real ERPNext write configuration from `config/core.env` when it is present, so batch actions can create and submit actual ERP drafts during local testing. If you explicitly want simulated writes for a smoke test, use `make run-dev MOBILE_API_DEV_ERP_WRITE=1`.

### Why `gscale-erp-read` Is Not Embedded Into `run-dev`
The ERP read service is modeled as a companion service, not as an implementation detail of `run-dev`. This mirrors production architecture more accurately and keeps the boundary between runtime orchestration and ERP-side read access explicit.

## Testing Strategy

This repository provides two levels of confidence:

- unit and package tests through `go test ./...`,
- live end-to-end simulation through `run-dev` plus `polygon`.

The companion `gscale-erp-read` repository should also be tested independently because item-catalog behavior is partly defined outside this repository.

## Recommended Reading Order

For a first-time reviewer, the most productive reading order is:

1. this README,
2. `internal/mobileapi`,
3. `core/workflow`,
4. `scale`,
5. `polygon`,
6. the `gscale-erp-read` README,
7. the `gscale-mobile-app` README.

## Companion References

- `gscale-erp-read`: read-only ERP catalog service and item filtering logic.
- `gscale-mobile-app`: operator-facing mobile client and field workflow UX.

Each repository is intentionally incomplete when read in isolation. They are designed to be understood as one coordinated system.
