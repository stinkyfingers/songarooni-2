#!/bin/bash
# scripts/unbind-feh-keys.sh — stop feh's own default slideshow navigation
# from reacting to the keys the foot pedal uses (Space, Return, Page Up,
# Page Down). songarooni never relies on feh's own key-driven next/prev
# navigation — it restarts feh pointed at a new image/directory itself (see
# internal/slideshow) — so fully unbinding these is safe.
#
# Per `man feh` (KEYS CONFIG SYNTAX): specifying an action with no key
# unbinds it. These are feh's REAL default actions for each key (man feh,
# KEYS section) — not menu_parent/next_random/prev_random, which either bind
# to a different key (menu_parent is Left, not Return) or aren't real feh
# actions at all (next_random/prev_random don't exist):
#   Space      -> next_img
#   Return     -> action_0
#   Page Up    -> jump_fwd
#   Page Down  -> jump_back

KEYS_FILE=~/.config/feh/keys
mkdir -p "$(dirname "$KEYS_FILE")"

# Guarded by a marker so re-running this on every start.sh launch doesn't
# keep appending duplicate blocks to the keys file forever.
MARKER="# songarooni: pedal-key unbinds"
if ! grep -qF "$MARKER" "$KEYS_FILE" 2>/dev/null; then
	cat << EOF >> "$KEYS_FILE"

$MARKER
next_img
action_0
jump_fwd
jump_back
EOF
fi
