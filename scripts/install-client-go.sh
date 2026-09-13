#!/bin/sh
set -eu

SERVICE=nodestatus-client-go
OLD_SERVICE=nodestatus-client
REPO=${REPO:-Sheldonsix/nodestatus-go}
VERSION=latest
TARGET=
DSN=
SERVER=
USERNAME=
PASSWORD=
CUSTOM=complete-go-client
INTERVAL=1.5
BIN=/usr/local/bin/nodestatus-client
RUNNER=/usr/local/bin/nodestatus-client-go-run
CONFIG_DIR=/etc/nodestatus-client-go
CONFIG_FILE=$CONFIG_DIR/client.env

die() {
  echo "error: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  sh install-client-go.sh --dsn ws://user:pass@example.com
  sh install-client-go.sh --server https://example.com --username node --password pass

Options:
  --dsn VALUE        ws(s)://username:password@host
  --server VALUE     http(s):// or ws(s):// server
  --username VALUE   client username
  --password VALUE   client password
  --custom VALUE     custom status text
  --interval VALUE   report interval in seconds
  --repo VALUE       GitHub repo, default: Sheldonsix/nodestatus-go
  --version VALUE    release tag, default: latest
  --target VALUE     asset target override, for example linux-amd64
EOF
}

for arg in "$@"; do
  case "$arg" in
    -h|--help)
      usage
      exit 0
      ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  if command -v sudo >/dev/null 2>&1 && [ -f "$0" ]; then
    exec sudo sh "$0" "$@"
  fi
  die "please run as root"
fi

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dsn)
      [ "$#" -ge 2 ] || die "--dsn requires a value"
      DSN=$2
      shift 2
      ;;
    --server)
      [ "$#" -ge 2 ] || die "--server requires a value"
      SERVER=$2
      shift 2
      ;;
    --username)
      [ "$#" -ge 2 ] || die "--username requires a value"
      USERNAME=$2
      shift 2
      ;;
    --password)
      [ "$#" -ge 2 ] || die "--password requires a value"
      PASSWORD=$2
      shift 2
      ;;
    --custom)
      [ "$#" -ge 2 ] || die "--custom requires a value"
      CUSTOM=$2
      shift 2
      ;;
    --interval)
      [ "$#" -ge 2 ] || die "--interval requires a value"
      INTERVAL=$2
      shift 2
      ;;
    --repo)
      [ "$#" -ge 2 ] || die "--repo requires a value"
      REPO=$2
      shift 2
      ;;
    --version)
      [ "$#" -ge 2 ] || die "--version requires a value"
      VERSION=$2
      shift 2
      ;;
    --target)
      [ "$#" -ge 2 ] || die "--target requires a value"
      TARGET=$2
      shift 2
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

if [ -z "$DSN" ]; then
  [ -n "$SERVER" ] && [ -n "$USERNAME" ] && [ -n "$PASSWORD" ] || die "set --dsn or --server/--username/--password"
fi

detect_target() {
  [ "$(uname -s)" = "Linux" ] || die "only Linux/OpenWrt is supported"

  case "$(uname -m)" in
    x86_64|amd64) echo linux-amd64 ;;
    i386|i686) echo linux-386 ;;
    aarch64|arm64) echo linux-arm64 ;;
    armv5*|armv5tel) echo linux-armv5 ;;
    armv6*|armv6l) echo linux-armv6 ;;
    armv7*|armv7l) echo linux-armv7 ;;
    mips) echo openwrt-mips ;;
    mipsel|mipsle) echo openwrt-mipsle ;;
    mips64) echo openwrt-mips64 ;;
    mips64el|mips64le) echo openwrt-mips64le ;;
    *) die "unsupported arch: $(uname -m), pass --target manually" ;;
  esac
}

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$2" "$1"
  else
    die "curl or wget is required"
  fi
}

sq() {
  printf "'"
  printf "%s" "$1" | sed "s/'/'\\\\''/g"
  printf "'"
}

