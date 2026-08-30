cgroups-v2-metrics-exporter
===========================

> Like Pod metrics, from systemd user sandboxes.
>

This metrics exporter collects resource utilization metrics such as cpu and memory footprint from systemd user units.


## 💯 Compute Resource Metrics: Example

Here is a minimal example for the metrics series:

```
cgroups_v2_cpu_usage_seconds_total{systemd_user_uid="<uid>", systemd_user_unit="<unit-name>"} 1.201
cgroups_v2_memory_working_set_bytes{systemd_user_uid="<uid>", systemd_user_unit="<unit-name>"} 49152
```

### 💯 What Can You Do with It?

This exporter enables resource utilization metrics for systemd units that are deployed in user-scoped sandboxes (non-root environments).

In Kubernetes, Pod metrics are collected from [`cAdvisor`](https://github.com/google/cadvisor) (for real-time resource utilization like CPU and memory)
and [`kube-state-metrics`](https://github.com/kubernetes/kube-state-metrics) (for object metadata and health state).

The `cgroups-v2-metrics-exporter` aims to replicate Pod metrics for systemd user-scoped services by reading the underlying cgroups v2 tracking files and exposing their values as Prometheus metrics series.


## 🔥 Challenge

Resource constraints for compute workloads are expressed with
[`resources.<requests|limits>.<cpu|memory>` in the Kubernetes world](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#resource-requests-and-limits-of-pod-and-container).
For systemd units, these constraints are expressed as `CPUWeight`, `CPUQuota`, `MemoryMin`, and `MemoryMax`.
Both Kubernetes and systemd enforce the resource constraints by
[configuring cgroups v2 at the kernel-level](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#how-pods-with-resource-limits-are-run).

In systemd user-scoped services (`systemctl --user`), resource constraints are implemented by delegating the cgroups controllers to the user scope.
Therefore, the cgroups tracking files are stored underneath `/sys/fs/cgroup/user.slice/user-$(id -u).slice`.

The `cgroups-v2-metrics-exporter` discovers systemd units for the current user.
It exports resource utilization metrics, e.g. cpu and memory footprint, and applies labels for the user id and the systemd unit name to the metrics series.

### 🔭 Alternative: Scraping Metrics at the Process Level

When running an unprivileged metrics collector inside user-scoped sessions (non-root),
collecting process-level metrics from `/proc` may fail due to UID isolation.
This may (or may not) be intentional due to UID isolation.

In addition, correlating the process level metrics with metrics of the parent systemd unit may be a cumbersome and error-prone.

### 🔍 Why It Exists in the First Place

In short: an (intentional) gap in OpenTelemetry `host_metrics` and/or `systemd` receiver.

This exporter acts as a working reference implementation to solve the bottleneck outlined in [OpenTelemetry Collector Contrib - Issue #50035](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/45912).
When running the OpenTelemetry Collector agent in user scope (non-root), it fails to collect metrics at the process level (because of UID isolation).

This exporter bypasses the limitation at the process level,
thus allowing unprivileged user sandboxes to scrape their own compute resource footprints, e.g. `cpu.stat` and `memory.current`,
without requiring root privileges.


## ✨ Features

- __User Unit Discovery__:
  Automagically finds systemd user units for the active user (specifically within the `app.slice` of the current user's slice).
- __Unprivileged Scraping__:
  Collects metrics for systemd user units matching the current user's ID (`id -u`).
- __Resource Utilization Metrics__:
  Scrapes metrics of compute resources, currently
  - CPU: `cpu.stat` metrics (tbd)
  - Memory: `memory.current` metrics (tbd)

### ⚡ Non-Functional Requirements (NFRs)

This metrics exporter follows [Prometheus best practices for scheduling of exporters](https://prometheus.io/docs/instrumenting/writing_exporters/#scheduling).
Therefore, collection of metrics is pull-based and values reflect the last known value at the time when the scrape request is executed.

- __Point-in-Time Metrics__:
  Pseudo-files under `/sys/fs/cgroup/` are only read when an HTTP scrape request arrives.
- __Abuse Protection__ (Optional):
  If a subsequent scrape occurs within the TTL window, the exporter serves the last known value from memory instead of re-reading the kernel.

Benefit:
to minimize kernel I/O and reduce kernel-context-switch overhead,
the export may (optionally) implement an on-demand cache with a short TTL (e.g., 2-5 seconds),
thus offering basic protection against accidental high-frequency scrape intervals.


## 🎯 Current Scope & Known Limitations

**The current phase of this project is explicitly focused on unprivileged, user-scoped systemd unit resource metrics (`systemctl --user`).**

The `cgroups-v2-metrics-exporter` implements the discovery of systemd units in a simple and naive approach:

1. Discover systemd user units of the current user:
   - Determine the uid of the current user: `uid=$(id -u)`
   - Scan for unit names in the `app.slice` of the current user: `/sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/app.slice/<unit_name>.service/`
   - Locate the cgroups v2 tracking files for the discovered units
2. Register a metrics [Collector](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#Collector) with the Prometheus [Registry](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#Registry)
   - Attach labels for `{systemd_user_uid="<uid>", systemd_user_unit="<unit_name>"}` to the metrics series
3. Expose the registered metrics series in Prometheus Exposition Format for HTTP-scraping at `http://127.0.0.1:9100/metrics`

It only discovers user units on startup (for the time being).


## 🎨 Future Scope

If you wish to bring this exporter to production-grade, feel free to reach out.


## 🛠️ Related Work

* For system-wide cgroups v2 Pressure Stall Information (PSI) monitoring, check out [arianvp/cgroup-exporter](https://github.com/arianvp/cgroup-exporter/).
* Gathering metrics from explicitly targeted, high-level paths like `user.slice` or specialized workloads like Slurm clusters, check out [treydock/cgroup_exporter](https://github.com/treydock/cgroup_exporter).
