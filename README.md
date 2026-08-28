# cgroups-v2-metrics-exporter

A lightweight, low-overhead Prometheus exporter written in Go for the Linux cgroups v2 unified hierarchy. 

## 🎯 Current Project Scope

While designed with a broad modular architecture to support the full cgroups v2 tree, **Phase 1 of this project is explicitly focused on unprivileged, user-scoped systemd unit resource metrics (`systemd --user`).**

### Why this exists (The OpenTelemetry Hostmetrics Gap)

This exporter acts as a working reference implementation to solve the bottleneck outlined in [OpenTelemetry Collector Contrib - Issue #50035](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/45912). When running an unprivileged telemetry collector agent inside user-scoped sessions, process-level collection via `/proc` fails due to UID isolation. 

By leveraging cgroup delegation, this exporter bypasses that limitation—allowing unprivileged user sandboxes to securely scrape their own `cpu.stat` and `memory.current` footprints without requiring root privileges.

## ⚡ High-Performance Architecture (Low Kernel I/O)

Unlike traditional scrapers that execute unbuffered file reads on every live Prometheus HTTP request, this exporter implements an internal background **Ticker-Cache Worker**. 

* **Regulated Scrapes:** Pseudo-files under `/sys/fs/cgroup/` are sampled via a controlled background goroutine loop.
* **Kernel Protection:** The `/metrics` endpoint serves strictly from an in-memory map, eliminating kernel-context-switch and disk I/O spikes during high-frequency scraping intervals.

## 🛠️ Related Work

* For system-wide cgroups v2 Pressure Stall Information (PSI) monitoring, check out [arianvp/cgroup-exporter](https://github.com/arianvp/cgroup-exporter/).

Popular Cgroups v2 Exportersarianvp/cgroup-exporter: A lightweight, unified-hierarchy-only cgroup-exporter command that tracks Pressure Stall Information (io.pressure, memory.pressure, cpu.pressure).flipkart-incubator/cgroupv2_exporter: A modular Go-based Prometheus exporter featuring pluggable metric collectors tailored specifically for cgroup v2 architecture.treydock/cgroup_exporter: A flexible exporter supporting custom config paths and Docker deployments via --path.cgroup.root.

