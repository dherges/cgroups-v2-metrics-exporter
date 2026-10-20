cgroups-v2-metrics-exporter
===========================

A lightweight, low-overhead Prometheus metrics exporter written in Go for the Linux cgroups v2 unified hierarchy.


## 🚀 Getting Started

Run the cgroups v2 metrics exporter in user land:

```bash
./cgroups-v2-metrics-exporter --host 127.0.0.1 --port 9100
```

Scrape Prometheus metrics at `http://127.0.0.1:9100/metrics`

### Options

| Flag | Environment variables | Default | Description |
|------|-----------------------|---------|-------------|
| `--host` | `METRICS_HOST`, `HOST` | `0.0.0.0` | IP address/host to listen on |
| `--port` | `METRICS_PORT`, `PORT` | `9100` | Port to expose the metrics on |
| `--cgroup-base-path` | `METRICS_CGROUP_BASE_PATH`, `CGROUP_BASE_PATH` | derived from the process uid | Override the cgroups v2 base directory (testing, Codespaces) |

The flag default is taken from the environment variable, so an explicit flag wins. `METRICS_*` takes precedence over the plain name.

> [!NOTE]
> The default `0.0.0.0` exposes the unauthenticated metrics on all interfaces. Pass `--host 127.0.0.1` (as above) unless a remote Prometheus needs to scrape it directly.

By default the cgroup base is `/sys/fs/cgroup/user.slice/user-<uid>.slice/user@<uid>.service`, where `<uid>` is the uid the exporter runs as.

### Metrics Series Reference

Units are discovered on every scrape (`app.slice/*.service` of the user manager), so started, stopped and restarted units show up on the next scrape.

All series carry the labels `systemd_user_uid` and `systemd_user_unit`. Extra labels are listed below.

| Metric | Type | Extra label | Source | Description |
|--------|------|-------------|--------|-------------|
| `cgroups_v2_cpu_usage_seconds_total` | counter | | `cpu.stat` | CPU time consumed |
| `cgroups_v2_cpu_periods_total` | counter | | `cpu.stat` | Enforcement periods elapsed |
| `cgroups_v2_cpu_throttled_periods_total` | counter | | `cpu.stat` | Periods in which the unit was throttled |
| `cgroups_v2_cpu_throttled_seconds_total` | counter | | `cpu.stat` | Total time the unit was throttled |
| `cgroups_v2_cpu_limit_cores` | gauge | | `cpu.max` | CPU quota in cores (`CPUQuota=200%` is `2`) |
| `cgroups_v2_memory_working_set_bytes` | gauge | | `memory.current`, `memory.stat` | Working set, `memory.current` minus `inactive_file` (cAdvisor semantics) |
| `cgroups_v2_memory_usage_bytes` | gauge | | `memory.current` | Current memory usage |
| `cgroups_v2_memory_limit_bytes` | gauge | | `memory.max` | Hard limit (`MemoryMax=`) |
| `cgroups_v2_memory_high_bytes` | gauge | | `memory.high` | Throttle limit (`MemoryHigh=`) |
| `cgroups_v2_memory_events_total` | counter | `event` | `memory.events` | Memory events: `low`, `high`, `max`, `oom`, `oom_kill`, ... |
| `cgroups_v2_pids_current` | gauge | | `pids.current` | Processes and threads in the unit |
| `cgroups_v2_pids_limit` | gauge | | `pids.max` | Task limit (`TasksMax=`) |
| `cgroups_v2_io_read_bytes_total` | counter | `device` | `io.stat` | Bytes read per block device |
| `cgroups_v2_io_write_bytes_total` | counter | `device` | `io.stat` | Bytes written per block device |
| `cgroups_v2_io_read_ops_total` | counter | `device` | `io.stat` | Read operations per block device |
| `cgroups_v2_io_write_ops_total` | counter | `device` | `io.stat` | Write operations per block device |

`device` is the block device as `major:minor`.

A unit that is missing a file or a controller only loses the series derived from that file. Limits set to `max` (unlimited) are omitted instead of exported with a made-up value. If cgroups v2 or the user manager is not available, `/metrics` still returns HTTP 200 without unit series.


## 🎯 Current Project Scope

**Unprivileged, user-scoped systemd unit resource metrics (`systemctl --user`), read straight from cgroups v2.**

### 💯 What Can You Do with It?

In Kubernetes, Pod metrics come from [`cAdvisor`](https://github.com/google/cadvisor) (CPU, memory, ...)
and [`kube-state-metrics`](https://github.com/kubernetes/kube-state-metrics) (object state).

This exporter replicates the cAdvisor part for systemd user units:
resource metrics for services running in non-root, user-scoped sandboxes.

### 🔍 Why It Exists in the First Place

An (intentional) gap in the OpenTelemetry `host_metrics` and `systemd` receivers, see
[OpenTelemetry Collector Contrib - Issue #50035](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/45912).

- The `systemd` receiver scrapes unit state (active, failed, ...), not resource usage.
- The `host_metrics` receiver reads `/proc` and fails in an unprivileged, user-scoped agent.

This exporter reads the user's own cgroup files (`cpu.stat`, `memory.current`, ...) instead,
which needs no root privileges.


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
