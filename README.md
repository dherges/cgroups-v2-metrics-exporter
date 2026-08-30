cgroups-v2-metrics-exporter
===========================

A lightweight, low-overhead Prometheus metrics exporter written in Go for the Linux cgroups v2 unified hierarchy.


## 🚀 Getting Started

Run the cgroups v2 metrics exporter in user land:

```bash
./cgroups-v2-metrics-exporter --host 127.0.0.1 --port 9100
```

Scrape Prometheus metrics at `http://127.0.0.1:9100/metrics`


## 🎯 Current Project Scope

**The current phase of this project is explicitly focused on unprivileged, user-scoped systemd unit resource metrics (`systemctl --user`).**

### 💯 What Can You Do with It?

In Kubernetes,
Pod metrics are collected from [`cAdvisor`](https://github.com/google/cadvisor) (for real-time resource utilization like CPU and memory)
and [`kube-state-metrics`](https://github.com/kubernetes/kube-state-metrics) (for object metadata and health state).

The `cgroups-v2-metrics-exporter` aims to replicate Pod metrics for systemd user units.
It enables resource utilization metrics for systemd services that are deployed in user-scoped sandboxes (non-root environmments).

### 🔍 Why It Exists in the First Place

In short: an (intentional) gap in OpenTelemetry `host_metrics` and/or `systemd` receiver.

This exporter acts as a working reference implementation to solve the bottleneck outlined in
[OpenTelemetry Collector Contrib - Issue #50035](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/45912).

In systemd user-scoped services,
resource constraints for CPU and memory are implemented by delegating cgroups v2 controllers to the user scope.

The `systemd` receiver in the OpenTelemetry Collector is focused on scraping primarily the systemd unit state
(active, reloading, failed, and so on).

Collecting process-level metrics from `/proc` fails,
when running the the `host_metrics` receiver in an unprivileged OpenTelemetry Collector agent inside user-scoped sessions (non-root).
This may (or may not) be intentional due to UID isolation.

This exporter bypasses the limitation at the process level,
allowing unprivileged user sandboxes to scrape their own compute resource footprints,
e.g. `cpu.stat` and `memory.current`, without requiring root privileges.


## 👉 Usage Example

To benefit from this metrics exporter, systemd must be configured such that resource controllers are delegated to the user scope.

### Enable User Lingering 

Allow lingering for the the non-root user.
It enables the user to keep its background services and user manager instance running even after they log out:

```bash
loginctl enable-linger <username>
```

### Enable Cgroups Delegation

To allow the non-root user to manage its own resource controller, enable control group delegation:

```bash
# Create the override directory
sudo mkdir -p /etc/systemd/system/user@.service.d/

# Add the delegation configuration
cat <<EOF | sudo tee /etc/systemd/system/user@.service.d/override.conf
[Service]
Delegate=cpu memory pids
EOF
```

### Apply Systemd Changes

Reload the systemd manager configuration and restart the user service manager:

```bash
sudo systemctl daemon-reload
sudo systemctl restart user@<uid>.service
```

### Set-Up A Sandboxed App

Save a unit file to the user-specific systemd directory, e.g. `~/.config/systemd/user/sandbox-app.service`:

```ini
[Unit]
Description=Sandbox User Application with Resource Metrics

[Service]
Type=exec
ExecStart=/usr/local/bin/my-app --port 8080
Restart=on-failure

Delegate=yes
CPUAccounting=yes
MemoryAccounting=yes
TasksAccounting=yes

## Equivalent to `resources.limits.cpu: 2000m` in Kubernetes notation
CPUQuota=200%

## Equivalent to `resources.limits.memory: 1Gi` in Kubernetes notation
MemoryMax=1G

[Install]
WantedBy=default.target
```

Reload the user manager and enable/start the user-scoped service:

```bash
systemctl --user daemon-reload
systemctl --user enable sandbox-app.service
systemctl --user start sandbox-app.service
```
