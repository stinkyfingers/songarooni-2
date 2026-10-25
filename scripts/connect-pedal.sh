#!/bin/bash
# scripts/connect-pedal.sh — ensure the M-Vave Chocolate is connected before
# the show starts. Safe to run anytime: a near-instant no-op if it's already
# connected, a few retries otherwise. Does NOT re-pair from scratch — if the
# bond itself is gone (e.g. the pedal was paired with another device since),
# this will fail and tell you to re-pair manually; that step needs the pedal
# physically in pairing mode, which can't be scripted.
MAC="XX:XX:XX:XX:XX:XX"   # from `bluetoothctl devices`

if bluetoothctl info "$MAC" 2>/dev/null | grep -q "Connected: yes"; then
    echo "Pedal already connected."
    exit 0
fi

echo "Pedal not connected — retrying..."
for i in 1 2 3 4 5; do
    bluetoothctl connect "$MAC" && exit 0
    sleep 2
done

echo "Could not reconnect automatically."
echo "The pedal may need a power-cycle, or to be put back into pairing mode"
echo "and re-paired fresh: bluetoothctl pair $MAC"
exit 1
