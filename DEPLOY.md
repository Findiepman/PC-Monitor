# Deploying dashd (and surviving the 12-hour restarts)

This guide goes from an empty GitHub repo to `https://dash.findiepman.dev`, set up so the whole stack comes back by itself after every reboot. Commands marked **PC** run on your Windows machine; everything else runs on the server over SSH.

What survives a reboot, and why:

| Piece | Comes back because | State kept in |
| --- | --- | --- |
| dashd | `systemctl enable`, `Restart=always` | `/var/lib/dashd` (sessions, audit log) |
| Login sessions | saved to `sessions.json` | `/var/lib/dashd/sessions.json` |
| Config and secrets | plain files | `/etc/dashd/` |
| cloudflared tunnel | installed as a service | `/etc/cloudflared/` |
| Docker containers | restart policy `unless-stopped` | Docker |
| Wings and game servers | `wings.service` enabled; Wings starts servers that were running | Wings |
| playit | `playit.service` enabled | playit |

The CPU graph starts empty after a reboot, since it only keeps five minutes in memory. That's expected.

---

## 1. Put the code on GitHub (PC)

Create an **empty private** repo on GitHub, without a README or .gitignore, then:

```powershell
cd C:\Users\fdiep\Desktop\programeren\JavaScript\dashboard
git init -b main
git add .
git update-index --chmod=+x deploy/update.sh
git status            # check: no dev/, config.yaml, sessions.json, *.jsonl or node_modules
git commit -m "dashd: server dashboard"
git remote add origin git@github.com:findiepman/dashd.git
git push -u origin main
```

What `.gitignore` keeps out of the repo:
- anything with credentials: `config.yaml`, `.env` files, `sessions.json`, `*.jsonl`, tunnel credentials
- your local `dev/` folder, which holds a mock login and TOTP secret
- build output (`bin/`, the built frontend) and `node_modules`

`agent/internal/webui/dist/.gitkeep` **is** committed on purpose. Without it `go build` fails on a fresh clone.

> If you ever commit a real `config.yaml` by accident, deleting it in a later commit isn't enough because it stays in the history. Generate a new password and TOTP secret with `dashd init`.

## 2. Prepare the server (once)

dashd builds on the server, so it needs Go 1.26 or newer and Node 20 or newer. The Ubuntu/Debian `golang` package is too old, so install Go from go.dev.

```bash
# Go
curl -LO https://go.dev/dl/go1.26.0.linux-amd64.tar.gz   # or the latest from go.dev/dl
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.*.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile && source ~/.profile

# Node (NodeSource LTS)
curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
sudo apt install -y nodejs git

go version && node -v
```

**Make sure the clock syncs at boot.** Login codes are time-based: if the clock is more than about 30 seconds off after a reboot, every code will be rejected.

```bash
sudo timedatectl set-ntp true
timedatectl            # "System clock synchronized: yes"
```

## 3. Clone and build

Clone into your home directory. The service never runs from here; it runs the installed copy.

```bash
cd ~
git clone git@github.com:findiepman/dashd.git     # set up a deploy key or use HTTPS + a token
cd dashd
(cd web && npm ci && npm run build)
(cd agent && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/dashd ./cmd/dashd)
sudo install -m 755 bin/dashd /usr/local/bin/dashd
```

## 4. Install dashd as a service

```bash
# System user that owns the state directory
sudo useradd --system --home /var/lib/dashd --shell /usr/sbin/nologin dashd

# Config: creates your login and prints a QR code to scan with your authenticator app
sudo mkdir -p /etc/dashd
sudo dashd init -o /etc/dashd/config.yaml
```

Edit `/etc/dashd/config.yaml` with `sudo nano /etc/dashd/config.yaml`. Use `deploy/config.example.yaml` as the reference. These parts matter for reboots:

```yaml
auth:
  # ...username, password_hash, totp_secret from init...
  session_ttl: 168h                           # stay signed in for a week, across reboots
  sessions_path: /var/lib/dashd/sessions.json

audit:
  path: /var/lib/dashd/audit.jsonl

providers:
  docker: {}
  systemd:
    units: [wings, playit, pteroq, cloudflared]   # use your real unit names
  pterodactyl:
    url: https://panel.findiepman.dev
    api_key: env:DASHD_PTERODACTYL_KEY
```

Check the real unit names with `systemctl list-units --type=service | grep -Ei 'wings|playit|ptero|cloudflared'`.

Secrets go in their own file:

```bash
sudo tee /etc/dashd/env >/dev/null <<'EOF'
DASHD_PTERODACTYL_KEY=ptlc_your_client_api_key
EOF
sudo chown root:root /etc/dashd/env && sudo chmod 600 /etc/dashd/env
sudo chown root:dashd /etc/dashd/config.yaml && sudo chmod 640 /etc/dashd/config.yaml
```

