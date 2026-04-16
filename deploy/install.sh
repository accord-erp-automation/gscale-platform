#!/usr/bin/env bash
set -euo pipefail

PREFIX="/opt/gscale-zebra"
APP_USER="${SUDO_USER:-}"
APP_GROUP=""
START_AFTER_INSTALL=0
INSTALL_BOT=0

usage() {
  cat <<'EOF'
Usage: ./install.sh [options]

Install gscale-zebra binaries and systemd services.
Works from either:
- release tar root (install.sh, bin/, config/, systemd/)
- repository deploy dir (expects ../bin from `make build`)

Options:
  --prefix <path>  Install root (default: /opt/gscale-zebra)
  --user <name>    Service user (default: SUDO_USER/current user)
  --group <name>   Service group (default: primary group of user)
  --with-bot       Also enable/start telegram bot service
  --start          Start services after install
  -h, --help       Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix)
      PREFIX="${2:-}"
      shift 2
      ;;
    --user)
      APP_USER="${2:-}"
      shift 2
      ;;
    --group)
      APP_GROUP="${2:-}"
      shift 2
      ;;
    --with-bot)
      INSTALL_BOT=1
      shift
      ;;
    --start)
      START_AFTER_INSTALL=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown arg: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run install.sh as root (example: sudo ./install.sh --start)." >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BIN_DIR=""
CONFIG_DIR=""
UNIT_DIR=""

if [[ -f "${SCRIPT_DIR}/bin/bot" && -f "${SCRIPT_DIR}/config/bot.env.example" && -f "${SCRIPT_DIR}/systemd/gscale-scale.service" ]]; then
  # Release tar layout:
  #   <root>/install.sh
  #   <root>/bin/*
  #   <root>/config/*
  #   <root>/systemd/*
  BIN_DIR="${SCRIPT_DIR}/bin"
  CONFIG_DIR="${SCRIPT_DIR}/config"
  UNIT_DIR="${SCRIPT_DIR}/systemd"
elif [[ -f "${SCRIPT_DIR}/../bin/bot" && -f "${SCRIPT_DIR}/config/bot.env.example" && -f "${SCRIPT_DIR}/systemd/gscale-scale.service" ]]; then
  # Repo layout:
  #   <repo>/deploy/install.sh
  #   <repo>/bin/*
  #   <repo>/deploy/config/*
  #   <repo>/deploy/systemd/*
  BIN_DIR="${SCRIPT_DIR}/../bin"
  CONFIG_DIR="${SCRIPT_DIR}/config"
  UNIT_DIR="${SCRIPT_DIR}/systemd"
else
  echo "Unable to detect install layout. Build binaries first (make build) or use release tar." >&2
  exit 1
fi

required=(
  "${BIN_DIR}/bot"
  "${BIN_DIR}/scale"
  "${BIN_DIR}/mobileapi"
  "${CONFIG_DIR}/bot.env.example"
  "${CONFIG_DIR}/core.env.example"
  "${CONFIG_DIR}/mobileapi.env.example"
  "${CONFIG_DIR}/scale.env.example"
  "${UNIT_DIR}/gscale-bot.service"
  "${UNIT_DIR}/gscale-mobileapi.service"
  "${UNIT_DIR}/gscale-scale.service"
)

for f in "${required[@]}"; do
  if [[ ! -f "$f" ]]; then
    echo "Required file not found: $f" >&2
    exit 1
  fi
done

if [[ -z "${APP_USER}" ]]; then
  APP_USER="$(id -un)"
fi

if ! id -u "${APP_USER}" >/dev/null 2>&1; then
  echo "User does not exist: ${APP_USER}" >&2
  exit 1
fi

if [[ -z "${APP_GROUP}" ]]; then
  APP_GROUP="$(id -gn "${APP_USER}")"
fi

if ! getent group "${APP_GROUP}" >/dev/null 2>&1; then
  echo "Group does not exist: ${APP_GROUP}" >&2
  exit 1
fi

echo "==> Installing to ${PREFIX}"
install -d -m 0755 "${PREFIX}" "${PREFIX}/bin" "${PREFIX}/logs"
install -d -m 0750 "${PREFIX}/config"

