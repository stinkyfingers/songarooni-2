BINARY := songarooni
DIST   := dist

.PHONY: build build-pi test run clean

build:
	go build -o $(BINARY) ./cmd/songarooni

# Cross-compile for the Raspberry Pi 3 (64-bit Raspberry Pi OS). No CGO in
# this codebase, so this runs directly on the Mac — no Docker or cross
# toolchain needed. See plans/cross-compile-plan.md.
build-pi:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(DIST)/$(BINARY) ./cmd/songarooni

test:
	go test ./...

run: build
	./$(BINARY)

clean:
	rm -rf $(BINARY) $(DIST)
