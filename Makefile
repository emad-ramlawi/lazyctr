.PHONY: build build-static clean install run

APP_NAME=lazyctr
VERSION?=1.0.0
BUILD_DIR=./build

build:
	@echo "Building $(APP_NAME)..."
	go build -o $(BUILD_DIR)/$(APP_NAME) .

build-static:
	@echo "Building static binary $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-a \
		-ldflags '-extldflags "-static" -s -w -X main.Version=$(VERSION)' \
		-o $(BUILD_DIR)/$(APP_NAME) .
	@echo "Static binary created: $(BUILD_DIR)/$(APP_NAME)"
	@ls -lh $(BUILD_DIR)/$(APP_NAME)

build-static-arm64:
	@echo "Building static binary $(APP_NAME) for ARM64..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-a \
		-ldflags '-extldflags "-static" -s -w -X main.Version=$(VERSION)' \
		-o $(BUILD_DIR)/$(APP_NAME)-arm64 .
	@echo "Static binary created: $(BUILD_DIR)/$(APP_NAME)-arm64"

install: build-static
	@echo "Installing $(APP_NAME) to /usr/local/bin..."
	sudo install -m 755 $(BUILD_DIR)/$(APP_NAME) /usr/local/bin/$(APP_NAME)
	@echo "Installation complete! Run with: sudo $(APP_NAME)"

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

run:
	@echo "Running $(APP_NAME)..."
	sudo go run .

help:
	@echo "Available targets:"
	@echo "  build         - Build the application"
	@echo "  build-static  - Build static binary (default: linux/amd64)"
	@echo "  build-static-arm64 - Build static binary for ARM64"
	@echo "  install       - Build and install to /usr/local/bin"
	@echo "  clean         - Remove build artifacts"
	@echo "  run           - Run the application with sudo"
	@echo "  help          - Show this help message"
