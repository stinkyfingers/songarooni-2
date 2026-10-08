#!/bin/sh
# Build and run songarooni. Start this by hand before each rehearsal/show
# (see initial-plan.md Phase 6 — no systemd for now).
set -e
cd "$(dirname "$0")/.."
go build -o songarooni ./cmd/songarooni
exec ./songarooni "$@"
