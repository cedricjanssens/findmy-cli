SHELL := /bin/bash
BIN := bin
HELPER_SRC := helpers/findmy-helper/main.swift
HELPER_BIN := $(BIN)/findmy-helper
GO_BIN := $(BIN)/findmy
FACE_SRC := helpers/face-detect/face-detect.swift
FACE_BIN := $(BIN)/face-detect

.PHONY: all build helper face-detect clean install

all: build

build: helper $(GO_BIN)

helper: $(HELPER_BIN)

face-detect: $(FACE_BIN)

$(HELPER_BIN): $(HELPER_SRC)
	@mkdir -p $(BIN)
	swiftc -O -o $@ $<

$(FACE_BIN): $(FACE_SRC)
	@mkdir -p $(BIN)
	swiftc -O -framework AVFoundation -framework CoreML -o $@ $<

# Install AdaFace IR-18 Core ML model (~42 MB download, ~48 MB extracted)
FACE_MODEL_DIR := /opt/homebrew/share/face-detect
FACE_MODEL_URL := https://github.com/john-rocky/CoreML-Models/releases/download/adaface-v1/AdaFace_IR18.mlpackage.zip

face-detect-model:
	@if [ -d "$(FACE_MODEL_DIR)/AdaFace_IR18.mlpackage" ]; then \
		echo "Model already installed at $(FACE_MODEL_DIR)"; \
	else \
		echo "Downloading AdaFace IR-18..."; \
		mkdir -p $(FACE_MODEL_DIR); \
		curl -fL --output /tmp/AdaFace_IR18.mlpackage.zip $(FACE_MODEL_URL); \
		cd $(FACE_MODEL_DIR) && unzip -q -o /tmp/AdaFace_IR18.mlpackage.zip; \
		rm /tmp/AdaFace_IR18.mlpackage.zip; \
		echo "Model installed at $(FACE_MODEL_DIR)/AdaFace_IR18.mlpackage"; \
	fi

$(GO_BIN): $(shell find cmd internal -name '*.go') go.mod
	@mkdir -p $(BIN)
	go build -o $@ ./cmd/findmy

clean:
	rm -rf $(BIN)

install: build $(FACE_BIN) face-detect-model
	cp $(GO_BIN) $(HELPER_BIN) $(FACE_BIN) /usr/local/bin/

# Reinstall over Homebrew without sudo. Run `make claim` once first.
BREW_BIN := $(shell brew --cellar findmy-cli 2>/dev/null)/0.1.0/bin

reinstall: build
	@if [ -w "$(BREW_BIN)/findmy" ] 2>/dev/null; then \
		cp $(GO_BIN) $(BREW_BIN)/findmy && cp $(HELPER_BIN) $(BREW_BIN)/findmy-helper && echo "Installed to $(BREW_BIN)"; \
	else \
		echo "error: $(BREW_BIN) not writable. Run 'make claim' once (needs sudo)."; exit 1; \
	fi

# One-time: make Homebrew binaries writable so reinstall works without sudo.
claim:
	chmod u+w "$(BREW_BIN)/findmy" "$(BREW_BIN)/findmy-helper"
	@echo "Done — 'make reinstall' now works without sudo."
