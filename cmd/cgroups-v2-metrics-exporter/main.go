package main

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"cgroups-v2-metrics-exporter/internal/collector"
	"cgroups-v2-metrics-exporter/internal/discovery"
)

// readHeaderTimeout is how long a client may take to send its request headers. It matches
// Prometheus' default scrape timeout.
const readHeaderTimeout = 10 * time.Second

// resolveFunc finds the cgroup tree of the user manager.
type resolveFunc func() (*discovery.UserCgroupInfo, error)

func main() {
	conf := GetConfig()

	// Resolved on every scrape so the exporter also starts on hosts without cgroups v2 or without a
	// user manager (it then exports no unit metrics) and recovers once they appear.
	resolve := func() (*discovery.UserCgroupInfo, error) {
		return discovery.DiscoverUserContext(conf.CgroupPath)
	}
	discover := func() ([]discovery.Unit, error) {
		userInfo, err := resolve()
		if err != nil {
			return nil, err
		}
		return userInfo.Discover()
	}

	log.Printf("Discovering systemd user units...")
	logStartup(resolve)

	registry := prometheus.NewRegistry()
	if err := registry.Register(collector.NewCgroupCollector(discover)); err != nil {
		log.Fatalf("Failed to register metrics collector: %v", err)
	}

	srv := newServer(conf.ListenAddr, registry)
	log.Printf("Starting Metrics Exporter on %s...", conf.ListenAddr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Metrics Exporter failed to start: %v", err)
	}
}

// logStartup reports what the exporter found at startup. It only logs: a missing hierarchy or
// controller must not stop the exporter, because the scrape recovers once the host is ready.
func logStartup(resolve resolveFunc) {
	userInfo, err := resolve()
	if err != nil {
		log.Printf("Warning: no usable cgroups v2 user hierarchy, exporting no unit metrics until it appears: %v", err)
		return
	}
	log.Printf("Discovered environment: UID=%s, BaseCgroupPath=%s", userInfo.UID, userInfo.BaseCpath)

	missing, err := userInfo.MissingControllers()
	if err != nil {
		log.Printf("Warning: cannot check delegated cgroup controllers: %v", err)
		return
	}
	if len(missing) > 0 {
		log.Printf("Warning: controllers %v are not delegated to the user manager, their metrics will be missing (see README: Delegate=cpu memory pids io)", missing)
	}
}

// newServer builds the HTTP server on its own mux, so nothing registered on http.DefaultServeMux
// by an imported package (net/http/pprof, for one) is ever served. ReadHeaderTimeout stops a client
// that never finishes its request headers from holding a connection open forever.
func newServer(addr string, registry *prometheus.Registry) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/", LandingPage)
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}
}
