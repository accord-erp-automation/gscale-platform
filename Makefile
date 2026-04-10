SHELL := /bin/sh

SCALE_DEVICE ?= /dev/ttyUSB0
ZEBRA_DEVICE ?= /dev/usb/lp0
BRIDGE_STATE_FILE ?= /tmp/gscale-zebra/bridge_state.json
POLYGON_HTTP_ADDR ?= 127.0.0.1:18000
POLYGON_SCENARIO ?= batch-flow
POLYGON_SEED ?= 42
APP_USER ?= $(shell id -un)
APP_GROUP ?= $(shell id -gn)
MOBILE_API_ADDR ?= 0.0.0.0:8081
MOBILE_API_SERVER_NAME ?= $(shell hostname)
CURL ?= curl
POLYGON_DEV_BIN ?= /tmp/gscale-zebra/polygon-dev
MOBILEAPI_DEV_BIN ?= /tmp/gscale-zebra/mobileapi-dev
SCALE_DEV_LAUNCH_LOG ?= /tmp/gscale-zebra/scale-dev.log
MOBILE_APP_DIR ?= /home/wikki/storage/local.git/erpnext_stock_telegram/mobile_app
MOBILE_API_PORT ?= $(shell printf '%s\n' "$(MOBILE_API_ADDR)" | awk -F: '{print $$NF}')
MOBILE_API_BASE_URL ?= http://127.0.0.1:$(MOBILE_API_PORT)
MOBILE_RUN_TARGET ?= run-auto
MOBILE_FLUTTER_RUN_ARGS ?= --dart-define=API_BASE_URL=$(MOBILE_API_BASE_URL)

.PHONY: help check-env build build-bot build-scale build-zebra build-polygon build-mobileapi run run-scale run-bot run-polygon run-test run-dev run-mobile run-mobile-android run-mobile-linux run-mobile-web stop-dev-services stop-bot-services fresh-bridge-state run-mobileapi test test-polygon test-mobileapi clean release release-all autostart-install autostart-status autostart-restart autostart-stop

help:
	@echo "Targets:"
	@echo "  make run        - scale TUI ni ishga tushiradi (bot auto-start bilan)"
	@echo "  make run-scale  - faqat scale TUI (bot auto-startsiz)"
	@echo "  make run-bot    - faqat telegram bot"
	@echo "  make run-polygon - real qurilmasiz polygon simulator"
	@echo "  make run-test   - polygon + scale TUI (qurilmasiz core test)"
	@echo "  make run-dev    - backend/core dev stack: polygon + mobileapi + scale"
	@echo "  make run-mobile - Flutter mobile client (default: auto device)"
	@echo "  make run-mobile-android - Flutter mobile client Android uchun"
	@echo "  make run-mobile-linux   - Flutter mobile client Linux desktop uchun"
	@echo "  make run-mobile-web     - Flutter mobile client Web uchun"
	@echo "  make stop-dev-services - run-dev qoldirgan servislarni to'xtatadi"
	@echo "  make stop-bot-services - ishlayotgan bot processlarini to'xtatadi"
	@echo "  make run-mobileapi - mobile API backend"
	@echo "  make build      - bot + scale + zebra binary build (./bin)"
	@echo "  make build-polygon - polygon binary build (./bin)"
	@echo "  make build-mobileapi - mobile API binary build (./bin)"
	@echo "  make test       - barcha modullarda test"
	@echo "  make test-polygon - polygon modul testlari"
	@echo "  make test-mobileapi - mobile API testlari"
	@echo "  make autostart-install - systemd service'larni o'rnatadi va start qiladi"
	@echo "  make autostart-status  - service holatini ko'rsatadi"
	@echo "  make autostart-restart - service'larni restart qiladi"
	@echo "  make autostart-stop    - service'larni to'xtatadi"
	@echo "  make release    - linux/amd64 tar release"
	@echo "  make release-all - linux/amd64 + linux/arm64 tar release"
	@echo "  make clean      - local build papkalarini tozalash"
	@echo ""
	@echo "Override:"
	@echo "  make run SCALE_DEVICE=/dev/ttyUSB1 ZEBRA_DEVICE=/dev/usb/lp0"
	@echo "  make run-polygon SCENARIO=stress"
	@echo "  make run-mobile MOBILE_API_BASE_URL=http://127.0.0.1:8081"
	@echo "  make run-mobile-android MOBILE_APP_DIR=/path/to/mobile_app"

