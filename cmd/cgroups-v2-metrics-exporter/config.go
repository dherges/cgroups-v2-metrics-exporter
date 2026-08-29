package main

import (
	"flag"
	"fmt"
	"os"
)

// Config interface for the cgroups-v2-metrics-exporter binary
type Config struct {
	// Listen address in "127.0.0.1:9100" format
	ListenAddr string
	// Base path for the cgroups tracking files, e.g. "/path/to/uid.service"
	CgroupPath string
}

func GetConfig() Config {
	hostFlag := flag.String("host", envValue("METRICS_HOST", "HOST", "0.0.0.0"),
		"The IP address/host to listen on")
	portFlag := flag.String("port", envValue("METRICS_PORT", "PORT", "9100"),
		"The port to expose Prometheus metrics on")
	cgroupPathFlag := flag.String("cgroup-base-path",
		envValue("METRICS_CGROUP_BASE_PATH", "CGROUP_BASE_PATH", ""),
		"Override the base cgroups v2 path for testing/Codespaces")
	flag.Parse()

	return Config{
		ListenAddr: fmt.Sprintf("%s:%s", *hostFlag, *portFlag),
		CgroupPath: *cgroupPathFlag,
	}
}

func envValue(env1 string, env2 string, defValue string) string {
	if val1 := os.Getenv(env1); val1 != "" {
		return val1
	}
	if val2 := os.Getenv(env2); val2 != "" {
		return val2
	}
	return defValue
}
