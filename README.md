# Songarooni

A local, offline Raspberry Pi app for a live band: pick a song from a phone, show its slides on the venue display, click along at the right tempo. No internet connection or cloud service required at show time — see `plans/initial-plan.md` for the full design rationale, and `plans/` generally for how the design evolved.

## Running it

```
make build      # native build, for Mac development
make build-pi   # cross-compile for the Pi (aarch64, no CGO/Docker needed)
make test
```

Deploy the whole app bundle (binary, songs, every `scripts/` file `start.sh` depends on, and `config.prod.yaml` — renamed to `config.yaml` on the Pi) in one shot:

```
make deploy HOST=admin@<host>   # defaults to admin@songarooni.local if omitted
```

`config.prod.yaml` is gitignored and machine-local — it holds this Pi's actual hardware settings (its USB slide directory, real audio device, etc.), separate from the plain `config.yaml` in this repo, which is a Mac-side dev/test config and must never be deployed. Slide images themselves aren't part of this — they're copied onto the Pi by hand over USB.

Then on the Pi itself:

```
./start.sh        # or scripts/run.sh — started by hand, no systemd (see plans/initial-plan.md Phase 6)
```

See `plans/cross-compile-plan.md` for the full build/provisioning walkthrough, including installing `alsa-utils` on the Pi (and a Chromium-family browser for the venue display — see "Venue display" below).

## Setting the Pi's hostname (`songarooni.local`)

This is what actually makes `songarooni.local` resolve on the network — relevant any time you set up a new Pi, or need to repeat/change it.

1. **Set the system hostname**, if it isn't already what you want:
   ```bash
   hostname   # check the current one
   ```
   If it's not `songarooni` (or whatever name you want): `sudo raspi-config` → System Options → Hostname, then reboot for it to fully take effect.

2. **Make sure Avahi (mDNS) is installed and running** — this is what actually answers `<hostname>.local` lookups on the network; it's usually already present on Raspberry Pi OS Desktop images, but if not:
   ```bash
   sudo apt install avahi-daemon
   sudo systemctl enable --now avahi-daemon
   ```

That's it — no other configuration needed. The hostname becomes `<hostname>.local` automatically; there's no separate "domain" to set. See "Network setup" below for the fallback if mDNS doesn't resolve reliably on a given venue's network (some router firmwares block it).

## Network setup

The phone and the Pi need to share a private local network, with no reliance on internet access. Two options:

### Option A — Dedicated router

Connect the Pi to a router via Ethernet; the phone joins the router's Wi-Fi. Simpler Pi-side configuration, at the cost of carrying an extra router. The router's WAN side can either be left disconnected entirely, or — if it's a travel router — bridged to whatever Wi-Fi happens to be available wherever you're playing, without affecting the band network at all.

**Using a travel router** (e.g. GL.iNet and similar): these are built to uplink to an existing Wi-Fi network while serving their own separate SSID downstream — exactly the "connect to a router while also serving" setup. Exact menu names vary by brand, but the shape is the same:

1. Connect to the travel router's own admin page (check the device for its default address/SSID — commonly `192.168.8.1` for GL.iNet).
2. Find the uplink mode — labeled "Repeater," "WISP," "Internet," or "Client" mode depending on the brand — and have it scan for and join whatever upstream Wi-Fi you want (venue Wi-Fi, a phone hotspot, home Wi-Fi for testing). This is independent of the router's own downstream network.
3. Set the router's own downstream Wi-Fi SSID/password (e.g. `BandControl`) under its regular Wi-Fi/AP settings — separate from the upstream connection you just configured.
4. Connect the Pi to one of the router's LAN (not WAN) ports via Ethernet.
5. The phone joins `BandControl` as usual.

The upstream link only ever provides internet access to devices behind the router — it has no bearing on whether the Pi and phone can reach each other. If the upstream Wi-Fi drops entirely (or you never configure one), the router keeps serving its own `BandControl` network to the Pi and phone exactly the same either way, so there's no real downside to leaving the uplink configured even if you don't always need it.

**Using a name instead of the router-assigned IP** (e.g. `http://songarooni.local:8080` instead of `http://192.168.x.x:8080`): Raspberry Pi OS ships mDNS (Avahi) for exactly this — no router configuration needed, and it's what makes `raspberrypi.local` work out of the box on a stock image. See "Setting the Pi's hostname" above if it isn't already configured. Once it is, just navigate to `http://<hostname>.local:8080` from the phone (e.g. `http://songarooni.local:8080`) — iOS has first-class Bonjour/mDNS support built in, so this is reliably the easiest option given the phone side is always an iPhone here.

