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

Connect the Pi to a router via Ethernet, with the router's own Wi-Fi configured with no WAN/internet uplink. The phone joins the router's Wi-Fi. Simpler Pi-side configuration, at the cost of carrying an extra router.

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
