# Agent Handoff

## Scope

This workspace currently matters across three local codebases:

- `gscale-zebra`
- `erp_scz_db_reader` at `/home/wikki/storage/local.git/erpnext_n1/erp/gscale_erp_read`
- the current mobile app checkout at `/home/wikki/storage/local.git/gscale-zebra/hard_con_v2`

The ERP bench itself is at:

- `/home/wikki/storage/local.git/erpnext_n1/erp`

The mobile app checkout now tracks:

- `origin = https://github.com/WIKKIwk/hard_con_v2.git`

## Current Repo State

- `gscale-zebra`
  - branch: `main`
  - `origin/main` is currently behind local working changes
  - working tree includes code changes for core-owned ERP config, read-service discovery, and mobile ERP setup UX
  - working tree may also look dirty because live processes write log files under `logs/`

- `erp_scz_db_reader`
  - branch: `main`
  - latest pushed commit includes `GET /v1/handshake`
  - the service now advertises itself as `gscale_erp_read`

- `hard_con_v2` checkout
  - path: `/home/wikki/storage/local.git/gscale-zebra/hard_con_v2`
  - branch: `main`
  - working tree includes the latest Server-tab ERP setup simplification and discovery-driven UI

## Active Local Processes Right Now

At the time of writing this handoff, these local processes are still running:

- `make run-dev`
- `polygon-dev`
- `mobileapi-dev`
- `scale` via `script ... /tmp/gscale-zebra/scale-dev.log`
- `make run-bot`
- `go run ./cmd/bot`

If the next agent wants a clean slate, stop them first:

```bash
cd /home/wikki/storage/local.git/gscale-zebra
make stop-dev-services
make stop-bot-services
```

## What Was Completed

### 1. ERP read service was created outside `gscale-zebra`

Location:

- `/home/wikki/storage/local.git/erpnext_n1/erp/gscale_erp_read`

Purpose:

- standalone read-only Go service on the ERP side
- item search
- warehouse shortlist
- item detail (`stock_uom`)
- warehouse detail (`company`)

Important endpoints currently implemented:

- `GET /healthz`
- `GET /v1/items?query=...&limit=...`
- `GET /v1/items/{item_code}`
- `GET /v1/items/{item_code}/warehouses?query=...&limit=...`
- `GET /v1/warehouses/{warehouse}`

Important behavior:

- item search is now filtered by stock
- it only returns items with positive `tabBin.actual_qty`

Relevant commit in service repo:

- `cb27235` `Filter ERP item search by stock`

### 2. Bot reads now use the ERP DB reader

In `gscale-zebra`, bot read-paths were switched so that when `ERP_READ_URL` is set:

- `CheckConnection`
- `SearchItems`
- `SearchItemWarehouses`
- `lookupItemStockUOM`
- `lookupWarehouseCompany`

use the ERP DB reader service instead of direct ERP REST.

Write-paths still stay on ERP REST:

- create draft
- submit draft
- delete draft

Relevant commit:

- `a33971b` `Route bot reads through ERP DB reader`

Local config note:

- `bot/.env` was updated locally so `ERP_READ_URL=http://127.0.0.1:8090`
- this `.env` is not tracked in git

### 3. Polygon was upgraded to feel more like real hardware

`polygon` is no longer just a rigid fixed loop.

It now supports realistic scenarios:

- `batch-flow`
- `idle`
- `stress`
- `calibration`

It also supports runtime scenario switching and better fake zebra logs.

Relevant commits:

- `0baa4eb` `Make polygon feel more like real hardware`
- `2e9d1cb` `Show richer run-dev workflow logs`
- `3762b29` `Trim run-dev output to printer flow`
- `d0b15bc` `Let polygon own simulated print requests`

### 4. Runtime make targets were stabilized and made fresher

Important fixes:

- `make run-dev` startup was stabilized
- bad `script` invocation was fixed
- `stop-bot-services` no longer kills `make run-bot` itself
- tracked runtime targets now try to start from a fresh bridge state

Relevant commits:

- `bd2ee80` `Stabilize make run-dev startup`
- `e5013a0` `Make runtime targets start fresh`
- `00c6379` `Avoid self-kill in stop-bot-services`

### 5. Batch start behavior was tightened

The flow was changed so batch does **not** auto-start after selection anymore.

Current intended behavior:

1. item selection
2. warehouse selection
3. explicit `Batch Start`
4. only then batch loop starts

