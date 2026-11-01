#!/usr/bin/env bash
# Pull the latest code, rebuild dashd and restart it.
# Usage, on the server: cd ~/dashd && sudo ./deploy/update.sh
set -euo pipefail

cd "$(dirname "$0")/.."
REPO_USER="$(stat -c %U .)"
# sudo resets PATH, so find Go and npm explicitly.
GO="$(command -v go || echo /usr/local/go/bin/go)"
NPM="$(command -v npm)"

echo "==> Pulling latest changes"
sudo -u "$REPO_USER" git pull --ff-only

echo "==> Building the web app"
(cd web && sudo -u "$REPO_USER" "$NPM" ci && sudo -u "$REPO_USER" "$NPM" run build)

echo "==> Building dashd"
(cd agent && sudo -u "$REPO_USER" env CGO_ENABLED=0 "$GO" build -trimpath -ldflags="-s -w" -o ../bin/dashd ./cmd/dashd)

echo "==> Installing and restarting"
install -m 755 bin/dashd /usr/local/bin/dashd
systemctl restart dashd
sleep 2
systemctl --no-pager --lines=5 status dashd
