# cgroups-v2-metrics-exporter

A lightweight, low-overhead Prometheus metrics exporter written in Go for the Linux cgroups v2 unified hierarchy.


## 🎯 Current Project Scope

While designed with a broad modular architecture to support the full cgroups v2 tree, **the current phase of this project is explicitly focused on unprivileged, user-scoped systemd unit resource metrics (`systemctl --user`).**

### Why this exists (The OpenTelemetry Hostmetrics Gap)

This exporter acts as a working reference implementation to solve the bottleneck outlined in [OpenTelemetry Collector Contrib - Issue #50035](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/45912). When running an unprivileged telemetry collector agent inside user-scoped sessions, process-level collection via `/proc` fails due to UID isolation. 

By leveraging cgroups delegation, this exporter bypasses that limitation—allowing unprivileged user sandboxes to scrape their own `cpu.stat` and `memory.current` footprints without requiring root privileges.

### Features

- Scrape metrics for systemd user units of the current user (`id -u`).
- Discover user units of the current user (within `app.slice` of the current user's slice)
- Scrape resource utilization metrics:
  - cpu: tbd
  - memory: tbd

### Non-Functional Requirements (NFRs)

#### ⚡ Minimal Kernel I/O

Traditional scrapers execute unbuffered file reads on every live Proemtheus HTTP request.
As an option, this exporter could implements an internal background **Ticker-Cache Worker**.

Benefits:
* **Regulated Scrapes:** Pseudo-files under `/sys/fs/cgroup/` are sampled via a controlled background goroutine loop.
* **Kernel Protection:** The `/metrics` endpoint serves from an in-memory map, eliminating kernel-context-switch and disk I/O spikes in high-frequency scraping intervals.


## Implementation

- cgroups base path: `/sys/fs/cgroup`
- user slice of the current user: `/sys/fs/cgroup/user.slice/user-$(id -u).slice/`
- user unit slice of app slice: `/sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/app.slice/<user_unit>.service/`


## 🛠️ Related Work

* For system-wide cgroups v2 Pressure Stall Information (PSI) monitoring, check out [arianvp/cgroup-exporter](https://github.com/arianvp/cgroup-exporter/).
* Gathering metrics from explicitly targeted, high-level paths like `user.slice` or specialized workloads like Slurm clusters, check out [treydock/cgroup_exporter](https://github.com/treydock/cgroup_exporter).