Permissions and the service:

```bash
# Let dashd control only the allowlisted units (edit the list to match your config)
sudo cp deploy/50-dashd.rules /etc/polkit-1/rules.d/50-dashd.rules
sudo systemctl restart polkit

# The service itself
sudo cp deploy/dashd.service /etc/systemd/system/dashd.service
sudo systemctl daemon-reload
sudo systemctl enable --now dashd
systemctl status dashd
```

`enable` is what makes it start on every boot, and `Restart=always` brings it back if it crashes.

> `/etc/polkit-1/rules.d` needs polkit 0.106 or newer (Debian 12, Ubuntu 23.04 and later). Check with `pkaction --version`. On older systems, restarting systemd units from the dashboard fails with "Interactive authentication required". Docker and Pterodactyl controls still work.

## 5. Cloudflare Tunnel as a service

The simplest setup that survives reboots is a dashboard-managed tunnel:

1. In Cloudflare Zero Trust, go to **Networks → Tunnels → Create a tunnel** (type *Cloudflared*) and name it `findiepman`.
2. Copy the install command it shows. It looks like `sudo cloudflared service install eyJ...`. Run it on the server. That installs `cloudflared.service`, enabled at boot.
3. Under **Public Hostname**, add subdomain `dash`, domain `findiepman.dev`, type `HTTP`, URL `127.0.0.1:7070`.
4. Optional but recommended: under **Access → Applications**, add `dash.findiepman.dev` with a policy that allows only your email. Anyone else is stopped before they even see the sign-in page.

If you already run a locally configured tunnel instead, add the ingress rule from `deploy/cloudflared.yml` and make sure it's enabled: `sudo systemctl enable cloudflared`.

## 6. Make everything else come back after a reboot

```bash
# Services dashd watches
sudo systemctl enable docker wings playit cloudflared dashd
systemctl is-enabled docker wings playit cloudflared dashd    # all should say "enabled"
```

**Docker containers** only come back if they have a restart policy:

```bash
# See the current policy for every container
docker ps -a --format '{{.Names}}' | xargs -I{} docker inspect -f '{{.Name}} {{.HostConfig.RestartPolicy.Name}}' {}

# Fix any that say "no"
docker update --restart unless-stopped <container-name>
```

In docker-compose files, add `restart: unless-stopped` to each service so recreated containers keep it.

**Pterodactyl game servers** are started by Wings after boot, provided they were running before the reboot. The panel (and its database) must be up too. If you run the panel in Docker, it needs the restart policy above.

## 7. Test a reboot before trusting it

```bash
sudo reboot
# wait a minute, SSH back in
systemctl --failed                          # should list nothing relevant
systemctl status dashd cloudflared --no-pager
journalctl -u dashd -b --no-pager | tail -20
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7070/   # 200
```

Then open `https://dash.findiepman.dev` on your phone over mobile data. You should **still be signed in** from before the reboot, and every service should show a green light.

If you were signed out: check that `/var/lib/dashd/sessions.json` exists and that `sessions_path` is set, and look for `could not save sessions` in `journalctl -u dashd`.

## 8. Updating later

Push from your PC, then on the server:

```bash
cd ~/dashd && sudo ./deploy/update.sh
```

It pulls, rebuilds the frontend and binary, installs it and restarts dashd. You stay signed in.

If you change `deploy/dashd.service` or `deploy/50-dashd.rules`, copy them again, then run `sudo systemctl daemon-reload && sudo systemctl restart dashd` (or restart polkit for the rules). `update.sh` doesn't touch files under `/etc`.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| Every code is "wrong" after a reboot | Clock not synced yet. Run `timedatectl`, then wait for NTP. |
| Signed out after every reboot | `sessions_path` not set, or `/var/lib/dashd` isn't writable (check `StateDirectory=dashd` in the unit). |
| Docker group shows "socket not reachable" | dashd isn't in the `docker` group. Check that `SupplementaryGroups=docker` is in the unit and that Docker is running. |
| systemd restart says "Interactive authentication required" | The polkit rule is missing, lists a different unit name, or polkit is too old. |
| Services logs say "No journal files were found" | Missing `systemd-journal` group. It's set in the unit; run `daemon-reload` after copying. |
| Pterodactyl group says the panel rejected the API key | Use a **client** key (`ptlc_`), not an application key (`ptla_`). |
| Tunnel shows a Cloudflare 502 page | dashd isn't running, or the public hostname points somewhere other than `127.0.0.1:7070`. |
