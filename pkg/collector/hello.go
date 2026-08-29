package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Fake hello prometheus metrics collector...
type helloPrometheusCollector struct {
	helloDesc *prometheus.Desc
	fakeValue float64
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
	c.fakeValue += 1
	ch <- prometheus.MustNewConstMetric(c.helloDesc, prometheus.CounterValue, c.fakeValue)
}
