# Validation Module Makefile

MODULE_NAME=validation-module
BUILD_DIR=../../../build/wasm
SOURCE_FILE=main.go

.PHONY: build clean test

build: $(BUILD_DIR)/$(MODULE_NAME).wasm

$(BUILD_DIR)/$(MODULE_NAME).wasm: $(SOURCE_FILE)
	@mkdir -p $(BUILD_DIR)
	GOOS=wasip1 GOARCH=wasm go build -o $(BUILD_DIR)/$(MODULE_NAME).wasm $(SOURCE_FILE)
	@echo "Built WASM module: $(BUILD_DIR)/$(MODULE_NAME).wasm"

test:
	go test -v ./...

clean:
	rm -f $(BUILD_DIR)/$(MODULE_NAME).wasm

install-tools:
	@echo "No additional tools required for this module"

.PHONY: build test clean install-tools