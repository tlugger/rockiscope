#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/home/pi/rockiscope"
REPO="tlugger/rockiscope"
SERVICE_NAME="rockiscope"
GO_MIN_VERSION="1.21"

CURR_DIR="$(pwd)"
LOCAL_DIR="$(cd "$(dirname "$0")" && pwd)"

spin() {
  local pid=$1 msg=$2
  local frames=("⠋" "⠙" "⠹" "⠸" "⠼" "⠴" "⠦" "⠧" "⠇" "⠏")
  local i=0
  while kill -0 "$pid" 2>/dev/null; do
    printf "\r  %s %s" "${frames[$((i % 10))]}" "$msg"
    i=$((i + 1))
    sleep 0.1
  done
  wait "$pid" && printf "\r  ✅ %s\n" "$msg" || { printf "\r  ❌ %s\n" "$msg"; return 1; }
}

step() { echo ""; echo "── $1 ──"; }
ok()   { echo "  ✅ $1"; }
warn() { echo "  ⚠️  $1"; }
fail() { echo "  ❌ $1"; exit 1; }

# ── Banner ───────────────────────────────────────────────────────────

mkdir -p "$INSTALL_DIR"

if [ -f "$INSTALL_DIR/rockiscope" ]; then
  echo ""
  echo "  ⚾ Rockiscope Updater"
  echo "  ─────────────────────"
  echo "  Updating existing installation"
else
  echo ""
  echo "  ⚾ Rockiscope Installer"
  echo "  ───────────────────────"
  echo "  Horoscopes & predictions for the Colorado Rockies"
fi
echo ""

# ── Local .env ──────────────────────────────────────────────────────

if [ "$CURR_DIR" != "$INSTALL_DIR" ] && [ -f "$CURR_DIR/.env" ]; then
  step "Loading .env"
  cp "$CURR_DIR/.env" "$INSTALL_DIR/.env"
  chmod 600 "$INSTALL_DIR/.env"
  ok "Copied .env from current directory"
elif [ -f "$INSTALL_DIR/.env" ]; then
  ok "Using existing .env"
else
  step "Bluesky configuration"
  cat > "$INSTALL_DIR/.env" << 'EOF'
BLUESKY_USERNAME=yourname.bsky.social
BLUESKY_PASSWORD=your-app-password
EOF
  chmod 600 "$INSTALL_DIR/.env"
  ok "Created $INSTALL_DIR/.env with placeholder values"
  warn "Edit .env with your real Bluesky credentials"
  echo "     Create an app password at: https://bsky.app/settings/app-passwords"
  NEEDS_CREDS=1
fi

# ── Local binary ────────────────────────────────────────────────────

if [ "$CURR_DIR" != "$INSTALL_DIR" ] && [ -f "$CURR_DIR/rockiscope" ]; then
  step "Installing local binary"

  if systemctl is-active "$SERVICE_NAME" &>/dev/null; then
    systemctl stop "$SERVICE_NAME"
    ok "Stopped running service"
  fi

  cp "$CURR_DIR/rockiscope" "$INSTALL_DIR/rockiscope"
  chmod +x "$INSTALL_DIR/rockiscope"
  SKIP_BINARY=1

  ok "Installed from current directory"
else
  # ── Architecture ─────────────────────────────────────────────────
  step "Detecting system"

  ARCH=$(uname -m)
  case "$ARCH" in
    aarch64|arm64) GOARCH="arm64" ;;
    armv7l|armhf)  GOARCH="arm" ;;
    x86_64)        GOARCH="amd64" ;;
    *)             fail "Unsupported architecture: $ARCH" ;;
  esac
  ok "Architecture: $ARCH → linux/$GOARCH"

  # Stop running service before replacing
  if systemctl is-active "$SERVICE_NAME" &>/dev/null; then
    systemctl stop "$SERVICE_NAME"
    ok "Stopped running service"
  fi

  # ── Get the binary ─────────────────────────────────────────────────
  step "Getting rockiscope binary"

  DOWNLOAD_URL=$(curl -sf "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
    | grep "browser_download_url.*linux.*${GOARCH}" \
    | head -1 \
    | cut -d '"' -f 4 || true)

  if [ -n "$DOWNLOAD_URL" ]; then
    echo "  📦 Found release binary"
    (curl -sfL -o "$INSTALL_DIR/rockiscope" "$DOWNLOAD_URL") &
    spin $! "Downloading binary"
    chmod +x "$INSTALL_DIR/rockiscope"
  else
    echo "  📦 No release found — building from source"

    if ! command -v git &>/dev/null; then
      fail "git is required to build from source. Install it with: sudo apt install git"
    fi

    if ! command -v go &>/dev/null; then
      warn "Go not found — installing via apt"
      (sudo apt-get update -qq && sudo apt-get install -y -qq golang-go) &
      spin $! "Installing Go"
    fi

    GO_VERSION=$(go version | grep -oP '\d+\.\d+' | head -1)
    ok "Go $GO_VERSION found"

    TMPDIR=$(mktemp -d)
    trap "rm -rf $TMPDIR" EXIT

    (git clone --depth 1 "https://github.com/$REPO.git" "$TMPDIR/rockiscope" 2>/dev/null) &
    spin $! "Cloning repository"

    (cd "$TMPDIR/rockiscope" && go build -o "$INSTALL_DIR/rockiscope" . 2>&1) &
    spin $! "Building binary"

    chmod +x "$INSTALL_DIR/rockiscope"
  fi
fi

ok "Binary installed to $INSTALL_DIR/rockiscope"

# Verify it runs
if "$INSTALL_DIR/rockiscope" version &>/dev/null; then
  VERSION=$("$INSTALL_DIR/rockiscope" version 2>&1)
  ok "$VERSION"
else
  warn "Binary version check failed — continuing anyway"
fi

# ── systemd service ─────────────────────────────────────────────────

step "Setting up systemd service"

cat > "/etc/systemd/system/${SERVICE_NAME}.service" << EOF
[Unit]
Description=Rockiscope - Rockies Horoscope Bot
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
EnvironmentFile=$INSTALL_DIR/.env
ExecStart=$INSTALL_DIR/rockiscope
Restart=always
RestartSec=30
StandardOutput=append:$INSTALL_DIR/rockiscope.log
StandardError=append:$INSTALL_DIR/rockiscope.log

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null 2>&1
ok "Service enabled"

if [ "${NEEDS_CREDS:-}" = "1" ]; then
  warn "Service installed but not started — update .env first"
else
  systemctl restart "$SERVICE_NAME"
  ok "Service started"
fi

# ── Done ───────────────────────────────────────────────────────────

echo ""
if [ "${NEEDS_CREDS:-}" = "1" ]; then
  echo "  ⚾ Rockiscope installed! ⚾"
  echo ""
  echo "  Next steps:"
  echo "    1. Edit $INSTALL_DIR/.env with your Bluesky credentials"
  echo "    2. sudo systemctl start rockiscope"
else
  echo "  ⚾ Rockiscope is live! ⚾"
fi
echo ""
echo "  Commands:"
echo "    sudo systemctl status rockiscope     # check status"
echo "    sudo systemctl restart rockiscope   # restart"
echo "    tail -f $INSTALL_DIR/rockiscope.log # view logs"
echo "    $INSTALL_DIR/rockiscope preview    # preview today's post"
echo "    $INSTALL_DIR/rockiscope post        # force post now"
echo ""
echo "  Maybe this is our year. Probably not. 🏔️"
echo ""