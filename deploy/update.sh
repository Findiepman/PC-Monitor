#!/usr/bin/env bash
# Pull the latest code, rebuild dashd and restart it.
# Run from the repo checkout on the server: sudo ./deploy/update.sh
set -euo pipefail

cd "$(dirname "$0")/.."
REPO_USER="$(stat -c %U .)"

echo "Pulling latest changes"
sudo -u "$REPO_USER" git pull --ff-only

echo "Building frontend"
(cd web && sudo -u "$REPO_USER" npm ci && sudo -u "$REPO_USER" npm run build)

echo "Building dashd"
(cd agent && sudo -u "$REPO_USER" env CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/dashd ./cmd/dashd)

echo "Installing and restarting"
install -m 755 bin/dashd /usr/local/bin/dashd
systemctl restart dashd
sleep 2
systemctl --no-pager --lines=5 status dashd
