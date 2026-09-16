# dashd

A self-hosted dashboard for one Linux server: live CPU, memory, disk and network, Docker containers and their health, Pterodactyl game servers, systemd services like Wings and playit, logs you can filter and copy, and start/stop/restart for all of it. It's meant to live at `dash.findiepman.dev` behind a Cloudflare Tunnel.

```
browser ──https──> Cloudflare ──tunnel──> cloudflared ──> 127.0.0.1:7070 dashd
```

`dashd` is a single Go binary with the Svelte frontend embedded.

## Layout

```
agent/                    Go agent
  cmd/dashd               entrypoint and subcommands (serve, init, hash-password, totp-code)
  internal/api            REST, websocket hub, embedded SPA
  internal/auth           argon2id password, TOTP, sessions, lockout
  internal/host           CPU/mem/disk/net sampling (plus a mock)
  internal/provider       Unit/Provider contract and registry
  internal/providers/*    docker, systemd, pterodactyl, mock
  internal/audit          JSONL log of every action
web/                      Svelte 5 + Vite frontend, builds into agent/internal/webui/dist
deploy/                   systemd unit, polkit rule, cloudflared and config examples
```

## Develop locally (Windows, macOS or Linux)

Mock mode fakes every provider and the host stats, so there's no need for Docker or systemd.

```sh
cd web && npm install && cd ..
mkdir dev && cd dev
go run ../agent/cmd/dashd init -mock -o config.yaml   # pick a username and password, scan the QR
go run ../agent/cmd/dashd serve -config config.yaml    # terminal 1
cd ../web && npm run dev                               # terminal 2, open http://localhost:5173
```

In mock mode, restarting `nginx` fails every other time so you can see how errors look. `go run ./cmd/dashd totp-code -secret <secret>` prints a current code if your phone isn't nearby.

Tests: `cd agent && go test ./...` and `cd web && npm run check`.

## Deploy to the server

For the full walkthrough (git, first install, surviving reboots, updates), see [DEPLOY.md](DEPLOY.md). The short version:

1. **Build** (on any machine with Go and Node): `make build` creates `bin/dashd` for linux/amd64. Without make, run `cd web && npm run build`, then `cd agent && GOOS=linux GOARCH=amd64 go build -o ../bin/dashd ./cmd/dashd`.
2. **Install:**
   ```sh
   sudo useradd --system --home /var/lib/dashd --shell /usr/sbin/nologin dashd
   sudo install -m 755 dashd /usr/local/bin/dashd
   sudo mkdir -p /etc/dashd
   sudo dashd init -o /etc/dashd/config.yaml        # creates the login and shows the TOTP QR code
   sudo chown root:dashd /etc/dashd/config.yaml && sudo chmod 640 /etc/dashd/config.yaml
   ```
   Merge the `providers:` section from `deploy/config.example.yaml` into it. Put secrets in `/etc/dashd/env` (for example `DASHD_PTERODACTYL_KEY=ptlc_...`, mode 600).
3. **Permissions:** copy `deploy/50-dashd.rules` to `/etc/polkit-1/rules.d/`, with the same unit list as your config.
4. **Service:** copy `deploy/dashd.service` to `/etc/systemd/system/`, then run `sudo systemctl enable --now dashd`.
5. **Tunnel:** add the ingress rule from `deploy/cloudflared.yml` and restart cloudflared. Check that `https://dash.findiepman.dev` shows the sign-in page.

Optional but recommended: put a Cloudflare Access policy for your own email in front of the hostname. Nobody can then reach the sign-in page at all.

## Security notes

- Anyone with access to the Docker socket effectively has root. That's why login needs a password **and** a TOTP code, and why dashd only listens on 127.0.0.1.
- Five wrong attempts from one IP lock that IP out for 15 minutes, and 30 failures in total pause all logins. A TOTP code can't be used twice.
- Sessions are HttpOnly, Secure, SameSite=Strict cookies. They're saved to `sessions.json` as SHA-256 hashes, so a restart doesn't sign you out and a leaked file can't be used as a cookie. State-changing requests and the websocket also check that the Origin matches the host.
- The systemd provider only sees units in its allowlist, and polkit enforces the same list at the OS level. The polkit rule only allows restarting cloudflared, because stopping it would lock you out.
- Every action, including failed ones, is appended to the audit log and shown under Activity.

## Adding a new kind of service

Everything in the UI is a *unit* from a *provider*. To add one (pm2, a remote Wings node, a Minecraft RCON check, ...):

1. Create `agent/internal/providers/<name>/<name>.go` implementing `provider.Provider`:
   `Name`, `Label`, `List`, `Do(id, action)` and `Logs(id, tail)`. Map native states onto `provider.State*` and use `provider.ActionsFor` for the action set.
2. Add a config block to `config.Providers` in `agent/internal/config/config.go`.
3. Register it in `serve()` in `agent/cmd/dashd/main.go`.

The frontend picks up the new group, its LEDs, actions and logs without changes. Discord bots running in Docker, as systemd units or on Pterodactyl already work: add them to the matching config.

## Keyboard

| Key | Action |
| --- | --- |
| `j` / `k` | Move through services |
| `r` then `r` | Restart the selected service |
| `s` then `s` | Stop the selected service |
| `/` | Filter logs |
| `Ctrl K` / `⌘ K` | Command palette (every action for every unit) |
| `Esc` | Cancel |

Restart and stop buttons need a press and hold, so a stray tap on a phone does nothing.
