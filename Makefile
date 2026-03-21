# Variables
APP_NAME := kubevision
BUILD_DIR := dist

# Default build
all: build

# Clean build directory
clean:
	rm -rf $(BUILD_DIR)
	mkdir -p $(BUILD_DIR)

# Build for current platform
build:
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/kubevision

# Install dependencies
deps:
	go mod tidy
	go mod download

.PHONY: all clean build deps