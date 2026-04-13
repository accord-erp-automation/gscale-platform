# Server Handoff

## Maqsad

Bu fayl keyingi agent serverga tez ulanib, `mobile_server` va ERP holatini
tekshira olishi uchun yozildi.

## Asosiy hostlar

- mini server LAN IP: `192.168.0.112`
- mini server Tailscale host: `fedora`
- mini server Tailscale IP: `100.92.208.128`
- backend domen: `https://core.wspace.sbs`
- ERP domen: `https://erp.accord.uz`

## SSH

### LAN orqali

```bash
ssh wikki@192.168.0.112
```

### Tailscale orqali

```bash
tailscale ssh wikki@fedora
```

Yoki:

```bash
tailscale ssh wikki@100.92.208.128
```

## Login ma'lumotlari

- SSH user: `wikki`
- SSH password: `Aywi2008_`

## Remote papkalar

- ERP bench root: `/home/wikki/erpnext`
- ERP site config: `/home/wikki/erpnext/sites/erpnext.local/site_config.json`
- mobile server deploy root: `/home/wikki/deploy/mobile_server_deploy`
- mobile server binary: `/home/wikki/deploy/mobile_server_deploy/core`
- gscale read test/deploy workspace: `/home/wikki/deploy/gscale_erp_read`
- gscale read binary: `/usr/local/bin/gscale-erp-read`
- gscale read env: `/etc/gscale-erp-read.env`

## Remote DB

Remote `site_config.json` ichidan:

- db name: `_6e638c125763c447`
- db user: `_6e638c125763c447`
- db password: `Ih75SyALC9D2ej2I`
- db host: `127.0.0.1`
- db port: `3306`

## cloudflared

- tunnel id: `b8a4874e-0519-4d7d-a648-4772c91023c5`
- credentials file: `/home/wikki/.cloudflared/b8a4874e-0519-4d7d-a648-4772c91023c5.json`

## systemd service'lar

- `mobile-server-core.service`
- `mobile-server-tunnel.service`
- `mobile-server-watchdog.timer`
- `mobile-server-watchdog.service`
- `gscale-erp-read.service`

Eski unitlar:

- `accord-mobile-core.service`
- `accord-mobile-tunnel.service`

ular `disabled` bo'lishi kerak.

## systemd holatini tekshirish

```bash
echo 'Aywi2008_' | sudo -S systemctl status mobile-server-core.service --no-pager
echo 'Aywi2008_' | sudo -S systemctl status mobile-server-tunnel.service --no-pager
echo 'Aywi2008_' | sudo -S systemctl status mobile-server-watchdog.timer --no-pager
echo 'Aywi2008_' | sudo -S systemctl status gscale-erp-read.service --no-pager
```

## Restart

```bash
echo 'Aywi2008_' | sudo -S systemctl restart mobile-server-core.service
echo 'Aywi2008_' | sudo -S systemctl restart mobile-server-tunnel.service
```

## Watchdog log

```bash
echo 'Aywi2008_' | sudo -S journalctl -u mobile-server-watchdog.service -n 100 --no-pager
```

## Core log

```bash
echo 'Aywi2008_' | sudo -S journalctl -u mobile-server-core.service -n 100 --no-pager
```

## Tunnel log

```bash
echo 'Aywi2008_' | sudo -S journalctl -u mobile-server-tunnel.service -n 100 --no-pager
```

## Health check

### Public

```bash
curl -sS https://core.wspace.sbs/healthz
```

### LAN

```bash
curl -sS http://192.168.0.112:8081/healthz
```

### Remote localhost

```bash
curl -sS http://127.0.0.1:8081/healthz
```

### ERP read localhost

```bash
curl -sS http://127.0.0.1:8090/healthz
curl -sS http://127.0.0.1:8090/v1/handshake
```

### ERP read public

```bash
curl -k -sS https://erp.accord.uz/gscale-read/v1/handshake
curl -k -sS 'https://erp.accord.uz/gscale-read/v1/items?limit=1'
```

## Werka login test

Local `.env` bo'yicha dev Werka login:

- phone: `888862440`
- code: `20JHV3XPHFFN`

Test:

```bash
curl -sS \
  -H 'Content-Type: application/json' \
  -d '{"phone":"888862440","code":"20JHV3XPHFFN"}' \
  https://core.wspace.sbs/v1/mobile/auth/login
```

