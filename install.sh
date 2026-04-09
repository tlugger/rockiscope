#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/home/pi/rockiscope"
REPO="tlugger/rockiscope"
SERVICE_NAME="rockiscope"
GO_MIN_VERSION="1.21"

# ── Helpers ──────────────────────────────────────────────────────────

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

IS_UPDATE=0
if [ -f "$INSTALL_DIR/rockiscope" ]; then
  IS_UPDATE=1
fi

echo ""
if [ "$IS_UPDATE" = "1" ]; then
  echo "  ⚾ Rockiscope Updater"
  echo "  ─────────────────────"
  echo "  Updating existing installation"
else
  echo "  ⚾ Rockiscope Installer"
  echo "  ───────────────────────"
  echo "  Horoscopes & predictions for the Colorado Rockies"
fi
echo ""

# ── Architecture ─────────────────────────────────────────────────────

step "Detecting system"

ARCH=$(uname -m)
case "$ARCH" in
  aarch64|arm64) GOARCH="arm64" ;;
  armv7l|armhf)  GOARCH="arm" ;;
  x86_64)        GOARCH="amd64" ;;
  *)             fail "Unsupported architecture: $ARCH" ;;
esac
ok "Architecture: $ARCH → linux/$GOARCH"

# ── Install directory ────────────────────────────────────────────────

mkdir -p "$INSTALL_DIR"
ok "Install directory: $INSTALL_DIR"

# ── Get the binary ───────────────────────────────────────────────────

# Stop running service before replacing binary
if systemctl is-active "$SERVICE_NAME" &>/dev/null; then
  systemctl stop "$SERVICE_NAME"
  ok "Stopped running service"
fi

step "Getting rockiscope binary"

# Try GitHub release first
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

  # Check for git
  if ! command -v git &>/dev/null; then
    fail "git is required to build from source. Install it with: sudo apt install git"
  fi

  # Check for Go
  if ! command -v go &>/dev/null; then
    warn "Go not found — installing via apt"
    (sudo apt-get update -qq && sudo apt-get install -y -qq golang-go) &
    spin $! "Installing Go"
  fi

  GO_VERSION=$(go version | grep -oP '\d+\.\d+' | head -1)
  ok "Go $GO_VERSION found"

  # Clone and build
  TMPDIR=$(mktemp -d)
  trap "rm -rf $TMPDIR" EXIT

  (git clone --depth 1 "https://github.com/$REPO.git" "$TMPDIR/rockiscope" 2>/dev/null) &
  spin $! "Cloning repository"

  (cd "$TMPDIR/rockiscope" && go build -o "$INSTALL_DIR/rockiscope" . 2>&1) &
  spin $! "Building binary"

  chmod +x "$INSTALL_DIR/rockiscope"
fi

ok "Binary installed to $INSTALL_DIR/rockiscope"

# Verify it runs
if "$INSTALL_DIR/rockiscope" version &>/dev/null; then
  VERSION=$("$INSTALL_DIR/rockiscope" version 2>&1)
  ok "$VERSION"
else
  warn "Binary built but version check failed — continuing anyway"
fi

# ── Bluesky credentials ─────────────────────────────────────────────

step "Bluesky configuration"

# Check if we can prompt interactively
CAN_PROMPT=0
if [ -t 0 ]; then
  CAN_PROMPT=1
fi

if [ -f "$INSTALL_DIR/.env" ]; then
  echo "  📄 Existing .env found"
  if [ "$CAN_PROMPT" = "1" ]; then
    read -rp "  Overwrite credentials? [y/N] " overwrite
    if [[ ! "$overwrite" =~ ^[Yy]$ ]]; then
      ok "Keeping existing credentials"
      SKIP_CREDS=1
    fi
  else
    ok "Keeping existing credentials (non-interactive)"
    SKIP_CREDS=1
  fi
fi

if [ "${SKIP_CREDS:-}" != "1" ]; then
  if [ "$CAN_PROMPT" != "1" ]; then
    warn "Non-interactive shell — cannot prompt for credentials"
    echo "  Create $INSTALL_DIR/.env manually:"
    echo ""
    echo "    cat > $INSTALL_DIR/.env << 'EOF'"
    echo "    BLUESKY_USERNAME=yourname.bsky.social"
    echo "    BLUESKY_PASSWORD=your-app-password"
    echo "    EOF"
    echo "    chmod 600 $INSTALL_DIR/.env"
    echo ""
    SKIP_CREDS=1
  else
    echo "  🔑 You need a Bluesky app password"
    echo "     Create one at: https://bsky.app/settings/app-passwords"
    echo ""
    read -rp "  Bluesky username (e.g. yourname.bsky.social): " BS_USER
    read -rsp "  Bluesky app password: " BS_PASS
    echo ""

    if [ -z "$BS_USER" ] || [ -z "$BS_PASS" ]; then
      fail "Username and password are required"
    fi

    cat > "$INSTALL_DIR/.env" << EOF
BLUESKY_USERNAME=$BS_USER
BLUESKY_PASSWORD=$BS_PASS
EOF
    chmod 600 "$INSTALL_DIR/.env"
    ok "Credentials saved to $INSTALL_DIR/.env"

    # Test auth
    echo ""
    (cd "$INSTALL_DIR" && source .env && export BLUESKY_USERNAME BLUESKY_PASSWORD && "$INSTALL_DIR/rockiscope" test-auth >/dev/null 2>&1) &
    spin $! "Testing Bluesky authentication" || warn "Auth test failed — check your credentials"
  fi
fi

# ── systemd service ──────────────────────────────────────────────────

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

if [ -f "$INSTALL_DIR/.env" ]; then
  systemctl restart "$SERVICE_NAME"
  ok "Service started"
else
  warn "Service installed but not started — create .env first, then: sudo systemctl start rockiscope"
fi

# ── Done ─────────────────────────────────────────────────────────────

echo ""
echo "  ⚾ Rockiscope is live! ⚾"
echo ""
echo "  Commands:"
echo "    sudo systemctl status rockiscope     # check status"
echo "    sudo systemctl restart rockiscope    # restart"
echo "    tail -f $INSTALL_DIR/rockiscope.log  # view logs"
echo "    $INSTALL_DIR/rockiscope preview      # preview today's post"
echo "    $INSTALL_DIR/rockiscope post         # force post now"
echo ""
echo "  Maybe this is our year. Probably not. 🏔️"
echo ""
