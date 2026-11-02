# Deploy guide

Follow the steps in order. Every step says **where** to run it:

- 🖥️ **PC**: PowerShell on your Windows PC
- 🐧 **Server**: an SSH session on your Linux server

Copy the commands exactly. The only thing you ever replace is text in `<ANGLE_BRACKETS>`.

**Before you start**, you need:
- the server already running Docker, Wings and playit
- the server on Debian 12, Ubuntu 23.04 or newer (check with `cat /etc/os-release`)
- `findiepman.dev` in your Cloudflare account
- an authenticator app on your phone (Google Authenticator, Aegis, 2FAS, ...)

---

## Step 1. Put the code on GitHub 🖥️ PC

1. Go to https://github.com/new
2. Repository name: `dashd`. Select **Private**. Don't tick any of the "Add a README / .gitignore / license" boxes.
3. Click **Create repository**.

Then in PowerShell:

```powershell
cd C:\Users\fdiep\Desktop\programeren\JavaScript\dashboard
git init -b main
git add .
git update-index --chmod=+x deploy/update.sh
git status
```

**Check the `git status` list before going on.** It must NOT contain `dev/`, `config.yaml`, `node_modules` or anything ending in `.jsonl`. If it does, stop and tell me.

```powershell
git commit -m "dashd"
git remote add origin https://github.com/findiepman/dashd.git
git push -u origin main
```

✅ **You should see** your files on https://github.com/findiepman/dashd

---

## Step 2. Install Go, Node and git 🐧 Server

```bash
sudo apt update
sudo apt install -y git curl

# Go (the apt version is too old)
curl -LO https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz
rm go1.26.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
source ~/.profile

# Node
curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
sudo apt install -y nodejs

# Keep the clock correct after every reboot (login codes depend on it)
sudo timedatectl set-ntp true
```

✅ **Check:**

```bash
go version        # go version go1.26.0 linux/amd64
node -v           # v22 or higher
timedatectl       # "System clock synchronized: yes"
```

---

## Step 3. Download the code to the server 🐧 Server

The repo is private, so GitHub needs a token instead of your password.

**Make the token** (in your browser):
1. Go to https://github.com/settings/personal-access-tokens/new
2. Token name: `server`. Expiration: whatever you like.
3. Repository access: **Only select repositories**, then pick `dashd`.
4. Permissions: **Contents: Read-only**.
5. Click **Generate token** and copy it.

**Clone** (on the server):

```bash
git config --global credential.helper store
git clone https://github.com/findiepman/dashd.git ~/dashd
```

