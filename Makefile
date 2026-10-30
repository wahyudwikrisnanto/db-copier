WAILS ?= $(shell command -v wails 2>/dev/null || echo $(HOME)/go/bin/wails)
TAGS  := webkit2_41
# Undo the VS Code snap terminal's GTK environment, which crashes GTK apps.
RUN   := ./scripts/clean-env.sh

.PHONY: dev build run test

dev:
	$(RUN) $(WAILS) dev -tags $(TAGS)

build:
	$(WAILS) build -tags $(TAGS)

run: build
	$(RUN) ./build/bin/db-copier

test:
	go test ./internal/...
