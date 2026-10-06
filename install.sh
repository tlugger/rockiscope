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

# ── Back up bot data ────────────────────────────────────────────────
# Upgrades never touch these files, but a copy costs nothing and the season's
# prediction history can't be regenerated.

DATA_FILES=(prediction_history.json season_state.json last_post_date last_reply_date archive)
EXISTING=()
for f in "${DATA_FILES[@]}"; do
  [ -e "$INSTALL_DIR/$f" ] && EXISTING+=("$f")
done

if [ ${#EXISTING[@]} -gt 0 ]; then
  step "Backing up bot data"
  mkdir -p "$INSTALL_DIR/backups"
  BACKUP="$INSTALL_DIR/backups/data-$(date +%Y%m%d-%H%M%S).tar.gz"
  tar -czf "$BACKUP" -C "$INSTALL_DIR" "${EXISTING[@]}"
  ok "Saved ${EXISTING[*]} → $BACKUP"
  # Keep the 10 most recent backups.
  ls -1t "$INSTALL_DIR"/backups/data-*.tar.gz 2>/dev/null | tail -n +11 | xargs -r rm -f || true
fi

# ── Stage the new binary ────────────────────────────────────────────
# The running service keeps going until a verified binary is ready.

STAGED="$INSTALL_DIR/rockiscope.new"
rm -f "$STAGED"

if [ "$CURR_DIR" != "$INSTALL_DIR" ] && [ -f "$CURR_DIR/rockiscope" ]; then
  step "Using local binary"
  cp "$CURR_DIR/rockiscope" "$STAGED"
  ok "Staged binary from current directory"
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

  # ── Get the binary ─────────────────────────────────────────────────
  step "Getting rockiscope binary"

  RELEASE_JSON=$(curl -sf "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null || true)
  DOWNLOAD_URL=$(echo "$RELEASE_JSON" \
    | grep "browser_download_url.*linux-${GOARCH}\"" \
    | head -1 \
    | cut -d '"' -f 4 || true)

  if [ -n "$DOWNLOAD_URL" ]; then
    echo "  📦 Found release binary"
    (curl -sfL --retry 3 -o "$STAGED" "$DOWNLOAD_URL") &
    spin $! "Downloading binary" || fail "Download failed. The running bot was not touched."

    SUM_URL=$(echo "$RELEASE_JSON" | grep "browser_download_url.*linux-${GOARCH}.sha256" | head -1 | cut -d '"' -f 4 || true)
    if [ -n "$SUM_URL" ] && command -v sha256sum &>/dev/null; then
      EXPECTED=$(curl -sfL --retry 3 "$SUM_URL" | awk '{print $1}' || true)
      ACTUAL=$(sha256sum "$STAGED" | awk '{print $1}')
      if [ -z "$EXPECTED" ] || [ "$EXPECTED" != "$ACTUAL" ]; then
        rm -f "$STAGED"
        fail "Checksum mismatch. The running bot was not touched."
      fi
      ok "Checksum verified"
    else
      warn "No checksum available, skipping verification"
    fi
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

    VERSION=$(git -C "$TMPDIR/rockiscope" describe --tags --always 2>/dev/null || echo "dev")

    (cd "$TMPDIR/rockiscope" && go build -ldflags "-X main.version=$VERSION" -o "$STAGED" . 2>&1) &
    spin $! "Building binary" || fail "Build failed. The running bot was not touched."
  fi
fi

chmod +x "$STAGED"
if ! NEW_VERSION=$("$STAGED" version 2>&1); then
  rm -f "$STAGED"
  fail "New binary won't run on this machine. The running bot was not touched."
fi
ok "Staged $NEW_VERSION"

# ── Swap it in ──────────────────────────────────────────────────────

step "Installing binary"

if [ -f "$INSTALL_DIR/rockiscope" ]; then
  OLD_VERSION=$("$INSTALL_DIR/rockiscope" version 2>/dev/null || echo "unknown version")
fi

if systemctl is-active "$SERVICE_NAME" &>/dev/null; then
  systemctl stop "$SERVICE_NAME"
  ok "Stopped running service"
fi

if [ -f "$INSTALL_DIR/rockiscope" ]; then
  mv -f "$INSTALL_DIR/rockiscope" "$INSTALL_DIR/rockiscope.prev"
  ok "Kept previous binary as rockiscope.prev (${OLD_VERSION:-unknown})"
fi
mv -f "$STAGED" "$INSTALL_DIR/rockiscope"
ok "Binary installed to $INSTALL_DIR/rockiscope"

# ── systemd service ─────────────────────────────────────────────────

step "Setting up systemd service"

cat > "/etc/systemd/system/${SERVICE_NAME}.service" << EOF
[Unit]
Description=Rockiscope - Rockies Horoscope Bot
After=network-online.target time-sync.target
Wants=network-online.target time-sync.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
EnvironmentFile=$INSTALL_DIR/.env
Environment="ROCKISCOPE_DATA_DIR=$INSTALL_DIR"
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

  # Give it a moment, then make sure it stayed up. If not, put the old
  # binary back so the bot never ends up dead after an upgrade.
  sleep 10
  if systemctl is-active "$SERVICE_NAME" &>/dev/null; then
    ok "Service started"
  elif [ -f "$INSTALL_DIR/rockiscope.prev" ]; then
    warn "New version failed to stay running. Rolling back."
    tail -n 20 "$INSTALL_DIR/rockiscope.log" 2>/dev/null | sed 's/^/     /' || true
    mv -f "$INSTALL_DIR/rockiscope.prev" "$INSTALL_DIR/rockiscope"
    systemctl restart "$SERVICE_NAME"
    fail "Rolled back to ${OLD_VERSION:-the previous version}. Check $INSTALL_DIR/rockiscope.log"
  else
    fail "Service failed to start. Check $INSTALL_DIR/rockiscope.log"
  fi

  PHASE=$(cd "$INSTALL_DIR" && ROCKISCOPE_DATA_DIR="$INSTALL_DIR" "$INSTALL_DIR/rockiscope" phase 2>/dev/null | grep '^Phase:' | awk '{print $2}' || true)
  [ -n "$PHASE" ] && ok "Bot mode: $PHASE"
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
echo "    $INSTALL_DIR/rockiscope phase      # show the current mode"
echo "    $INSTALL_DIR/rockiscope preview    # preview what would post (dry run)"
echo "    $INSTALL_DIR/rockiscope post        # force post now"
echo ""
echo "  Maybe this is our year. Probably not. 🏔️"
echo ""