Also:

- `Material Receipt` callback no longer starts batch at all
- it now only tells the user to press `Batch Start`

Relevant commits:

- `cc4e23d` `Require explicit batch start after selection`
- `4dc076f` `Stop Material Receipt from starting batch`

### 6. Shared batch workflow core and mobile control endpoints landed

The batch orchestration was pulled toward a shared core instead of living only in Telegram bot handlers.

Important recent commits:

- `ed73cb0` `Rewrite agent handoff for current architecture`
- `f10cab5` `Extract shared batch control and workflow core`
- `eeffe24` `Add mobileapi batch control endpoints`
- `05650db` `Wire ERP read service into dev stack`
- `2b3442c` `Refresh scale dev runtime logs`

Current shape:

- the shared batch workflow/core layer is now the intended owner for reusable orchestration
- `mobileapi` now has batch control endpoints for the mobile/web clients
- the Telegram bot should keep getting thinner over time instead of owning every workflow detail

### 7. Mobile app was split into its own checkout and synced with `hard_con_v2`

The mobile app now lives in a dedicated checkout:

- `/home/wikki/storage/local.git/gscale-zebra/hard_con_v2`

Important recent commits in that repo:

- `70dc1e1` `Split dashboard and add control panel`
- `0c75bdc` `Connect mobile app to mobileapi batch control`
- `4a15be1` `Polish control workflow and iOS run target`
- `614bf46` `Fix iPhone Wi-Fi server discovery`
- `990a6fa` `Refine mobile search UX and add iOS IPA workflow`
- `a5ae288` `Polish mobile control flow and remove line tab`

Important behavior now:

- the mobile app uses `mobileapi` batch control endpoints
- iOS Wi-Fi discovery is tuned for local network server detection
- the checkout origin points at `hard_con_v2`
- item selection now uses a modal picker flow instead of a persistent inline list
- warehouse selection now uses the same modal picker pattern
- the `Line` tab was removed; bottom navigation is now `Control` and `Server`
- ping is shown in the app bar on the control screen
- GitHub Actions now includes an unsigned iOS `.ipa` artifact workflow

## Important Current Behavior

### Telegram bot logic owner

Right now the Telegram bot still owns the user-facing orchestration path, but the reusable core is being extracted out:

- wait for stable positive qty
- create EPC
- create ERP draft
- write `print_request`
- wait for print result
- submit/delete draft

This logic currently lives primarily in:

- `/home/wikki/storage/local.git/gscale-zebra/bot/internal/app/callback_handler.go`
- `/home/wikki/storage/local.git/gscale-zebra/bot/internal/batchstate/store.go`

### Mobile API now exposes batch control endpoints

`mobileapi` currently provides:

- health
- handshake
- simple local auth
- profile
- monitor snapshot
- monitor stream
- item search
- warehouse shortlist
- batch state
- batch start
- batch stop

Relevant files:

- `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/server.go`
- `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/config.go`

### Mobile client UI direction

Current mobile UX is intentionally narrower now:

- `Control` is the main operational screen
- `Server` remains for connection and backend health context
- `Line` was removed from the bottom navigation because it was redundant
- product and warehouse are selected through modal pickers
- the control screen should stay compact and avoid persistent suggestion lists in-page

## Most Important Architecture Direction

The intended architecture should become:

- one shared workflow/core layer
- Telegram bot = client
- mobile app = client
- future web UI = client

That means:

- do not keep business workflow logic duplicated in Telegram bot handlers and future mobile handlers
- do not put heavy business logic directly into `mobileapi` HTTP handlers
- first extract orchestration from the bot into a reusable service layer
- then let both bot and `mobileapi` call that service layer

## What The Next Agent Should Do

### Main next task

Keep the shared workflow/core layer as the main orchestration owner, and keep the Telegram bot and mobile app as thin clients over it.

### ERP Setup Direction

This direction is now partially implemented.

Current behavior:

- `mobileapi` starts even when ERP write config is absent
- monitor and non-ERP write features can still come up in that degraded state
- `Batch Start` fails fast with `erp_not_configured` instead of dying late in the workflow
- the mobile app `Server` screen can submit ERP config through:
  - `GET /v1/mobile/setup/status`
  - `POST /v1/mobile/setup/erp`
  - `DELETE /v1/mobile/setup/erp`
- submitted secret fields are cleared from the mobile UI after a successful save

