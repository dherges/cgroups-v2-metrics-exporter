.PHONY: build clean

TARGET_DIR=bin

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
	@echo "Serving http server..."
	./bin/cgroups-v2-metrics-exporter server
