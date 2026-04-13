# gscale-zebra Linux release

This package is built for Linux and is tested for Ubuntu/Arch style hosts.

## Contents

- `bin/scale` - scale + zebra workflow worker
- `bin/bot` - telegram + ERP worker
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

Service management:

```bash
sudo systemctl restart gscale-scale.service gscale-bot.service
sudo systemctl status gscale-scale.service gscale-bot.service
```

## Repo mode (without release tar)

If you run directly from repository, you can do:

```bash
cd /home/wikki/local.git/gscale-zebra
make autostart-install
```

This builds binaries, installs systemd units, and enables boot auto-start.
