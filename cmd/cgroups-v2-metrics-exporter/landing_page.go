package main

import (
	"fmt"
	"net/http"
)

func LandingPage(w http.ResponseWriter, r *http.Request) {
	// Prevent catching all sub-paths arbitrarily
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `
		<!http-equiv="Content-Type" content="text/html; charset=utf-8">
		<html>
		<head><title>cgroups-v2-metrics-exporter</title></head>
		<body>
		<h1>cgroups-v2-metrics-exporter</h1>
		<p><a href="/metrics">Metrics Endpoint</a></p>
		</body>
		</html>
	`)
}
