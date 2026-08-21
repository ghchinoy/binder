# binder — Phase 1 (corpus → OKF v0.2 bundle)
#
# Local gates (Go toolchain; deps fetched from the module proxy):  make check
# Full exit gate incl. external differential validation:          make gate

GO      ?= go
BIN     := bin/binder
OKF_VER := v0.3.0
OKF_PKG := github.com/okfcli/okf/cmd/okf@$(OKF_VER)

.PHONY: all build test vet fmt-check check gate interop okf-install golden-update docs clean

all: build

build:
	$(GO) build -o $(BIN) .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

# gofmt over binder's own sources.
fmt-check:
	@unformatted=$$(gofmt -l cmd internal . 2>/dev/null || true); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	else echo "gofmt: clean"; fi

# Verification gate: everything that needs only the Go toolchain (deps pinned
# via go.mod/go.sum, fetched from the module proxy).
check: fmt-check vet test

# Regenerate the CLI command reference (docs/commands/) from binder's own Cobra
# command tree. Deterministic and idempotent; run after adding/changing a
# command or flag. The internal/gendocs drift test (part of `make check`) fails
# if the committed reference falls out of sync with the command tree.
docs:
	$(GO) run ./cmd/gendocs

# Regenerate the byte-stable golden fixture after an intentional change.
golden-update:
	$(GO) test ./internal/convert -run TestConvertGolden -update

# Install the external, vendor-neutral OKF validator used by the interop gate.
okf-install:
	$(GO) install $(OKF_PKG)

# Differential-validation + viewer-edge gate against okfcli/okf.
interop: build
	bash scripts/interop.sh

# Full Phase-1 exit gate: local checks + external interop.
gate: check interop

clean:
	rm -rf bin
