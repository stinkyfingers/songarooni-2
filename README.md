# Songarooni

A local, offline Raspberry Pi app for a live band: pick a song from a phone, show its slides on the venue display, click along at the right tempo. No internet connection or cloud service required at show time — see `plans/initial-plan.md` for the full design rationale, and `plans/` generally for how the design evolved.

## Running it

```
make build      # native build, for Mac development
make build-pi   # cross-compile for the Pi (aarch64, no CGO/Docker needed)
make test
```

On the Pi:

```
scp dist/songarooni config.yaml songs.csv pi@<host>:~/songarooni/
scp -r media/ pi@<host>:~/songarooni/
./songarooni        # or scripts/run.sh — started by hand, no systemd (see plans/initial-plan.md Phase 6)
```

See `plans/cross-compile-plan.md` for the full build/provisioning walkthrough, including installing `feh` and `alsa-utils` on the Pi.

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
