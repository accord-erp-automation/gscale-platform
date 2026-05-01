#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/dist"
VERSION=""
ARCHES=()

usage() {
  cat <<'EOF'
Usage: tools/release.sh [options]

Build Linux release tarballs for gscale-zebra.

Options:
  --version <v>   Release version tag (default: YYYYMMDD-<git-short>)
  --arch <arch>   Target arch (amd64 or arm64). Can be passed multiple times.
  --out <dir>     Output directory (default: ./dist)
  -h, --help      Show help

Examples:
  ./tools/release.sh --arch amd64
  ./tools/release.sh --arch amd64 --arch arm64 --version v0.2.0
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --arch)
      ARCHES+=("${2:-}")
      shift 2
      ;;
    --out)
      OUT_DIR="${2:-}"
      shift 2
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

if [[ ! -f "${ROOT_DIR}/go.work" ]]; then
  echo "go.work not found. Run from repository root." >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "go command not found in PATH." >&2
  exit 1
fi

if [[ ${#ARCHES[@]} -eq 0 ]]; then
  ARCHES=("amd64")
fi

for arch in "${ARCHES[@]}"; do
  case "$arch" in
    amd64|arm64) ;;
    *)
      echo "Unsupported arch: $arch (allowed: amd64, arm64)" >&2
      exit 1
      ;;
  esac
done

if [[ -z "${VERSION}" ]]; then
  git_short="$(git -C "${ROOT_DIR}" rev-parse --short HEAD 2>/dev/null || echo "nogit")"
  VERSION="$(date +%Y%m%d)-${git_short}"
fi

mkdir -p "${OUT_DIR}"
WORK_DIR="${OUT_DIR}/.stage"
rm -rf "${WORK_DIR}"
mkdir -p "${WORK_DIR}"

artifacts=()

build_binary() {
  local arch="$1"
  local out="$2"
  local pkg="$3"
  CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" \
    go build -trimpath -ldflags="-s -w" -o "${out}" "${pkg}"
}

for arch in "${ARCHES[@]}"; do
  pkg_name="gscale-zebra-${VERSION}-linux-${arch}"
  pkg_dir="${WORK_DIR}/${pkg_name}"

  mkdir -p "${pkg_dir}/bin" "${pkg_dir}/config" "${pkg_dir}/systemd"

  echo "==> Building ${pkg_name}"
  build_binary "${arch}" "${pkg_dir}/bin/bot" "./bot/cmd/bot"
  build_binary "${arch}" "${pkg_dir}/bin/scale" "./scale"
  build_binary "${arch}" "${pkg_dir}/bin/mobileapi" "./cmd/mobileapi"
  build_binary "${arch}" "${pkg_dir}/bin/zebra" "./zebra"

  install -m 0755 "${ROOT_DIR}/deploy/install.sh" "${pkg_dir}/install.sh"
  install -m 0644 "${ROOT_DIR}/deploy/README.md" "${pkg_dir}/README.md"
  install -m 0644 "${ROOT_DIR}/deploy/config/bot.env.example" "${pkg_dir}/config/bot.env.example"
  install -m 0644 "${ROOT_DIR}/deploy/config/core.env.example" "${pkg_dir}/config/core.env.example"
  install -m 0644 "${ROOT_DIR}/deploy/config/mobileapi.env.example" "${pkg_dir}/config/mobileapi.env.example"
  install -m 0644 "${ROOT_DIR}/deploy/config/scale.env.example" "${pkg_dir}/config/scale.env.example"
  install -m 0644 "${ROOT_DIR}/deploy/systemd/gscale-bot.service" "${pkg_dir}/systemd/gscale-bot.service"
  install -m 0644 "${ROOT_DIR}/deploy/systemd/gscale-mobileapi.service" "${pkg_dir}/systemd/gscale-mobileapi.service"
  install -m 0644 "${ROOT_DIR}/deploy/systemd/gscale-scale.service" "${pkg_dir}/systemd/gscale-scale.service"
  printf '%s\n' "${VERSION}" > "${pkg_dir}/VERSION"

  tarball="${OUT_DIR}/${pkg_name}.tar.gz"
  tar -C "${WORK_DIR}" -czf "${tarball}" "${pkg_name}"
  artifacts+=("${tarball}")
done

echo
echo "Release artifacts:"
for artifact in "${artifacts[@]}"; do
  echo " - ${artifact}"
done
