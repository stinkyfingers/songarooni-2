BINARY := songarooni
DIST   := dist

# Where `make deploy` ships the app. Override on the command line, e.g.
# `make deploy HOST=admin@192.168.1.50`.
HOST       ?= admin@songarooni.local
REMOTE_DIR ?= ~/songarooni/

.PHONY: build build-pi test run clean deploy

build:
	go build -o $(BINARY) ./cmd/songarooni

# Cross-compile for the Raspberry Pi 3 (64-bit Raspberry Pi OS). No CGO in
# this codebase, so this runs directly on the Mac — no Docker or cross
# toolchain needed. See plans/cross-compile-plan.md.
build-pi:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(DIST)/$(BINARY) ./cmd/songarooni

test:
	go test ./...

# Ships the whole per-show app bundle in one shot: the binary, songs.csv,
# scripts/start.sh, and songarooni.desktop (the one-time desktop-icon
# launcher — see README.md's "One-time Pi setup" section for the single
# manual step to activate it; this just keeps it up to date on the Pi
# alongside everything else).
# rsync (not scp) so it's one command for a list of files and so executable
# bits survive the copy. Deliberately NOT included:
#   - media/ — slide images are copied onto the Pi by hand over USB, not
#     synced from here.
#   - One-time Pi system setup (sudoers) — see README.md's "One-time Pi
#     setup" section; that installs into a system path once and isn't part
#     of the recurring app bundle.
# config.prod.yaml (gitignored, machine-local — holds this Pi's real
# hardware paths: the USB slide_dir, its actual audio_device, etc.) is what
# ships, renamed to config.yaml on the Pi, since the binary always looks for
# a file by that literal name (see cmd/songarooni/main.go's -config flag
# default). The plain config.yaml in this repo is a Mac-side dev/test config
# and must never land on the Pi — deploying it overwrites the real settings.
deploy: build-pi
	rsync -av \
		$(DIST)/$(BINARY) songs.json \
		scripts/start.sh scripts/songarooni.desktop \
		$(HOST):$(REMOTE_DIR)
	rsync -av config.prod.yaml $(HOST):$(REMOTE_DIR)config.yaml

run: build
	./$(BINARY)

clean:
	rm -rf $(BINARY) $(DIST)
