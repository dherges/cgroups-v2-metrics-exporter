.PHONY: build build-linux-amd64 clean

BIN := ./bin
FAKE_DATA_DIR := $(PWD)/fake-data
TARGET := cgroups-v2-metrics-exporter

LOCAL_HOST ?= 127.0.0.1
LOCAL_PORT ?= 10060

build:
	@echo "=== Compiling $(TARGET) ==="
	@mkdir -p $(BIN)
	go build -o $(BIN)/$(TARGET) ./cmd/$(TARGET)
	@echo "Done! All binaries in $(BIN)/"

# static binary for Linux hosts (x86_64)
build-linux-amd64:
	@mkdir -p $(BIN)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o $(BIN)/$(TARGET)-linux-amd64 ./cmd/$(TARGET)
	@echo "Done! $(BIN)/$(TARGET)-linux-amd64"

build-windows:
	GOOS=windows GOARCH=amd64 go build -o $(BIN)/$(TARGET).exe ./cmd/$(TARGET)

clean:
	@echo "Cleaning up old builds..."
	go clean -cache
	@rm -rf $(BIN)

serve: build
	@echo "=== Serving Metrics Exporter ==="
	@export CGROUP_BASE_PATH=$(FAKE_DATA_DIR); \
	$(BIN)/$(TARGET)

run: build
	@echo "=== Launching & Testing Exporter ==="
	@export CGROUP_BASE_PATH=$(FAKE_DATA_DIR); \
	export METRICS_HOST=$(LOCAL_HOST); \
	export METRICS_PORT=$(LOCAL_PORT); \
	$(BIN)/$(TARGET) & pid=$$!; \
	trap "echo '=== Cleaning Up ==='; kill $$pid 2>/dev/null || true" EXIT INT TERM; \
	sleep 0.5; \
	echo "Executing: curl http://$(LOCAL_HOST):$(LOCAL_PORT)/metrics"; \
	echo "------------------------------------------------"; \
	curl --silent http://$(LOCAL_HOST):$(LOCAL_PORT)/metrics; \
	echo "------------------------------------------------"

test:
	@echo "=== Running Go Tests ==="
	go test -v ./...
