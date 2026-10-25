#!/bin/bash
# scripts/connect-pedal.sh — ensure the M-Vave Chocolate is connected before
# the show starts. Safe to run anytime: a near-instant no-op if it's already
# connected, a few retries otherwise. Does NOT re-pair from scratch — if the
# bond itself is gone (e.g. the pedal was paired with another device since),
# this will fail and tell you to re-pair manually; that step needs the pedal
# physically in pairing mode, which can't be scripted.
MAC="25:FA:C2:54:4A:62"   # from `bluetoothctl devices`

# A BLE reconnect (including one that happened on its own, before this script
# ever ran) tears down the old /dev/input/eventN node and creates a new one.
# triggerhappy's systemd unit globs device files once at startup and never
# re-checks, so it's left reading a dead fd forever until restarted — this
# restart is what makes it pick up whatever device node is current right now.
restart_triggerhappy() {
    sudo systemctl restart triggerhappy 2>/dev/null || true
}

if bluetoothctl info "$MAC" 2>/dev/null | grep -q "Connected: yes"; then
    echo "Pedal already connected."
    restart_triggerhappy
    exit 0
fi

echo "Pedal not connected — retrying..."
for i in 1 2 3 4 5; do
    if bluetoothctl connect "$MAC"; then
        restart_triggerhappy
        exit 0
    fi
    sleep 2
done

echo "Could not reconnect automatically."
echo "The pedal may need a power-cycle, or to be put back into pairing mode"
echo "and re-paired fresh: bluetoothctl pair $MAC"
exit 1
