.PHONY: build clean

TARGET_DIR=bin
TARGET_PORT?=9100
MOCK_DIR=$(PWD)/fake-units

build:
	@echo "Compiling..."
	@mkdir -p $(TARGET_DIR)
	go build -o $(TARGET_DIR)/cgroups-v2-metrics-exporter ./cmd/cgroups-v2-metrics-exporter
	@echo "Done! All binaries in ./$(TARGET_DIR)/"

build-windows:
	GOOS=windows GOARCH=amd64 go build -o $(TARGET_DIR)/cgroups-v2-metrics-exporter.exe ./cmd/cgroups-v2-metrics-exporter

clean:
	@echo "Cleaning up old builds..."
	go clean -cache
	@rm -rf $(TARGET_DIR)

serve: build
	@echo "Serving metrics..."
	./$(TARGET_DIR)/cgroups-v2-metrics-exporter

run: build
	@echo "=== Launching & Testing Exporter ==="
	@export CGROUP_BASE_PATH=$(MOCK_DIR); \
	export METRICS_PORT=$(TARGET_PORT); \
	./$(TARGET_DIR)/cgroups-v2-metrics-exporter & pid=$$!; \
	trap "echo '=== Cleaning Up ==='; kill $$pid 2>/dev/null || true" EXIT INT TERM; \
	sleep 0.5; \
	echo "Executing: curl http://127.0.0.1:$(TARGET_PORT)/metrics"; \
	echo "------------------------------------------------"; \
	curl -s http://127.0.0.1:$(TARGET_PORT)/metrics; \
	echo "------------------------------------------------"
