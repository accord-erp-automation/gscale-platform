# Current GScale Context

## Topology

- `make run-dev` runs on this laptop.
- The ERP side is the `v16` server reachable via `https://erp.accord.uz`.
- The mobile app talks to the laptop's `mobileapi` during local dev.
- The laptop's `mobileapi` talks to ERP and `gscale-read` on the `v16` side.

Important correction:

- There is no separate "mobileapi PC" in the current test flow.
- The only remote machine that matters for ERP data is the `v16` server / mini PC.

## Main Errors We Hit

### 1. ERP setup from mobile app failed with:

```text
Exception: open /opt/gscale-zebra/config/core.env.tmp: permission denied
```

Root cause:

- `mobileapi` systemd service was running as user `wikki`.
- `/opt/gscale-zebra/config` had root-owned permissions.
- Saving ERP setup writes `core.env.tmp` next to `core.env`.
- Because the service user could not write into that directory, save failed.

Relevant code:

- [internal/mobileapi/setup_store.go](/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/setup_store.go)
- [core/runtimecfg/runtimecfg.go](/home/wikki/storage/local.git/gscale-zebra/core/runtimecfg/runtimecfg.go)
- [deploy/install.sh](/home/wikki/storage/local.git/gscale-zebra/deploy/install.sh)

Fix:

- Changed deploy install so `/opt/gscale-zebra/config` is owned by the service user and writable enough for atomic `*.tmp` saves.

### 2. Mobile app showed wrong server behavior during dev

Root cause:

- On the laptop, both of these were active at the same time:
  - systemd `gscale-mobileapi.service` on `:39117`
  - `make run-dev` `mobileapi-dev` on another candidate port, e.g. `:41257`
- The mobile app kept connecting to the wrong server, often the systemd one.
- That produced confusing errors and made it look like `run-dev` was broken while the app was actually talking to `/opt/gscale-zebra/...`.

Fix:

- Added a guard in `make run-dev` so it refuses to start if local `gscale-scale.service` or `gscale-mobileapi.service` are still running.

Relevant file:

- [Makefile](/home/wikki/storage/local.git/gscale-zebra/Makefile)

### 3. `run-dev` did not stop fast enough on `Ctrl+C`

Root cause:

- The shell loop was sleeping in 1-second chunks.
- Signal handling and cleanup were not as immediate as desired.

Fix:

- Updated traps for `INT`/`TERM`.
- Reduced the idle loop granularity to 0.2s.

Relevant file:

- [Makefile](/home/wikki/storage/local.git/gscale-zebra/Makefile)

### 4. Item search from mobile app did not show expected products

Example:

- Searching for `xot lanch` in ERP UI shows matching items.
- Mobile app item picker returned `Mahsulot topilmadi.`

What we verified:

- The mobile app sends query almost directly to `/v1/mobile/items`.
- `mobileapi` forwards that query to `gscale-read` / ERP search.
- Direct ERP API search can find the item.
- `gscale-read` production search can return empty for `xot lanch` and can return noisy results for short terms like `hot`.

Likely root cause:

- The issue is not primarily in the mobile app UI.
- The issue is in the ERP read-service search behavior or in the exact deployed binary/config of `gscale-read`.

Relevant files:

- [hard_con_v2/lib/main.dart](/home/wikki/storage/local.git/gscale-zebra/hard_con_v2/lib/main.dart)
- [internal/mobileapi/control_service.go](/home/wikki/storage/local.git/gscale-zebra/internal/mobileapi/control_service.go)
- `/home/wikki/storage/local.git/erpnext_n1/erp/gscale_erp_read/internal/store/store.go`

## Mini PC / Service Work That Was Done

We installed and tested `scale + mobileapi` as services and checked reboot survival.

What was confirmed:

- `gscale-scale.service` can come back after reboot.
- `gscale-mobileapi.service` can come back after reboot.
- `mobileapi /healthz` returned `ok`.

But for the current dev workflow, that service mode on the laptop must not run in parallel with `make run-dev`.

## Current Commits That Matter

Already pushed recently:

- `8b85d8a` `Fix gscale deploy permissions and mobileapi service`
- `95eca9b` `Guard run-dev against local gscale service conflicts`

This file is meant to preserve the reasoning and avoid repeating the same confusion.

## Practical Rules Going Forward

### Local dev

- Use `make run-dev` on the laptop.
- Do not leave local systemd `gscale-scale.service` / `gscale-mobileapi.service` running at the same time.

### Service install

- Use `make run` / `make autostart-install` only when persistent service mode is intended.

### ERP setup save

- If you ever see `/opt/gscale-zebra/config/core.env.tmp: permission denied` again, check ownership and mode of `/opt/gscale-zebra/config`.

### Item search debugging

- Compare these two directly:
  - ERP API result
  - `gscale-read` result
- If ERP finds the item but `gscale-read` does not, debug the read-service, not the app first.
