package main

import (
	"log"
	"net/http"
	"cgroups-v2-metrics-exporter/pkg/collector"
	"cgroups-v2-metrics-exporter/pkg/discovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	conf := GetConfig()

	log.Printf("Discovering systemd user units...")
	userInfo, err := discovery.DiscoverUserContext(conf.CgroupPath)
	if err != nil {
		log.Fatalf("Failed to initialize systemd user discovery: %v", err)
	}
	log.Printf("Discovered environment: UID=%s, BaseCgroupPath=%s", userInfo.UID, userInfo.BaseCpath)

	registry := prometheus.NewRegistry()
	collector := collector.NewHelloCollector()
	if err := registry.Register(collector); err != nil {
		log.Fatalf("Failed to register metrics collector: %v", err)
	}
	handlerOpts := promhttp.HandlerOpts{}
	http.Handle("/metrics", promhttp.HandlerFor(registry, handlerOpts))
	http.HandleFunc("/", LandingPage)

	// Start http server
	log.Printf("Starting Metrics Exporter on %s...\n", conf.ListenAddr)
	if err := http.ListenAndServe(conf.ListenAddr, nil); err != nil {
		log.Fatalf("Metrics Exporter failed to start: %v", err)
	}
}
