#!/bin/sh
set -eu

REPO=${REPO:-https://github.com/Sheldonsix/nodestatus-go.git}
BRANCH=${BRANCH:-main}
INSTALL_DIR=${INSTALL_DIR:-/opt/nodestatus-go}
WEB_USERNAME=${WEB_USERNAME:-admin}
WEB_PASSWORD=${WEB_PASSWORD:-}
WEB_SECRET=${WEB_SECRET:-}
BIND=${BIND:-127.0.0.1:35601:35601}

die() {
  echo "error: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  sh scripts/install.sh
  sh scripts/install.sh --password strong-password --bind 0.0.0.0:35601:35601

Options:
  --repo VALUE       git repo, default: https://github.com/Sheldonsix/nodestatus-go.git
  --branch VALUE     git branch, default: main
  --dir VALUE        install dir, default: /opt/nodestatus-go
  --username VALUE   admin username, default: admin
  --password VALUE   admin password, generated when empty
  --secret VALUE     JWT secret, generated when empty
  --bind VALUE       compose port bind, default: 127.0.0.1:35601:35601
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --repo)
      [ "$#" -ge 2 ] || die "--repo requires a value"
      REPO=$2
      shift 2
      ;;
    --branch)
      [ "$#" -ge 2 ] || die "--branch requires a value"
      BRANCH=$2
      shift 2
      ;;
    --dir)
      [ "$#" -ge 2 ] || die "--dir requires a value"
      INSTALL_DIR=$2
      shift 2
      ;;
    --username)
      [ "$#" -ge 2 ] || die "--username requires a value"
      WEB_USERNAME=$2
      shift 2
      ;;
    --password)
      [ "$#" -ge 2 ] || die "--password requires a value"
      WEB_PASSWORD=$2
      shift 2
      ;;
    --secret)
      [ "$#" -ge 2 ] || die "--secret requires a value"
      WEB_SECRET=$2
      shift 2
      ;;
    --bind)
      [ "$#" -ge 2 ] || die "--bind requires a value"
      BIND=$2
      shift 2
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

need() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$1"
  else
    date +%s | sha256sum | cut -c "1-$((2 * $1))"
  fi
}

sudo_cmd() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    die "run as root or install sudo"
  fi
}

compose() {
  if $DOCKER compose version >/dev/null 2>&1; then
    $DOCKER compose "$@"
  elif command -v docker-compose >/dev/null 2>&1; then
    docker-compose "$@"
  else
    die "docker compose is required"
  fi
}

need docker
DOCKER=docker
if ! docker info >/dev/null 2>&1; then
  if command -v sudo >/dev/null 2>&1 && sudo docker info >/dev/null 2>&1; then
    DOCKER="sudo docker"
  else
    die "docker is not running or current user can not access it"
  fi
fi

if [ -f Dockerfile ] && [ -f go.mod ] && [ -f docker-compose.yml ]; then
  APP_DIR=$PWD
else
  need git
  sudo_cmd mkdir -p "$INSTALL_DIR"
  sudo_cmd chown "$(id -u):$(id -g)" "$INSTALL_DIR"
  if [ -d "$INSTALL_DIR/.git" ]; then
    git -C "$INSTALL_DIR" fetch origin "$BRANCH"
    git -C "$INSTALL_DIR" checkout "$BRANCH"
    git -C "$INSTALL_DIR" pull --ff-only
  else
    rmdir "$INSTALL_DIR" 2>/dev/null || true
    git clone --branch "$BRANCH" "$REPO" "$INSTALL_DIR"
  fi
  APP_DIR=$INSTALL_DIR
fi

[ -n "$WEB_PASSWORD" ] || WEB_PASSWORD=$(random_hex 12)
[ -n "$WEB_SECRET" ] || WEB_SECRET=$(random_hex 32)

umask 077
cat > "$APP_DIR/.env" <<EOF
WEB_USERNAME=$WEB_USERNAME
WEB_PASSWORD=$WEB_PASSWORD
WEB_SECRET=$WEB_SECRET
BIND=$BIND
EOF

cd "$APP_DIR"
compose up -d --build

cat <<EOF
NodeStatus Go is running.
Directory: $APP_DIR
Username:  $WEB_USERNAME
Password:  $WEB_PASSWORD
Bind:      $BIND
EOF
