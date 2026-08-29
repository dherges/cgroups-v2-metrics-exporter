package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"cgroups-v2-metrics-exporter/pkg/sd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Config struct {
	ListenAddr string
	CgroupPath string
}

func getEnvValue(primaryEnvVar string, secondaryEnvVar string, defaultValue string) string {
	if primaryValue := os.Getenv(primaryEnvVar); primaryValue != "" {
		return primaryValue
	}
	if secondaryValue := os.Getenv(secondaryEnvVar); secondaryValue != "" {
		return secondaryValue
	}
	return defaultValue
}

func getConfig() Config {
	envHost := getEnvValue("METRICS_HOST", "HOST", "0.0.0.0")
	hostFlag := flag.String("host", envHost, "The IP address/host to listen on")

	envPort := getEnvValue("METRICS_PORT", "PORT", "9100")
	portFlag := flag.String("port", envPort, "The port to expose Prometheus metrics on")

	envCgroupPath := getEnvValue("METRICS_CGROUP_BASE_PATH", "CGROUP_BASE_PATH", "")
	cgroupPathFlag := flag.String("cgroup-base-path", envCgroupPath, "Override the base cgroups v2 path for testing/Codespaces")

	flag.Parse()

	return Config{
		ListenAddr: fmt.Sprintf("%s:%s", *hostFlag, *portFlag),
		CgroupPath: *cgroupPathFlag,
	}
}

func main() {
	conf := getConfig()

	log.Printf("Discovering systemd user units...")
	userInfo, err := sd.DiscoverUserContext(conf.CgroupPath)
	if err != nil {
		log.Fatalf("Failed to initialize systemd user discovery: %v", err)
	}
	log.Printf("Discovered environment: UID=%s, BaseCgroupPath=%s", userInfo.UID, userInfo.BaseCpath)

	registry := prometheus.NewRegistry()
	collector := NewHelloCollector()
	if err := registry.Register(collector); err != nil {
		log.Fatalf("Failed to register metrics collector: %v", err)
	}
	handlerOpts := promhttp.HandlerOpts{}
	http.Handle("/metrics", promhttp.HandlerFor(registry, handlerOpts))

	// Start http server
	log.Printf("Starting Metrics Exporter on %s...\n", conf.ListenAddr)
	if err := http.ListenAndServe(conf.ListenAddr, nil); err != nil {
		log.Fatalf("Metrics Exporter failed to start: %v", err)
	}
}


// Fake hello prometheus metrics scollector...
type helloPrometheusCollector struct {
	helloDesc *prometheus.Desc
}

func NewHelloCollector() *helloPrometheusCollector {
	return &helloPrometheusCollector{
		helloDesc: prometheus.NewDesc(
			"hello_prometheus_total",
			"A fake metric to test the skeleton exporter setup.",
			nil, nil,
		),
	}
}

func (c *helloPrometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.helloDesc
}

func (c *helloPrometheusCollector) Collect(ch chan<- prometheus.Metric) {
	var fakeValue float64 = 1
	ch <- prometheus.MustNewConstMetric(c.helloDesc, prometheus.CounterValue, fakeValue)
}
