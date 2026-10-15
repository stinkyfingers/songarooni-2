#!/bin/bash
# Double-click this on the Pi to start Songarooni. Must live in the same
# directory as the compiled `songarooni` binary (copy it there alongside
# config.yaml/songs.csv/media/ when deploying — see README.md).

# Depending on how the file manager is configured, double-clicking an
# executable script can run it directly with no terminal attached at all —
# stdout going nowhere, and nothing to Ctrl+C. If that's the case here
# (stdout isn't a terminal), relaunch this same script inside one.
if [ ! -t 1 ]; then
	script="$(readlink -f "$0")"
	for term in x-terminal-emulator lxterminal xterm; do
		if command -v "$term" >/dev/null 2>&1; then
			exec "$term" -e "$script"
		fi
	done
	# No terminal emulator found on PATH — fall through and run headless;
	# at least songarooni itself still starts.
fi

cd "$(dirname "$0")" || exit 1

# Defensive: force the audio card's ALSA mixer unmuted at full volume every
# time we start. This has silently gone muted before (likely a side effect
# of PipeWire managing the device) with zero error anywhere — aplay plays
# identically whether the channel is muted or not, so the app itself can't
# detect or log this. CARD must match the card number in config.yaml's
# audio_device (e.g. "plughw:1,0" -> CARD=1); adjust if that ever changes.
CARD=1
amixer -c "$CARD" sset PCM 100% unmute >/dev/null 2>&1 || true

# If config.yaml's addr uses a privileged port (<1024, e.g. :80), the binary
# needs cap_net_bind_service to bind it without running as root — see
# README.md. Only ever asks for a password once: skipped entirely once
# already granted, and skipped entirely if the configured port doesn't need
# it at all, so a normal :8080 setup never sees a prompt. Re-granted
# automatically here after every fresh redeploy, since the capability
# doesn't carry over when a new binary file replaces the old one.
port="$(sed -nE 's/^[[:space:]]*addr:[[:space:]]*"?:([0-9]+)"?.*/\1/p' config.yaml 2>/dev/null)"
if [ -n "$port" ] && [ "$port" -lt 1024 ] 2>/dev/null; then
	if command -v setcap >/dev/null 2>&1 && command -v getcap >/dev/null 2>&1; then
		if ! getcap ./songarooni 2>/dev/null | grep -q cap_net_bind_service; then
			echo "Port $port needs elevated privilege to bind — granting it to ./songarooni once (you may be asked for your password):"
			sudo setcap 'cap_net_bind_service=+ep' ./songarooni
		fi
	fi
fi

echo "On the phone, navigate to one of these addresses (port is whatever 'addr' is set to in config.yaml, usually :8080):"
hostname -I
echo

./songarooni

echo
read -n 1 -s -r -p "songarooni exited — press any key to close this window..."
echo
