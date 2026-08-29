package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func getEnvValue(primaryEnvVar string, secondaryEnvVar string, defaultValue string) string {
	primaryValue := os.Getenv(primaryEnvVar)
	if primaryValue != "" {
		return primaryValue
	}

	secondaryValue := os.Getenv(secondaryEnvVar)
	if secondaryValue != "" {
		return secondaryValue
	}

	return defaultValue
}

func getListenAddress() string {
	envHost := getEnvValue("METRICS_HOST", "HOST", "0.0.0.0")
	hostFlag := flag.String("host", envHost, "The IP address/host to listen on")

	envPort := getEnvValue("METRICS_PORT", "PORT", "9100")
	portFlag := flag.String("port", envPort, "The port to expose Prometheus metrics on")

	flag.Parse()

	return fmt.Sprintf("%s:%s", *hostFlag, *portFlag)
}

func main() {
	helloMetric := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "hello_prometheus_total",
		Help: "A fake metric to test the skeleton exporter setup.",
	})
	if err := prometheus.Register(helloMetric); err != nil {
		log.Fatalf("Failed to register metric: %v", err)
	}

	// Register prometheus metrics handlers
	http.Handle("/metrics", promhttp.Handler())

	// Start http server
	addr := getListenAddress()
	log.Printf("Starting Metrics Exporter on %s...\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Metrics Exporter failed to start: %v", err)
	}
}