check-env:
	@test -f bot/.env || (echo "xato: bot/.env topilmadi (bot/.env.example dan nusxa oling)"; exit 1)

fresh-bridge-state:
	@mkdir -p /tmp/gscale-zebra
	@rm -f "$(BRIDGE_STATE_FILE)" "$(BRIDGE_STATE_FILE).lock"

build: build-bot build-scale build-zebra

build-bot:
	@mkdir -p bin
	go build -o ./bin/bot ./bot/cmd/bot

build-scale:
	@mkdir -p bin
	go build -o ./bin/scale ./scale

build-zebra:
	@mkdir -p bin
	go build -o ./bin/zebra ./zebra

build-polygon:
	@mkdir -p bin
	go build -o ./bin/polygon ./polygon

build-mobileapi:
	@mkdir -p bin
	go build -o ./bin/mobileapi ./cmd/mobileapi

run: check-env fresh-bridge-state stop-dev-services stop-bot-services
	cd scale && go run . --no-bridge --device "$(SCALE_DEVICE)" --zebra-device "$(ZEBRA_DEVICE)" --bridge-state-file "$(BRIDGE_STATE_FILE)"

run-scale: fresh-bridge-state stop-dev-services stop-bot-services
	cd scale && go run . --no-bot --no-bridge --device "$(SCALE_DEVICE)" --zebra-device "$(ZEBRA_DEVICE)" --bridge-state-file "$(BRIDGE_STATE_FILE)"

run-bot: check-env fresh-bridge-state stop-bot-services
	cd bot && go run ./cmd/bot

run-polygon: fresh-bridge-state stop-dev-services stop-bot-services
	$(MAKE) -C polygon run

run-test: fresh-bridge-state stop-dev-services stop-bot-services
	@POLY_PID=""; \
	trap 'if [ -n "$$POLY_PID" ]; then kill $$POLY_PID 2>/dev/null || true; fi' EXIT INT TERM; \
	(cd polygon && go run . --http-addr "$(POLYGON_HTTP_ADDR)" --bridge-state-file "$(BRIDGE_STATE_FILE)" --scenario "$(POLYGON_SCENARIO)" --seed "$(POLYGON_SEED)" >/tmp/gscale-zebra/polygon.log 2>&1) & \
	POLY_PID=$$!; \
	sleep 1; \
	cd scale && go run . --no-bot --no-zebra --bridge-url "http://$(POLYGON_HTTP_ADDR)/api/v1/scale" --bridge-state-file "$(BRIDGE_STATE_FILE)"

