package main

import (
	"cgroups-v2-metrics-exporter/pkg/collector"
	"cgroups-v2-metrics-exporter/pkg/discovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
)

func main() {
	conf := GetConfig()

	log.Printf("Discovering systemd user units...")
	// Resolved on every scrape so the exporter also starts on hosts without cgroups v2 or without a
	// user manager (it then exports no unit metrics) and recovers once they appear.
	discover := func() ([]discovery.Unit, error) {
		userInfo, err := discovery.DiscoverUserContext(conf.CgroupPath)
		if err != nil {
			return nil, err
		}
		return userInfo.Discover()
	}
	if userInfo, err := discovery.DiscoverUserContext(conf.CgroupPath); err != nil {
		log.Printf("Warning: no usable cgroups v2 user hierarchy, exporting no unit metrics until it appears: %v", err)
	} else {
		log.Printf("Discovered environment: UID=%s, BaseCgroupPath=%s", userInfo.UID, userInfo.BaseCpath)
		if missing, err := userInfo.MissingControllers(); err != nil {
			log.Printf("Warning: cannot check delegated cgroup controllers: %v", err)
		} else if len(missing) > 0 {
			log.Printf("Warning: controllers %v are not delegated to the user manager (see README: Delegate=cpu memory pids)", missing)
		}
	}

	registry := prometheus.NewRegistry()
	cgroupCollector := collector.NewCgroupCollector(discover)
	if err := registry.Register(cgroupCollector); err != nil {
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