Core config ownership now:

- ERP config is no longer conceptually owned by `bot/.env`
- shared runtime config lives under `config/core.env`
- shared loading logic lives in:
  - `/home/wikki/storage/local.git/gscale-zebra/core/runtimecfg/runtimecfg.go`
- `mobileapi` persistence helpers live in:
  - `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/setup_store.go`

Read-service discovery now:

- mobile UI no longer needs a manual `ERP read URL`
- the server accepts only:
  - `erp_url`
  - `erp_api_key`
  - `erp_api_secret`
- after validating ERP write access, the server auto-discovers the read service
- shared discovery logic lives in:
  - `/home/wikki/storage/local.git/gscale-zebra/core/erpread/discovery.go`
- the read service now exposes:
  - `GET /v1/handshake`
  - expected payload: `{"ok":true,"service":"gscale_erp_read"}`

Bot behavior now:

- bot ERP client also uses the same read-service discovery fallback when `ERP_READ_URL` is not explicitly set
- relevant files:
  - `/home/wikki/storage/local.git/gscale-zebra/bot/internal/erp/client.go`
  - `/home/wikki/storage/local.git/gscale-zebra/bot/internal/erp/stock_entry.go`

Remote production-ish ERP host state:

- on the Fedora mini server, `gscale-erp-read` is now deployed as a systemd service:
  - `gscale-erp-read.service`
- binary path:
  - `/usr/local/bin/gscale-erp-read`
- env file:
  - `/etc/gscale-erp-read.env`
- it is bound to ERP startup via systemd ordering and `PartOf=erpnext-prod.service`
- nginx proxies the service under:
  - `https://erp.accord.uz/gscale-read/...`
  - `https://erp.accord.uz/gscale_erp_read/...`
- reboot was tested end-to-end:
  - `erpnext-prod.service` came up
  - `gscale-erp-read.service` waited for `/login`
  - handshake and item search worked after reboot

### Recommended sequence

1. Commit and push the current `gscale-zebra` core-config and discovery changes.
2. Commit and push the current `hard_con_v2` Server-tab simplification.
3. Decide whether `run-dev` should keep explicit `ERP_READ_URL` wiring or fully rely on discovery by default.
4. If desired, mirror the remote ERP proxy path locally under nginx or Caddy for parity.

### Suggested first control endpoints

- `GET /v1/mobile/items?query=...`
- `GET /v1/mobile/items/{item_code}/warehouses?query=...`
- `GET /v1/mobile/batch/state`
- `POST /v1/mobile/batch/start`
- `POST /v1/mobile/batch/stop`

## Things To Be Careful About

- There are live local processes right now. Stop them before doing runtime debugging.
- `bot/.env` is local and untracked. Do not accidentally commit secrets.
- `config/core.env` is local runtime state. Do not commit it.
- `erp_scz_db_reader` handshake support is already pushed, but local test deploys may still exist on remote hosts.
- The mobile app checkout is inside this repo now at `/home/wikki/storage/local.git/gscale-zebra/hard_con_v2`.
- `Material Receipt` must not start batch anymore. Only `Batch Start` is allowed to do that.
- `hard_con_v2` now has a GitHub Actions workflow at `.github/workflows/ios-unsigned-ipa.yml` that builds an unsigned `.ipa` artifact on push.
- Current mobile batch flow still depends on ERP write config being available server-side, but the provisioning flow for that now exists and is usable.

## Good Starting Files For The Next Agent

- `/home/wikki/storage/local.git/gscale-zebra/bot/internal/app/callback_handler.go`
- `/home/wikki/storage/local.git/gscale-zebra/bot/internal/batchstate/store.go`
- `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/server.go`
- `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/config.go`
- `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/setup_store.go`
- `/home/wikki/storage/local.git/gscale-zebra/core/runtimecfg/runtimecfg.go`
- `/home/wikki/storage/local.git/gscale-zebra/core/erpread/discovery.go`
- `/home/wikki/storage/local.git/erpnext_n1/erp/gscale_erp_read/internal/store/store.go`
- `/home/wikki/storage/local.git/gscale-zebra/hard_con_v2/lib/main.dart`
- `/home/wikki/storage/local.git/gscale-zebra/hard_con_v2/Makefile`
- `/home/wikki/storage/local.git/gscale-zebra/hard_con_v2/.github/workflows/ios-unsigned-ipa.yml`
