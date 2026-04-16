# gscale-zebra Linux release

This package is built for Linux and is tested for Ubuntu/Arch style hosts.

## Contents

- `bin/scale` - scale + zebra workflow worker
- `bin/bot` - telegram + ERP worker (optional)
- `bin/mobileapi` - mobile API worker
- `bin/zebra` - zebra diagnostic utility
- `config/*.env.example` - config templates
- `systemd/*.service` - service templates
- `install.sh` - install helper

## Quick install

```bash
tar -xzf gscale-zebra-<version>-linux-<arch>.tar.gz
cd gscale-zebra-<version>-linux-<arch>
sudo ./install.sh --start
```

Then set real credentials:

- `config/bot.env` (telegram token)
- `config/core.env` (shared core ERP creds)
- `config/scale.env` (device paths)
- `config/mobileapi.env` (mobile API bind/server overrides)

Default service management:

```bash
sudo systemctl restart gscale-scale.service gscale-mobileapi.service
sudo systemctl status gscale-scale.service gscale-mobileapi.service
```

## Repo mode (without release tar)

If you run directly from repository, you can do:

```bash
cd /home/wikki/local.git/gscale-zebra
make autostart-install
```

This builds binaries, installs scale + mobileapi systemd units, and enables boot auto-start.

Optional bot install:

```bash
make autostart-install-bot
```

`mobileapi.env` optional:
- if missing, `mobileapi` default bind/config values bilan ishga tushadi
- faqat port/server name override kerak bo'lsa yaratiladi
