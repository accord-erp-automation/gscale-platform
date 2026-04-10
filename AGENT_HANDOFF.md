# Agent Handoff

## Scope

This workspace currently matters across three local codebases:

- `gscale-zebra`
- `erp_scz_db_reader` at `/home/wikki/storage/local.git/erpnext_n1/erp/gscale_erp_read`
- the current mobile app checkout at `/home/wikki/storage/local.git/erpnext_stock_telegram/mobile_app`

The ERP bench itself is at:

- `/home/wikki/storage/local.git/erpnext_n1/erp`

The mobile app checkout now tracks:

- `origin = git@github.com:WIKKIwk/hard_con_v2.git`
- `legacy-origin = git@github.com:WIKKIwk/ERP_mobile.git`

## Current Repo State

- `gscale-zebra`
  - branch: `main`
  - HEAD: `4dc076f` (`Stop Material Receipt from starting batch`)
  - `origin/main` is the same commit
  - working tree may look dirty because live processes are currently writing log files

- `erp_scz_db_reader`
  - branch: `main`
  - HEAD: `cb27235` (`Filter ERP item search by stock`)
  - ahead of `origin/main` by 1 commit
  - not pushed yet

- `mobile_app` checkout
  - path: `/home/wikki/storage/local.git/erpnext_stock_telegram/mobile_app`
  - branch: `main`
  - HEAD: `70dc1e1` (`Split dashboard and add control panel`)
  - synced with `hard_con_v2`

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

## Important Current Behavior

### Telegram bot logic owner

Right now the Telegram bot still owns the real orchestration logic:

- wait for stable positive qty
- create EPC
- create ERP draft
- write `print_request`
- wait for print result
- submit/delete draft

This logic currently lives primarily in:

- `/home/wikki/storage/local.git/gscale-zebra/bot/internal/app/callback_handler.go`
- `/home/wikki/storage/local.git/gscale-zebra/bot/internal/batchstate/store.go`

### Mobile API is still mostly monitor/read API

`mobileapi` currently provides:

- health
- handshake
- simple local auth
- profile
- monitor snapshot
- monitor stream

It does **not** yet expose real control endpoints for:

- item selection
- warehouse selection
- batch start
- batch stop
- orchestration actions

Relevant files:

- `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/server.go`
- `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/config.go`

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

Use the current `mobileapi` as the HTTP entrypoint for mobile/web control, but only after extracting the batch orchestration into a reusable core/application layer.

### Recommended sequence

1. Extract a shared batch workflow service out of bot orchestration code.
2. Keep Telegram bot as a thin client over that service.
3. Add control endpoints to `mobileapi` for:
   - item search
   - warehouse shortlist
   - batch state
   - batch start
   - batch stop
4. Make the mobile app consume those endpoints.

### Suggested first control endpoints

- `GET /v1/mobile/items?query=...`
- `GET /v1/mobile/items/{item_code}/warehouses?query=...`
- `GET /v1/mobile/batch/state`
- `POST /v1/mobile/batch/start`
- `POST /v1/mobile/batch/stop`

## Things To Be Careful About

- There are live local processes right now. Stop them before doing runtime debugging.
- `bot/.env` is local and untracked. Do not accidentally commit secrets.
- `erp_scz_db_reader` has an unpushed commit (`cb27235`). Keep that in mind if the next agent changes the service again.
- The mobile app checkout is not inside this repo; it lives at `/home/wikki/storage/local.git/erpnext_stock_telegram/mobile_app`.
- `Material Receipt` must not start batch anymore. Only `Batch Start` is allowed to do that.

## Good Starting Files For The Next Agent

- `/home/wikki/storage/local.git/gscale-zebra/bot/internal/app/callback_handler.go`
- `/home/wikki/storage/local.git/gscale-zebra/bot/internal/batchstate/store.go`
- `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/server.go`
- `/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/config.go`
- `/home/wikki/storage/local.git/erpnext_n1/erp/gscale_erp_read/internal/store/store.go`