install -m 0755 "${BIN_DIR}/bot" "${PREFIX}/bin/bot"
install -m 0755 "${BIN_DIR}/scale" "${PREFIX}/bin/scale"
install -m 0755 "${BIN_DIR}/mobileapi" "${PREFIX}/bin/mobileapi"
if [[ -f "${BIN_DIR}/zebra" ]]; then
  install -m 0755 "${BIN_DIR}/zebra" "${PREFIX}/bin/zebra"
fi

if [[ ! -f "${PREFIX}/config/bot.env" ]]; then
  install -m 0640 "${CONFIG_DIR}/bot.env.example" "${PREFIX}/config/bot.env"
fi
if [[ ! -f "${PREFIX}/config/core.env" ]]; then
  install -m 0640 "${CONFIG_DIR}/core.env.example" "${PREFIX}/config/core.env"
fi
if [[ ! -f "${PREFIX}/config/scale.env" ]]; then
  install -m 0640 "${CONFIG_DIR}/scale.env.example" "${PREFIX}/config/scale.env"
fi

chown -R "${APP_USER}:${APP_GROUP}" "${PREFIX}/logs" "${PREFIX}/config"

escape_sed() {
  printf '%s' "$1" | sed 's/[\\/&]/\\&/g'
}

prefix_esc="$(escape_sed "${PREFIX}")"
user_esc="$(escape_sed "${APP_USER}")"
group_esc="$(escape_sed "${APP_GROUP}")"

render_unit() {
  local in_file="$1"
  local out_file="$2"
  sed \
    -e "s/__PREFIX__/${prefix_esc}/g" \
    -e "s/__APP_USER__/${user_esc}/g" \
    -e "s/__APP_GROUP__/${group_esc}/g" \
    "${in_file}" > "${out_file}"
}

tmp_scale="$(mktemp)"
tmp_bot="$(mktemp)"
tmp_mobileapi="$(mktemp)"
trap 'rm -f "${tmp_scale}" "${tmp_bot}" "${tmp_mobileapi}"' EXIT

render_unit "${UNIT_DIR}/gscale-scale.service" "${tmp_scale}"
render_unit "${UNIT_DIR}/gscale-bot.service" "${tmp_bot}"
render_unit "${UNIT_DIR}/gscale-mobileapi.service" "${tmp_mobileapi}"

install -m 0644 "${tmp_scale}" /etc/systemd/system/gscale-scale.service
install -m 0644 "${tmp_bot}" /etc/systemd/system/gscale-bot.service
install -m 0644 "${tmp_mobileapi}" /etc/systemd/system/gscale-mobileapi.service

echo "==> Reloading systemd"
systemctl daemon-reload
systemctl enable gscale-scale.service gscale-mobileapi.service >/dev/null
if [[ "${INSTALL_BOT}" == "1" ]]; then
  systemctl enable gscale-bot.service >/dev/null
else
  systemctl disable gscale-bot.service >/dev/null 2>&1 || true
  systemctl stop gscale-bot.service >/dev/null 2>&1 || true
fi

if [[ "${START_AFTER_INSTALL}" == "1" ]]; then
  echo "==> Starting services"
  systemctl restart gscale-scale.service gscale-mobileapi.service
  if [[ "${INSTALL_BOT}" == "1" ]]; then
    systemctl restart gscale-bot.service
  fi
fi

echo
echo "Installed."
echo "Config files:"
echo " - ${PREFIX}/config/scale.env"
echo " - ${PREFIX}/config/bot.env"
echo " - ${PREFIX}/config/core.env"
echo " - ${PREFIX}/config/mobileapi.env (optional, only for overrides)"
echo
echo "Useful commands:"
echo " - systemctl status gscale-scale.service"
echo " - systemctl status gscale-mobileapi.service"
echo " - journalctl -u gscale-scale.service -f"
echo " - journalctl -u gscale-mobileapi.service -f"
if [[ "${INSTALL_BOT}" == "1" ]]; then
  echo " - systemctl status gscale-bot.service"
  echo " - journalctl -u gscale-bot.service -f"
fi
