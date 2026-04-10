SHELL := /bin/sh

BRIDGE_STATE_FILE ?= /tmp/gscale-zebra/bridge_state.json
HTTP_ADDR ?= 127.0.0.1:18000
PRINT_MODE ?= success
PRINT_DELAY ?= 1100ms
SCENARIO ?= batch-flow
SEED ?= 42

.PHONY: help run build test fresh-state

help:
	@echo "Targets:"
	@echo "  make run   - polygon simulatorni ishga tushiradi"
	@echo "  make build - polygon binary build qiladi"
	@echo "  make test  - polygon testlarini yugurtiradi"
	@echo ""
	@echo "Override:"
	@echo "  make run HTTP_ADDR=0.0.0.0:18000"
	@echo "  make run PRINT_MODE=alternate"
	@echo "  make run SCENARIO=stress"

fresh-state:
	@mkdir -p $$(dirname "$(BRIDGE_STATE_FILE)")
	@rm -f "$(BRIDGE_STATE_FILE)" "$(BRIDGE_STATE_FILE).lock"

run: fresh-state
	go run . --http-addr "$(HTTP_ADDR)" --bridge-state-file "$(BRIDGE_STATE_FILE)" --print-mode "$(PRINT_MODE)" --print-delay "$(PRINT_DELAY)" --scenario "$(SCENARIO)" --seed "$(SEED)"

build:
	go build .

test:
	go test ./...