Two things worth doing alongside this, regardless of whether mDNS works reliably on a given venue's network:
- **Give the Pi a static DHCP reservation** on the router (bind its MAC address to a fixed IP in the router's admin page) — so even typing the numeric IP as a fallback, it's always the same one, rather than potentially changing after a reboot.
- **"Add to Home Screen" on the phone** once the URL (name or IP) is confirmed working — turns it into a one-tap icon, which sidesteps the "cumbersome to type" problem entirely regardless of which naming approach you use.

If a venue's network ever blocks mDNS (some router firmwares do, particularly on guest networks), the static-IP-plus-bookmark combination above is the dependable fallback.

### Option B — Pi as its own access point (recommended — no extra hardware)

The Pi's built-in Wi-Fi creates the network directly; no router required.

1. Check what's managing Wi-Fi on the Pi first, rather than installing a conflicting network service:
   ```bash
   cat /etc/os-release
   nmcli general status
   ```
2. If NetworkManager is managing Wi-Fi (the default on current Raspberry Pi OS), create the hotspot:
   ```bash
   sudo nmcli device wifi hotspot \
     ifname wlan0 \
     ssid BandControl \
     password 'choose-a-strong-password'
   ```
   NetworkManager sets up a private subnet and DHCP automatically. Confirm the assigned address:
   ```bash
   ip -4 addr show wlan0
   ```
3. On the phone: join `BandControl` in Settings → Wi-Fi, then open Safari to `http://<pi-ip>:8080` — the port is whatever `addr` is set to in `config.yaml` (`:8080` by default).
4. Verify song selection and state updates work with the Pi fully disconnected from the internet.
5. Make the hotspot persist across reboots — `nmcli` hotspots are saved as a connection profile by default; confirm with `nmcli connection show` and `nmcli connection up <profile-name>` if it doesn't come up automatically.
6. Test reconnecting after the phone sleeps or walks out of Wi-Fi range.

If NetworkManager isn't managing Wi-Fi on your Raspberry Pi OS version, use the access-point setup appropriate for it instead — typically `hostapd` plus a DHCP server (`dnsmasq`). Don't run two competing hotspot configurations at once.

A couple of things worth knowing going in:

- Use the Pi's numeric IP address rather than `songarooni.local` — mDNS resolution isn't reliable across every network setup, and the app doesn't currently register an mDNS name anyway.
- iOS will warn that `BandControl` has no internet connection. That's expected and harmless — the phone can still reach the Pi's site; just confirm Safari actually stays connected to it during a real set rather than switching to cellular.


## One-time Pi setup: M-Vave Chocolate foot pedal

Everything below installs into system paths outside the app directory and only needs doing once per Pi (not on every `make deploy`). See `plans/pedal-connection-plan.md` for the full rationale.

1. **Pair and trust the pedal** with the Pi over Bluetooth first (`bluetoothctl` — scan, pair, trust), if you haven't already.

2. **Make the Bluetooth adapter survive a reboot** — without this, a trusted pedal still won't auto-reconnect after a power-cycle because the adapter itself never powers back on:
   ```bash
   sudo tee -a /etc/bluetooth/main.conf > /dev/null << 'EOF'
   [Policy]
   AutoEnable=true
   EOF
   sudo systemctl restart bluetooth
   ```

3. **Fill in the pedal's real MAC address** in `scripts/connect-pedal.sh` (`bluetoothctl devices` to find it) before the next `make deploy`.

4. **Put the Pi user in the `input` group**, so `songarooni` itself can read `/dev/input/eventN` directly without running as root:
   ```bash
   sudo usermod -aG input "$(whoami)"
   ```
   Log out and back in (or reboot) for the group change to take effect.

5. **Confirm `config.prod.yaml`'s `pedal_device_name` actually matches the pedal.** It defaults to `"Chocolate"` — a best guess, not a confirmed value. Check the real name once the pedal is connected:
   ```bash
   cat /proc/bus/input/devices
   ```
   Look for the `N: Name=` line that isn't your mouse/keyboard, and set `pedal_device_name` in `config.prod.yaml` to match (a substring is fine — the match is case-insensitive `contains`, not exact).

That's it — pedal keystrokes aren't handled by `triggerhappy` or read from a terminal (both tried first; see `plans/pedal-connection-plan.md` for why each one broke). Instead, `songarooni` itself reads the pedal's raw Linux evdev device directly (see `internal/pedal`) and re-finds it automatically if reading ever fails (e.g. after a Bluetooth reconnect recreates the device node mid-show) — unlike `triggerhappy`, which only ever opens the device files it saw at its own startup and never recovers from that on its own. From here on, every show start (`start.sh`) just reconnects the pedal if needed, so the pedal drives the app directly.

## One-time Pi setup: easy single-click start

1. **Stop the port-80 password prompt from repeating on every redeploy.** `setcap`'s grant is tied to the specific binary file, and every `make deploy` replaces it — wiping the grant each time. A narrow passwordless-sudo rule for just this one command fixes it permanently:
   ```bash
   which setcap   # confirm the real path — usually /usr/sbin/setcap on Raspberry Pi OS
   echo "$(whoami) ALL=(ALL) NOPASSWD: /usr/sbin/setcap cap_net_bind_service=+ep *" | sudo tee /etc/sudoers.d/songarooni-setcap
   sudo chmod 440 /etc/sudoers.d/songarooni-setcap
   ```
   (Only needed at all if `config.prod.yaml`'s `addr` uses a port below 1024, e.g. `:80`.)

2. **Add a desktop icon for single-click start.** `make deploy` ships `songarooni.desktop` into the app directory alongside `start.sh`; activate it once:
   ```bash
   cp ~/songarooni/songarooni.desktop ~/Desktop/
   chmod +x ~/Desktop/songarooni.desktop
   ```
   Then right-click the new desktop icon and choose "Trust this launcher" (or similar — exact wording varies by Raspberry Pi OS version) the first time; after that, double-clicking just runs it directly. `Terminal=false` in the launcher is deliberate — `start.sh` already detects a non-terminal stdout and relaunches itself inside one on its own, so the launcher doesn't need to wrap it in one too.

## One-time Pi setup: GPU memory for smooth video

The default GPU memory split (as little as 76M on some Raspberry Pi OS images) isn't enough for a video-heavy slideshow — Chromium will report `memory_throttled` on `chrome://gpu` and fall back to slower rendering paths, showing up as jerky video/transitions and occasional blank-white flashes.

1. **Check the current split**: `vcgencmd get_mem gpu`.
2. **Increase it** — add (or edit) this line in whichever of `/boot/firmware/config.txt` (current Raspberry Pi OS) or `/boot/config.txt` (older versions) your Pi actually boots from — check both with `grep -n gpu /boot/firmware/config.txt /boot/config.txt 2>/dev/null` first:
   ```ini
   gpu_mem=128
   ```
   Check total RAM first (`free -h`) before picking a number — 128 is a safe default on a 1GB Pi; 256 is fine on 2GB+. The exact directive name matters (`gpu_mem`, not `gpu`) and unknown keys are silently ignored, so a typo here produces no error at all, just no change.
3. **Reboot** — this is a boot-time allocation, not something that can change live. `vcgencmd get_mem gpu` only reflects what was actually set at the last boot.
4. **Confirm it took**: `vcgencmd get_mem gpu` should now show the new value, and `chrome://gpu` (reachable by temporarily setting `no_kiosk: true` in `config.yaml` for a normal, non-fullscreen window) should no longer show `memory_throttled`.

**If video is still jerky after that**, two further things worth checking:
- **Thermal throttling**: `vcgencmd measure_temp` and `vcgencmd get_throttled`. Decoding the latter: bit `0x4` means *currently* throttled, `0x2` means frequency *currently* capped — both low/0 is good; `0x20000`/`0x40000` etc. mean capping/throttling happened *at some point since boot*, historical only. Sustained video decode generates real heat; if temps are consistently near 80°C, a heatsink/fan is worth adding.
- **Video encoding**: an oversized or high-profile H.264 encode can silently fall back to slow software decoding even with hardware decode available. Re-encoding at a resolution matching the actual display, Baseline or Main profile, moderate bitrate, is usually the single biggest win for a Pi 3/4-class board.

## Venue display: kiosk browser

`songarooni` serves a kiosk-mode slideshow page on its own port (`browser_slideshow_addr`, default `:8888`) and launches a fullscreen kiosk browser window pointed at it, auto-detecting a Chromium-family browser on `PATH` (`chromium-browser`, `chromium`, `google-chrome`, in that order — covers both older and current Raspberry Pi OS versions, which renamed the binary across releases). Set `browser_command` in `config.yaml` to override the auto-detected command entirely (a full command + args list; `{{url}}` is replaced with the slideshow page's URL) if you need something else. See `internal/slideshow` and `plans/browser-slideshow-plan.md` (which also covers an earlier `feh`-based display, since removed in favor of this).

No extra one-time Pi setup is strictly required beyond having a Chromium-family browser installed (Raspberry Pi OS ships one by default) — but if `make deploy` ever logs `no kiosk-capable browser found on PATH`, install one (`sudo apt install chromium` on current Raspberry Pi OS) or set `browser_command` explicitly.