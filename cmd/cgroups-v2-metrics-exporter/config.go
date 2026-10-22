package main

import (
	"flag"
	"fmt"
	"os"
)

const (
	// defaultHost is loopback: the metrics endpoint has no authentication, so listening on other
	// interfaces has to be asked for with --host.
	defaultHost = "127.0.0.1"
	// defaultPort is not 9100, which belongs to node_exporter and is usually taken on the same host.
	defaultPort = "10060"
)

// Config interface for the cgroups-v2-metrics-exporter binary
type Config struct {
	// Listen address in "127.0.0.1:10060" format
	ListenAddr string
	// Base path for the cgroups tracking files, e.g. "/path/to/uid.service"
	CgroupPath string
}

func GetConfig() Config {
	hostFlag := flag.String("host", envValue("METRICS_HOST", "HOST", defaultHost),
		"The IP address/host to listen on")
	portFlag := flag.String("port", envValue("METRICS_PORT", "PORT", defaultPort),
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
