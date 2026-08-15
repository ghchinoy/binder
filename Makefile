# binder — Phase 1 (corpus → OKF v0.2 bundle)
#
# Offline gates (no network/tools beyond the Go toolchain):  make check
# Full exit gate incl. external differential validation:      make gate

GO      ?= go
BIN     := bin/binder
OKF_VER := v0.3.0
OKF_PKG := github.com/okfcli/okf/cmd/okf@$(OKF_VER)

.PHONY: all build test vet fmt-check check gate interop okf-install golden-update clean

all: build

build:
	$(GO) build -o $(BIN) .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

# gofmt over binder's own sources (never vendor/).
fmt-check:
	@unformatted=$$(gofmt -l cmd internal . 2>/dev/null | grep -v '^vendor/' || true); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	else echo "gofmt: clean"; fi

# Offline verification gate: everything that needs only the Go toolchain.
check: fmt-check vet test

# Regenerate the byte-stable golden fixture after an intentional change.
golden-update:
	$(GO) test ./internal/convert -run TestConvertGolden -update

# Install the external, vendor-neutral OKF validator used by the interop gate.
okf-install:
	$(GO) install $(OKF_PKG)

# Differential-validation + viewer-edge gate against okfcli/okf.
interop: build
	bash scripts/interop.sh

# Full Phase-1 exit gate: offline checks + external interop.
gate: check interop

clean:
	rm -rf bin