run-dev: fresh-bridge-state
	@$(MAKE) stop-dev-services >/dev/null 2>&1 || true
	@$(MAKE) stop-bot-services >/dev/null 2>&1 || true
	@go build -o "$(POLYGON_DEV_BIN)" ./polygon
	@go build -o "$(MOBILEAPI_DEV_BIN)" ./cmd/mobileapi
	@POLY_PID=""; \
	MOBILEAPI_PID=""; \
	SCALE_PID=""; \
	TAIL_PIDS=""; \
	start_tail() { \
		FILE="$$1"; \
		LABEL="$$2"; \
		touch "$$FILE"; \
		tail -n 0 -F "$$FILE" 2>/dev/null | sed -u "s/^/[$$LABEL] /" & \
		TAIL_PIDS="$$TAIL_PIDS $$!"; \
	}; \
	cleanup() { \
		if [ -n "$$SCALE_PID" ]; then kill "$$SCALE_PID" 2>/dev/null || true; fi; \
		if [ -n "$$MOBILEAPI_PID" ]; then kill "$$MOBILEAPI_PID" 2>/dev/null || true; fi; \
		if [ -n "$$POLY_PID" ]; then kill "$$POLY_PID" 2>/dev/null || true; fi; \
		if [ -n "$$TAIL_PIDS" ]; then kill $$TAIL_PIDS 2>/dev/null || true; fi; \
		pgrep -f '[/]tmp/gscale-zebra/mobileapi-dev' | xargs -r kill 2>/dev/null || true; \
		pgrep -f '[/]tmp/gscale-zebra/polygon-dev' | xargs -r kill 2>/dev/null || true; \
		rm -f /tmp/gscale-zebra/mobileapi.pid /tmp/gscale-zebra/polygon.pid /tmp/gscale-zebra/scale.pid; \
	}; \
	trap 'cleanup' EXIT INT TERM; \
	"$(POLYGON_DEV_BIN)" --http-addr "$(POLYGON_HTTP_ADDR)" --bridge-state-file "$(BRIDGE_STATE_FILE)" --scenario "$(POLYGON_SCENARIO)" --seed "$(POLYGON_SEED)" >/tmp/gscale-zebra/polygon.log 2>&1 & \
	POLY_PID=$$!; \
	echo "$$POLY_PID" >/tmp/gscale-zebra/polygon.pid; \
	for i in $$(seq 1 40); do \
		if $(CURL) -fsS "http://$(POLYGON_HTTP_ADDR)/health" >/dev/null 2>&1; then \
			break; \
		fi; \
		sleep 1; \
	done; \
	if ! $(CURL) -fsS "http://$(POLYGON_HTTP_ADDR)/health" >/dev/null 2>&1; then \
		echo "run-dev: polygon failed to start"; \
		sed -n '1,160p' /tmp/gscale-zebra/polygon.log; \
		exit 1; \
	fi; \
	env MOBILE_API_ADDR="$(MOBILE_API_ADDR)" MOBILE_API_SERVER_NAME="$(MOBILE_API_SERVER_NAME)" BRIDGE_STATE_FILE="$(BRIDGE_STATE_FILE)" POLYGON_URL="http://$(POLYGON_HTTP_ADDR)" "$(MOBILEAPI_DEV_BIN)" >/tmp/gscale-zebra/mobileapi.log 2>&1 & \
	MOBILEAPI_PID=$$!; \
	echo "$$MOBILEAPI_PID" >/tmp/gscale-zebra/mobileapi.pid; \
	for i in $$(seq 1 40); do \
		if $(CURL) -fsS "http://127.0.0.1:8081/healthz" >/dev/null 2>&1; then \
			break; \
		fi; \
		sleep 1; \
	done; \
		if ! $(CURL) -fsS "http://127.0.0.1:8081/healthz" >/dev/null 2>&1; then \
			echo "run-dev: mobileapi failed to start"; \
			sed -n '1,160p' /tmp/gscale-zebra/mobileapi.log; \
			exit 1; \
		fi; \
		: > "$(SCALE_DEV_LAUNCH_LOG)"; \
		script -q -c 'cd "$(CURDIR)/scale" && exec go run . --no-bot --no-zebra --bridge-url "http://$(POLYGON_HTTP_ADDR)/api/v1/scale" --bridge-state-file "$(BRIDGE_STATE_FILE)"' "$(SCALE_DEV_LAUNCH_LOG)" >/dev/null 2>&1 & \
		SCALE_PID=$$!; \
		echo "$$SCALE_PID" >/tmp/gscale-zebra/scale.pid; \
		sleep 2; \
		if ! kill -0 "$$SCALE_PID" >/dev/null 2>&1; then \
			echo "run-dev: scale failed to start"; \
			sed -n '1,160p' "$(SCALE_DEV_LAUNCH_LOG)"; \
			exit 1; \
		fi; \
		printf '[run-dev] 1/3 simulator ready: http://%s\n' "$(POLYGON_HTTP_ADDR)"; \
		printf '[run-dev] 2/3 mobileapi ready: http://127.0.0.1:8081\n'; \
		printf '[run-dev] 3/3 core ready:      scale running in background\n'; \
		printf '[run-dev] live logs: scale print_request + polygon fake zebra\n'; \
		start_tail /tmp/gscale-zebra/polygon.log polygon; \
		start_tail "$(CURDIR)/logs/scale/worker.print_request.log" scale.print_request; \
		while :; do sleep 1; done

