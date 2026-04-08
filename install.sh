#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/home/pi/rockiscope"
REPO="tlugger/rockiscope"
SERVICE_NAME="rockiscope"

echo "=== Rockiscope Installer ==="
echo ""

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  aarch64|arm64) GOARCH="arm64" ;;
  armv7l|armhf)  GOARCH="arm" ;;
  x86_64)        GOARCH="amd64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac
echo "Detected architecture: $ARCH (Go: linux/$GOARCH)"

# Create install directory
echo ""
echo "Installing to $INSTALL_DIR..."
mkdir -p "$INSTALL_DIR"

# Download latest release binary
echo "Downloading latest release..."
DOWNLOAD_URL=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" \
  | grep "browser_download_url.*linux.*${GOARCH}" \
  | head -1 \
  | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
  echo "Could not find a release binary for linux/$GOARCH."
  echo "You may need to build from source:"
  echo "  GOOS=linux GOARCH=$GOARCH go build -o rockiscope"
  echo "  scp rockiscope pi@<your-pi>:$INSTALL_DIR/"
  echo ""
  echo "Continuing with setup (binary not downloaded)..."
else
  curl -L -o "$INSTALL_DIR/rockiscope" "$DOWNLOAD_URL"
  chmod +x "$INSTALL_DIR/rockiscope"
  echo "Binary downloaded successfully."
fi

# Collect Bluesky credentials
echo ""
echo "--- Bluesky Configuration ---"
echo "You need a Bluesky app password."
echo "Create one at: https://bsky.app/settings/app-passwords"
echo ""

read -rp "Bluesky username (e.g. yourname.bsky.social): " BS_USER
read -rsp "Bluesky app password: " BS_PASS
echo ""

# Write .env file
cat > "$INSTALL_DIR/.env" << EOF
BLUESKY_USERNAME=$BS_USER
BLUESKY_PASSWORD=$BS_PASS
EOF
chmod 600 "$INSTALL_DIR/.env"
echo "Credentials saved to $INSTALL_DIR/.env"

# Install systemd service
echo ""
echo "--- Setting up systemd service ---"

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
systemctl enable "$SERVICE_NAME"
systemctl start "$SERVICE_NAME"

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Rockiscope is now running as a systemd service."
echo ""
echo "Useful commands:"
echo "  sudo systemctl status $SERVICE_NAME    # check status"
echo "  sudo systemctl restart $SERVICE_NAME   # restart"
echo "  sudo systemctl stop $SERVICE_NAME      # stop"
echo "  tail -f $INSTALL_DIR/rockiscope.log    # view logs"
echo "  $INSTALL_DIR/rockiscope --dry-run      # test without posting"
echo ""
echo "Go Rockies!"