## Muhim endpointlar

- `GET /healthz`
- `POST /v1/mobile/auth/login`
- `GET /v1/mobile/werka/summary`
- `GET /v1/mobile/werka/home`
- `GET /v1/mobile/werka/history`
- `GET /v1/mobile/customer/summary`
- `GET /v1/mobile/customer/status-details?kind=pending`
- `GET /v1/mobile/admin/settings`
- `GET /gscale-read/v1/handshake`
- `GET /gscale-read/v1/items?query=...&limit=...`

## gscale_erp_read deploy

The ERP read service is now deployed on this Fedora server as a systemd unit.

Current live shape:

- systemd unit: `gscale-erp-read.service`
- binary: `/usr/local/bin/gscale-erp-read`
- env file: `/etc/gscale-erp-read.env`
- local bind: `127.0.0.1:8090`
- nginx proxy paths:
  - `/gscale-read/...`
  - `/gscale_erp_read/...`

Important behavior:

- the unit is `enabled`
- it is ordered after:
  - `network-online.target`
  - `mariadb.service`
  - `supervisord.service`
  - `erpnext-prod.service`
- it also has `PartOf=erpnext-prod.service`
- it waits for `http://127.0.0.1/login` before starting the read service binary

Useful commands:

```bash
echo 'Aywi2008_' | sudo -S systemctl status gscale-erp-read.service --no-pager -l
echo 'Aywi2008_' | sudo -S journalctl -u gscale-erp-read.service -n 100 --no-pager
echo 'Aywi2008_' | sudo -S systemctl restart gscale-erp-read.service
echo 'Aywi2008_' | sudo -S systemctl show gscale-erp-read.service -p After -p Wants -p PartOf -p WantedBy
```

Reboot verification already performed:

- the whole Fedora server was rebooted
- `erpnext-prod.service` came back first
- `gscale-erp-read.service` came back after ERP login became reachable
- local handshake and public `/gscale-read` handshake both worked after reboot

## Deploy yo'li

Local repo:

- source: `/home/wikki/storage/local.git/erpnext_stock_telegram/mobile_server`
- build output: `/home/wikki/storage/local.git/erpnext_stock_telegram/mobile_server/build/core`

Build:

```bash
cd /home/wikki/storage/local.git/erpnext_stock_telegram/mobile_server
go build -o build/core ./cmd/core
```

Binary'ni serverga chiqarish:

```bash
scp /home/wikki/storage/local.git/erpnext_stock_telegram/mobile_server/build/core \
  wikki@192.168.0.112:/home/wikki/deploy/mobile_server_deploy/core.new
```

Remote joyiga qo'yish va restart:

```bash
mv /home/wikki/deploy/mobile_server_deploy/core.new /home/wikki/deploy/mobile_server_deploy/core
echo 'Aywi2008_' | sudo -S systemctl restart mobile-server-core.service
echo 'Aywi2008_' | sudo -S systemctl restart mobile-server-tunnel.service
```

## Hozirgi muhim commitlar

- `2d21147` `Harden mobile server systemd recovery`
- `166780c` `Add mobile server watchdog timer`
- `686deb1` `Fix mobile server systemd pre-start cleanup`
- `108abee` `Fall back customer summary to canonical delivery notes`
- `0514dff` `Fall back werka summary and home to canonical sources`
- `3292025` `Bound werka login bootstrap latency`
- `223a8de` `Fall back werka history to canonical events`
- `8cacb6f` `Limit werka history reader to recent events`
- `e4f5859` `Use indexed ordering for werka recent feeds`
- `1bf884d` `Bound werka reader latency for summary and history`
- `0b5a811` `Use minimal status queries for werka summary`
- `52c6133` `Fix werka delivery note reader schema mismatch`

## Oxirgi muhim root cause

`werka/history` reader `Delivery Note` jadvalidagi mavjud bo'lmagan
`dn.remarks` ustunini o'qiyotgan edi. Shu schema mismatch reader'ni yiqitib,
endpointni sekin fallback yo'liga tushirayotgan edi.

Fix:

- `52c6133` `Fix werka delivery note reader schema mismatch`

## Eslatma

Bu repo ichida user o'zi o'chirgan fayl bor:

- `mobile_server/WERKA_NOTIFICATIONS_AND_RECENT_PLAN.md`

Uni qayta tiklamaslik kerak.