stop-dev-services:
	@if [ -f /tmp/gscale-zebra/scale.pid ]; then kill $$(cat /tmp/gscale-zebra/scale.pid) 2>/dev/null || true; fi
	@if [ -f /tmp/gscale-zebra/mobileapi.pid ]; then kill $$(cat /tmp/gscale-zebra/mobileapi.pid) 2>/dev/null || true; fi
	@if [ -f /tmp/gscale-zebra/polygon.pid ]; then kill $$(cat /tmp/gscale-zebra/polygon.pid) 2>/dev/null || true; fi
	@pgrep -f '[/]tmp/gscale-zebra/mobileapi-dev' | xargs -r kill 2>/dev/null || true
	@pgrep -f '[/]tmp/gscale-zebra/polygon-dev' | xargs -r kill 2>/dev/null || true
	@rm -f /tmp/gscale-zebra/mobileapi.pid /tmp/gscale-zebra/polygon.pid /tmp/gscale-zebra/scale.pid

stop-bot-services:
	@pkill -f '[g]o run ./cmd/bot' 2>/dev/null || true
	@pkill -f '/go-build/.*/[b]ot' 2>/dev/null || true
	@pkill -x bot 2>/dev/null || true

run-mobile:
	@test -d "$(MOBILE_APP_DIR)" || (echo "xato: mobile app checkout topilmadi: $(MOBILE_APP_DIR)"; exit 1)
	$(MAKE) -C "$(MOBILE_APP_DIR)" "$(MOBILE_RUN_TARGET)" FLUTTER_RUN_ARGS="$(MOBILE_FLUTTER_RUN_ARGS)"

run-mobile-android:
	@$(MAKE) run-mobile MOBILE_RUN_TARGET=run-android

run-mobile-linux:
	@$(MAKE) run-mobile MOBILE_RUN_TARGET=run-linux

run-mobile-web:
	@$(MAKE) run-mobile MOBILE_RUN_TARGET=run-web

run-mobileapi:
	go run ./cmd/mobileapi

test:
	cd bot && go test ./...
	cd bridge && go test ./...
	cd scale && go test ./...
	cd core && GOWORK=off go test ./...

test-polygon:
	cd polygon && go test ./...

test-mobileapi:
	go test ./internal/mobileapi ./cmd/mobileapi

clean:
	@if [ -d ./bin ]; then find ./bin -type f -delete; find ./bin -type d -empty -delete; fi
	@if [ -d ./dist ]; then find ./dist -type f -delete; find ./dist -type d -empty -delete; fi

autostart-install: check-env build
	sudo ./deploy/install.sh --user "$(APP_USER)" --group "$(APP_GROUP)" --start

autostart-status:
	sudo systemctl --no-pager --full status gscale-scale.service gscale-bot.service

autostart-restart:
	sudo systemctl restart gscale-scale.service gscale-bot.service

autostart-stop:
	sudo systemctl stop gscale-scale.service gscale-bot.service

release:
	./scripts/release.sh --arch amd64

release-all:
	./scripts/release.sh --arch amd64 --arch arm64