write_var() {
  printf "%s=" "$1"
  sq "$2"
  printf "\n"
}

install_binary() {
  target=${TARGET:-$(detect_target)}
  archive="nodestatus-client-${target}.tar.gz"
  if [ "$VERSION" = "latest" ]; then
    url="https://github.com/$REPO/releases/latest/download/$archive"
  else
    url="https://github.com/$REPO/releases/download/$VERSION/$archive"
  fi

  work=$(mktemp -d "${TMPDIR:-/tmp}/nodestatus-client.XXXXXX")
  trap 'rm -rf "$work"' EXIT

  download "$url" "$work/$archive"
  tar -xzf "$work/$archive" -C "$work"
  [ -f "$work/nodestatus-client" ] || die "archive does not contain nodestatus-client"

  mkdir -p "$(dirname "$BIN")"
  cp "$work/nodestatus-client" "$BIN.tmp"
  chmod 0755 "$BIN.tmp"
  mv "$BIN.tmp" "$BIN"
}

write_config() {
  mkdir -p "$CONFIG_DIR"
  {
    write_var NODESTATUS_DSN "$DSN"
    write_var NODESTATUS_SERVER "$SERVER"
    write_var NODESTATUS_USERNAME "$USERNAME"
    write_var NODESTATUS_PASSWORD "$PASSWORD"
    write_var NODESTATUS_CUSTOM "$CUSTOM"
    write_var NODESTATUS_INTERVAL "$INTERVAL"
  } > "$CONFIG_FILE"
  chmod 0600 "$CONFIG_FILE"

  cat > "$RUNNER" <<'EOF'
#!/bin/sh
set -eu

. /etc/nodestatus-client-go/client.env

if [ -n "${NODESTATUS_DSN:-}" ]; then
  exec /usr/local/bin/nodestatus-client -dsn "$NODESTATUS_DSN" -custom "${NODESTATUS_CUSTOM:-complete-go-client}" -interval "${NODESTATUS_INTERVAL:-1.5}"
fi

exec /usr/local/bin/nodestatus-client -server "$NODESTATUS_SERVER" -username "$NODESTATUS_USERNAME" -password "$NODESTATUS_PASSWORD" -custom "${NODESTATUS_CUSTOM:-complete-go-client}" -interval "${NODESTATUS_INTERVAL:-1.5}"
EOF
  chmod 0755 "$RUNNER"
}

disable_old_service() {
  if [ -x "/etc/init.d/$OLD_SERVICE" ]; then
    "/etc/init.d/$OLD_SERVICE" stop >/dev/null 2>&1 || true
    "/etc/init.d/$OLD_SERVICE" disable >/dev/null 2>&1 || true
  fi

  if command -v systemctl >/dev/null 2>&1; then
    systemctl stop "$OLD_SERVICE" >/dev/null 2>&1 || true
    systemctl disable "$OLD_SERVICE" >/dev/null 2>&1 || true
  fi
}

install_systemd() {
  cat > "/etc/systemd/system/$SERVICE.service" <<EOF
[Unit]
Description=NodeStatus Go Client
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$RUNNER
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable "$SERVICE"
  systemctl restart "$SERVICE"
}

install_openwrt() {
  cat > "/etc/init.d/$SERVICE" <<EOF
#!/bin/sh /etc/rc.common
START=99
USE_PROCD=1

start_service() {
  procd_open_instance
  procd_set_param command $RUNNER
  procd_set_param respawn
  procd_close_instance
}
EOF

  chmod 0755 "/etc/init.d/$SERVICE"
  "/etc/init.d/$SERVICE" enable
  "/etc/init.d/$SERVICE" restart
}

install_binary
write_config
disable_old_service

if [ -f /etc/openwrt_release ]; then
  install_openwrt
elif command -v systemctl >/dev/null 2>&1; then
  install_systemd
else
  die "no supported init system found: need systemd or OpenWrt procd"
fi

echo "$SERVICE installed"
