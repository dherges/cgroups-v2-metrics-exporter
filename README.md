# cgroups-v2-metrics-exporter

A lightweight, low-overhead Prometheus metrics exporter written in Go for the Linux cgroups v2 unified hierarchy.


## 🚀 Getting Started

Run the cgroups v2 metrics exporter in user land:

```bash
./cgroups-v2-metrics-exporter --host 127.0.0.1 --port 9100
```

Scrape Prometheus metrics at `http://127.0.0.1:9100/metrics`


## 🎯 Current Project Scope

**The current phase of this project is explicitly focused on unprivileged, user-scoped systemd unit resource metrics (`systemctl --user`).**

### 🔍 Why It Exists in the First Place

In short: an (intentional) gap in OpenTelemetry `host_metrics` receiver.

This exporter acts as a working reference implementation to solve the bottleneck outlined in [OpenTelemetry Collector Contrib - Issue #50035](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/45912). When running an unprivileged OpenTelemetry Collector agent inside user-scoped sessions (non-root), collecting process-level metrics from `/proc` fails. This may (or may not) be intentional due to UID isolation.

In systemd user-scoped services, resource constraints for CPU and memory are implemented by delegating cgroups v2 controllers to the user scope. This exporter bypasses the limitation at the process level, thus allowing unprivileged user sandboxes to scrape their own compute resource footprints, e.g. `cpu.stat` and `memory.current`, without requiring root privileges.

### 💯 What Can You Do with It?

In Kubernetes, Pod metrics are collected from [`cAdvisor`](https://github.com/google/cadvisor) (for real-time resource utilization like CPU and memory) and [`kube-state-metrics`](https://github.com/kubernetes/kube-state-metrics) (for object metadata and health state). The `cgroups-v2-metrics-exporter` aims to replicate Pod metrics for systemd user-scoped services by exposing the underlying cgroups v2 tracking files in Prometheus metrics series.

This exporter enables resource utilization metrics for systemd units that are deployed in user-scoped sandboxes (non-root environmments).

### ✨ Features

- __User Unit Discovery__:
  Automagically finds systemd user units for the active user (specifically within the `app.slice` of the current user's slice).
- __Unprivileged Scraping__:
  Collects metrics for systemd user units matching the current user's ID (`id -u`).
- __Resource Utilization Metrics__:
  Scrapes metrics of compute resources, currently
  - CPU: `cpu.stat` metrics (tbd)
  - Memory: `memory.current` metrics (tbd)

### ⚡ Non-Functional Requirements (NFRs)

- __Minimal Kernel I/O__:
  To prevent kernel-context-switch overhead during accidental high-frequency scraping intervals,
  the exporter implements an on-demand synchronous cache with a short TTL (e.g., 2–5 seconds).
- __Synchronous Resolution__:
  Pseudo-files under `/sys/fs/cgroup/` are only read when an HTTP scrape request arrives.
- __Abuse Protection__:
  If a subsequent scrape occurs within the TTL window, the exporter serves the in-memory payload instead of re-reading the kernel.

Benefit: by choosing synchronous resolution the exporter follows [Prometheus best practices for scheduling of exporters](https://prometheus.io/docs/instrumenting/writing_exporters/#scheduling). Scraping remains synchronous and pull-driven. At the same time, it limits reads to the TTL duration (e.g., max once per 5 seconds), thus offering some protection against accidental abusive scrapes.


## 🎨 Implementation

The cgroups v2 exporter implements the metrics exposition in a simple approach:

1. Discover systemd user units of the current user:
   - Determine the uid of the current user: `uid=$(id -u)`
   - Scan for unit names in the `app.slice` of the current user: `/sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/app.slice/<unit_name>.service/`
   - Locate the cgroups v2 tracking files for the discovered units
2. Register a metrics [Collector](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#Collector) with the Prometheus [Registry](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#Registry)
   - Attach labels for `{systemd_user_uid="<uid>", systemd_user_unit="<unit_name>"}` to the metrics series
3. Expose the registered metrics series in Prometheus Exposition Format for HTTP-scraping at `http://127.0.0.1:9100/metrics`


## 🛠️ Related Work

* For system-wide cgroups v2 Pressure Stall Information (PSI) monitoring, check out [arianvp/cgroup-exporter](https://github.com/arianvp/cgroup-exporter/).
* Gathering metrics from explicitly targeted, high-level paths like `user.slice` or specialized workloads like Slurm clusters, check out [treydock/cgroup_exporter](https://github.com/treydock/cgroup_exporter).