When it asks:
- Username: `findiepman`
- Password: **paste the token** (nothing appears while you paste, that's normal)

Git remembers the token, so updates won't ask again.

✅ **Check:** `ls ~/dashd` shows `agent  deploy  web  README.md ...`

---

## Step 4. Build and install dashd 🐧 Server

```bash
cd ~/dashd/web
npm ci
npm run build

cd ~/dashd/agent
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/dashd ./cmd/dashd

sudo install -m 755 ~/dashd/bin/dashd /usr/local/bin/dashd
```

✅ **Check:** `dashd help` prints the usage text.

---

## Step 5. Create your login 🐧 Server

```bash
sudo useradd --system --home /var/lib/dashd --shell /usr/sbin/nologin dashd
sudo mkdir -p /etc/dashd
sudo dashd init -o /etc/dashd/config.yaml
```

It asks for:
1. **Username**: what you'll log in with
2. **Password**: at least 12 characters (nothing shows while you type)

Then it prints a **QR code. Scan it with your authenticator app now.** If the QR doesn't fit your terminal, type the "secret" it prints into the app by hand instead.

✅ **You should see** `Wrote /etc/dashd/config.yaml`, and a new `dashd` entry in your authenticator app.

---

## Step 6. Choose what the dashboard shows 🐧 Server

### 6a. Find your service names

```bash
systemctl list-unit-files --type=service | grep -Ei 'wings|playit|ptero|cloudflared'
```

Write down the names without `.service`, for example `wings`, `playit`, `pteroq`, `cloudflared`.

### 6b. Put them in the config

```bash
sudo nano /etc/dashd/config.yaml
```

Find this line and make it match the names from 6a:

```yaml
    units: [wings, playit, pteroq, cloudflared]
```

Save with `Ctrl+O`, `Enter`, then exit with `Ctrl+X`.

### 6c. Connect Pterodactyl (skip if you don't want game servers in the dashboard)

1. Open your Pterodactyl panel → click your account (top right) → **API Credentials**.
2. Description: `dashd`. Leave "Allowed IPs" empty. Click **Create**.
3. Copy the key. It starts with `ptlc_`.

Save the key (replace the placeholder with your key):

```bash
echo 'DASHD_PTERODACTYL_KEY=<PASTE_KEY_HERE>' | sudo tee /etc/dashd/env > /dev/null
sudo chmod 600 /etc/dashd/env
```

Open the config again with `sudo nano /etc/dashd/config.yaml` and, at the bottom, remove the `# ` from these three lines. Also change the URL if your panel lives somewhere else:

```yaml
  pterodactyl:
    url: https://panel.findiepman.dev
    api_key: env:DASHD_PTERODACTYL_KEY
```

The indentation matters: `pterodactyl:` has 2 spaces in front of it, and the two lines below have 4.

### 6d. Lock down the config file

```bash
sudo chown root:dashd /etc/dashd/config.yaml
sudo chmod 640 /etc/dashd/config.yaml
```

---

## Step 7. Allow dashd to restart those services 🐧 Server

```bash
nano ~/dashd/deploy/50-dashd.rules
```

The names in the `allowed` list must match what you put in step 6b, with `.service` on the end. If they already match, don't change anything. Then:

```bash
sudo cp ~/dashd/deploy/50-dashd.rules /etc/polkit-1/rules.d/50-dashd.rules
sudo systemctl restart polkit
```

---

## Step 8. Start dashd and make it start on every boot 🐧 Server

```bash
sudo cp ~/dashd/deploy/dashd.service /etc/systemd/system/dashd.service
sudo systemctl daemon-reload
sudo systemctl enable --now dashd
```

✅ **Check:**

```bash
systemctl status dashd --no-pager
```

It should say **`active (running)`** in green. Then:

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7070/
```

It should print **`200`**.

If it isn't running, `journalctl -u dashd -n 30 --no-pager` shows why. The error message tells you which line of the config is wrong.

---

## Step 8b. Show and control your Discord bots (pm2) 🐧 Server

Skip this if you don't run bots with pm2. Everything pm2 runs shows up as a **Discord bots** group under Services and on its own **Discord bots** page, with the Discord name each bot logged in as, hold-to-confirm Start, Restart and Stop buttons, and their logs in the log pane (like `pm2 logs`). Anything a bot prints shows there, so a bot that logs its token at startup shows it to whoever is signed in to the dashboard. dashd can't look inside your home folder, so a small helper runs as your user: it writes the list to `/run/pm2-status/pm2.json` and takes the button presses and log requests on `/run/pm2-status/control.sock`.

```bash
nano ~/dashd/deploy/dashd-pm2.service
```

If pm2 runs under a user other than `fin`, change `fin` in all five places. The `-accounts fin-and-co=...` part shows the bot names from Fin & Co's `bots.json` (only the names, never the tokens); remove it if you don't have that. Then:

```bash
sudo cp ~/dashd/deploy/dashd-pm2.service /etc/systemd/system/dashd-pm2.service
sudo systemctl daemon-reload
sudo systemctl enable dashd-pm2
sudo systemctl restart dashd-pm2
```

✅ **Check:**

```bash
cat /run/pm2-status/pm2.json          # your bots with "status":"online"
getfacl -p /run/pm2-status/control.sock | grep dashd   # user:dashd:rw-
```

Now turn the page on. Open `sudo nano /etc/dashd/config.yaml` and add this under `providers:` (2 spaces before `pm2:`), if it isn't there yet:

```yaml
  pm2: {}
```

```bash
sudo systemctl restart dashd
```

✅ **Check:** reload the dashboard (Ctrl+Shift+R). The top bar has a **Discord bots** tab, and holding **Restart** on a bot restarts it (it shows up under Activity).

---

## Step 9. Put it on dash.findiepman.dev (Cloudflare website)

**If you don't have a Cloudflare Tunnel yet:**
1. Go to https://one.dash.cloudflare.com → **Networks** → **Tunnels** → **Create a tunnel**.
2. Choose **Cloudflared**. Name it `server`. Click **Save tunnel**.
3. Under "Choose your environment", pick **Debian** (it also works for Ubuntu). Copy the **second** command, the one starting with `sudo cloudflared service install`, and run it on the server. The first command installs cloudflared; run that too if cloudflared isn't installed yet.
4. Back in the browser, click **Next**.

**If you already have a tunnel,** open it in **Networks** → **Tunnels** → click it → **Edit** → **Public Hostname** tab.

**Add the hostname:**
- Subdomain: `dash`
- Domain: `findiepman.dev`
- Service type: `HTTP`
- URL: `127.0.0.1:7070`

Click **Save**.

**Strongly recommended: only let your email through.**
1. **Access** → **Applications** → **Add an application** → **Self-hosted**.
2. Application domain: `dash.findiepman.dev`.
3. Add a policy: Action **Allow**, Include **Emails**, then your email.
4. Save.

✅ **Check:** open https://dash.findiepman.dev on your phone with WiFi **off**. You should see the sign-in page (after the Cloudflare email code, if you added that). Sign in with your username, password and the code from your authenticator app.

---

## Step 10. Make everything come back after restarts 🐧 Server

**Turn on start-at-boot for everything** (skip names you don't have):

```bash
sudo systemctl enable docker wings playit cloudflared dashd
```

**Docker containers need a restart policy**, or they stay off after a reboot. List them:

```bash
docker ps -a --format '{{.Names}}' | xargs -I{} docker inspect -f '{{.Name}} {{.HostConfig.RestartPolicy.Name}}' {}
```

For every line that ends in `no`, run:

```bash
docker update --restart unless-stopped <CONTAINER_NAME>
```

If you use docker-compose, also add `restart: unless-stopped` under each service in the compose file, otherwise it's lost the next time you run `docker compose up`.

Game servers don't need anything: Wings starts the servers that were running before the reboot.

---

## Step 11. Test a real reboot 🐧 Server

```bash
sudo reboot
```

Wait 2 minutes, then open https://dash.findiepman.dev on your phone.

✅ **Everything worked if:**
- you're **still signed in** (no login page)
- the top right says **Live**
- every service you expect has a green dot

If something's off, SSH in and run:

```bash
systemctl --failed
journalctl -u dashd -b --no-pager | tail -30
```

---

## Updating after you change code

🖥️ **PC**, push your changes:

```powershell
git add .
git commit -m "<what you changed>"
git push
```

🐧 **Server**, pull, rebuild and restart in one go:

```bash
cd ~/dashd && sudo ./deploy/update.sh
```

You stay signed in during updates.

---

## Something's wrong

| What you see | Fix |
| --- | --- |
| Login code is always "wrong" | Server clock is off. Run `sudo timedatectl set-ntp true`, wait a minute, then check that `timedatectl` says synchronized. |
| Signed out after every reboot | Run `sudo ls -l /var/lib/dashd/`. If `sessions.json` isn't there, check `journalctl -u dashd` for "could not save sessions". |
| Docker section says "socket not reachable" | Docker isn't running: `sudo systemctl start docker`. |
| Discord bots page says "no pm2 snapshot" or "snapshot is … old" | The helper isn't running: `systemctl status dashd-pm2 --no-pager`, then `journalctl -u dashd-pm2 -n 20 --no-pager`. |
| Discord bots page says "pm2 isn't running" | Start pm2: `sudo systemctl start pm2-fin`. |
| Bot buttons say "the dashd-pm2 helper isn't taking actions" or "isn't allowed to use the helper's control socket" | Run `cd ~/dashd && sudo ./deploy/update.sh`, then check `getfacl -p /run/pm2-status/control.sock` shows `user:dashd:rw-`. If not, `journalctl -u dashd-pm2 -n 20 --no-pager` says why. |
| Bot buttons say "the helper is restarting" | Wait a few seconds and try again. |
| Restarting a service says "Interactive authentication required" | The name in `/etc/polkit-1/rules.d/50-dashd.rules` doesn't match step 6b. Fix it, then run `sudo systemctl restart polkit`. |
| Pterodactyl section says the key was rejected | You made an **application** key (`ptla_`). Make a **client** key (`ptlc_`) under your account's API Credentials. |
| Cloudflare error page (502) | dashd isn't running (step 8 check), or the tunnel URL isn't exactly `127.0.0.1:7070`. |
| `dashd` config error on start | `journalctl -u dashd -n 20` names the bad line. Usually wrong indentation in step 6c. |
