package collector

import (
	"errors"
	"io/fs"
	"log"

	"github.com/prometheus/client_golang/prometheus"

	"cgroups-v2-metrics-exporter/internal/cgroupfs"
	"cgroups-v2-metrics-exporter/internal/discovery"
)

// DiscoverFunc lists the currently running units; see discovery.UserCgroupInfo.Discover.
type DiscoverFunc func() ([]discovery.Unit, error)

var (
	unitLabels   = []string{"systemd_user_uid", "systemd_user_unit"}
	eventLabels  = append(append([]string{}, unitLabels...), "event")
	deviceLabels = append(append([]string{}, unitLabels...), "device")
)

// CgroupCollector exposes cgroup v2 metrics of systemd user units. It is stateless: units are
// discovered and their files are read on every scrape.
//
// It tolerates incomplete hosts and units. Every file is read independently, so a missing file (a
// controller that is not delegated, a unit that stopped after discovery) or an unreadable one only
// drops the metrics derived from that file. Limits set to "max" are omitted instead of being
// exported with a made-up value. If discovery fails (no cgroup v2, no user manager) the scrape
// simply returns no unit metrics.
type CgroupCollector struct {
	discover DiscoverFunc

	cpuUsage        *prometheus.Desc
	cpuPeriods      *prometheus.Desc
	cpuThrottled    *prometheus.Desc
	cpuThrottledSec *prometheus.Desc
	cpuLimitCores   *prometheus.Desc

	memoryWorkingSet *prometheus.Desc
	memoryUsage      *prometheus.Desc
	memoryLimit      *prometheus.Desc
	memoryHigh       *prometheus.Desc
	memoryEvents     *prometheus.Desc

	pidsCurrent *prometheus.Desc
	pidsLimit   *prometheus.Desc

	ioReadBytes  *prometheus.Desc
	ioWriteBytes *prometheus.Desc
	ioReadOps    *prometheus.Desc
	ioWriteOps   *prometheus.Desc
}

func NewCgroupCollector(discover DiscoverFunc) *CgroupCollector {
	desc := func(name, help string, labels []string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, labels, nil)
	}
	return &CgroupCollector{
		discover: discover,

		cpuUsage:        desc("cgroups_v2_cpu_usage_seconds_total", "Cumulative CPU time consumed by the unit (cpu.stat usage_usec).", unitLabels),
		cpuPeriods:      desc("cgroups_v2_cpu_periods_total", "Enforcement periods elapsed for the unit (cpu.stat nr_periods).", unitLabels),
		cpuThrottled:    desc("cgroups_v2_cpu_throttled_periods_total", "Periods in which the unit was throttled (cpu.stat nr_throttled).", unitLabels),
		cpuThrottledSec: desc("cgroups_v2_cpu_throttled_seconds_total", "Total time the unit was throttled (cpu.stat throttled_usec).", unitLabels),
		cpuLimitCores:   desc("cgroups_v2_cpu_limit_cores", "CPU quota of the unit in cores (cpu.max quota / period). Absent if unlimited.", unitLabels),

		memoryWorkingSet: desc("cgroups_v2_memory_working_set_bytes", "Memory working set of the unit: memory.current minus inactive_file from memory.stat.", unitLabels),
		memoryUsage:      desc("cgroups_v2_memory_usage_bytes", "Current memory usage of the unit (memory.current).", unitLabels),
		memoryLimit:      desc("cgroups_v2_memory_limit_bytes", "Hard memory limit of the unit (memory.max). Absent if unlimited.", unitLabels),
		memoryHigh:       desc("cgroups_v2_memory_high_bytes", "Memory throttle limit of the unit (memory.high). Absent if unlimited.", unitLabels),
		memoryEvents:     desc("cgroups_v2_memory_events_total", "Memory events of the unit (memory.events), by event: low, high, max, oom, oom_kill, ...", eventLabels),

		pidsCurrent: desc("cgroups_v2_pids_current", "Number of processes and threads in the unit (pids.current).", unitLabels),
		pidsLimit:   desc("cgroups_v2_pids_limit", "Task limit of the unit (pids.max). Absent if unlimited.", unitLabels),

		ioReadBytes:  desc("cgroups_v2_io_read_bytes_total", "Bytes read by the unit per block device (io.stat rbytes).", deviceLabels),
		ioWriteBytes: desc("cgroups_v2_io_write_bytes_total", "Bytes written by the unit per block device (io.stat wbytes).", deviceLabels),
		ioReadOps:    desc("cgroups_v2_io_read_ops_total", "Read operations of the unit per block device (io.stat rios).", deviceLabels),
		ioWriteOps:   desc("cgroups_v2_io_write_ops_total", "Write operations of the unit per block device (io.stat wios).", deviceLabels),
	}
}

func (c *CgroupCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		c.cpuUsage, c.cpuPeriods, c.cpuThrottled, c.cpuThrottledSec, c.cpuLimitCores,
		c.memoryWorkingSet, c.memoryUsage, c.memoryLimit, c.memoryHigh, c.memoryEvents,
		c.pidsCurrent, c.pidsLimit,
		c.ioReadBytes, c.ioWriteBytes, c.ioReadOps, c.ioWriteOps,
	} {
		ch <- d
	}
}

func (c *CgroupCollector) Collect(ch chan<- prometheus.Metric) {
	units, err := c.discover()
	if err != nil {
		log.Printf("Unit discovery failed: %v", err)
		return
	}
	for _, u := range units {
		c.collectCPU(ch, u)
		c.collectMemory(ch, u)
		c.collectPids(ch, u)
		c.collectIO(ch, u)
	}
}

func (c *CgroupCollector) collectCPU(ch chan<- prometheus.Metric, u discovery.Unit) {
	if s, err := cgroupfs.ReadCPUStat(u.Path); err != nil {
		logReadError(u, "cpu.stat", err)
	} else {
		ch <- prometheus.MustNewConstMetric(c.cpuUsage, prometheus.CounterValue, float64(s.UsageUsec)/1e6, u.UID, u.Name)
		ch <- prometheus.MustNewConstMetric(c.cpuPeriods, prometheus.CounterValue, float64(s.NrPeriods), u.UID, u.Name)
		ch <- prometheus.MustNewConstMetric(c.cpuThrottled, prometheus.CounterValue, float64(s.NrThrottled), u.UID, u.Name)
		ch <- prometheus.MustNewConstMetric(c.cpuThrottledSec, prometheus.CounterValue, float64(s.ThrottledUsec)/1e6, u.UID, u.Name)
	}

	if m, err := cgroupfs.ReadCPUMax(u.Path); err != nil {
		logReadError(u, "cpu.max", err)
	} else if !m.Unlimited {
		ch <- prometheus.MustNewConstMetric(c.cpuLimitCores, prometheus.GaugeValue, m.CPUs(), u.UID, u.Name)
	}
}

func (c *CgroupCollector) collectMemory(ch chan<- prometheus.Metric, u discovery.Unit) {
	if m, err := cgroupfs.ReadMemory(u.Path); err != nil {
		logReadError(u, "memory.current", err)
	} else {
		ch <- prometheus.MustNewConstMetric(c.memoryUsage, prometheus.GaugeValue, float64(m.Current), u.UID, u.Name)
		ch <- prometheus.MustNewConstMetric(c.memoryWorkingSet, prometheus.GaugeValue, float64(m.WorkingSet), u.UID, u.Name)
	}

	for _, l := range []struct {
		file string
		desc *prometheus.Desc
	}{{"memory.max", c.memoryLimit}, {"memory.high", c.memoryHigh}} {
		v, unlimited, err := cgroupfs.ReadLimit(u.Path, l.file)
		if err != nil {
			logReadError(u, l.file, err)
		} else if !unlimited {
			ch <- prometheus.MustNewConstMetric(l.desc, prometheus.GaugeValue, float64(v), u.UID, u.Name)
		}
	}

	if events, err := cgroupfs.ReadKeyed(u.Path, "memory.events"); err != nil {
		logReadError(u, "memory.events", err)
	} else {
		for event, v := range events {
			ch <- prometheus.MustNewConstMetric(c.memoryEvents, prometheus.CounterValue, float64(v), u.UID, u.Name, event)
		}
	}
}

func (c *CgroupCollector) collectPids(ch chan<- prometheus.Metric, u discovery.Unit) {
	if v, err := cgroupfs.ReadUint(u.Path, "pids.current"); err != nil {
		logReadError(u, "pids.current", err)
	} else {
		ch <- prometheus.MustNewConstMetric(c.pidsCurrent, prometheus.GaugeValue, float64(v), u.UID, u.Name)
	}

	if v, unlimited, err := cgroupfs.ReadLimit(u.Path, "pids.max"); err != nil {
		logReadError(u, "pids.max", err)
	} else if !unlimited {
		ch <- prometheus.MustNewConstMetric(c.pidsLimit, prometheus.GaugeValue, float64(v), u.UID, u.Name)
	}
}

func (c *CgroupCollector) collectIO(ch chan<- prometheus.Metric, u discovery.Unit) {
	devices, err := cgroupfs.ReadIOStat(u.Path)
	if err != nil {
		logReadError(u, "io.stat", err)
		return
	}
	for _, d := range devices {
		ch <- prometheus.MustNewConstMetric(c.ioReadBytes, prometheus.CounterValue, float64(d.ReadBytes), u.UID, u.Name, d.Device)
		ch <- prometheus.MustNewConstMetric(c.ioWriteBytes, prometheus.CounterValue, float64(d.WriteBytes), u.UID, u.Name, d.Device)
		ch <- prometheus.MustNewConstMetric(c.ioReadOps, prometheus.CounterValue, float64(d.ReadOps), u.UID, u.Name, d.Device)
		ch <- prometheus.MustNewConstMetric(c.ioWriteOps, prometheus.CounterValue, float64(d.WriteOps), u.UID, u.Name, d.Device)
	}
}

// logReadError skips the metrics of one file. A missing file is expected when the unit stopped
// after discovery, or when a controller is not delegated, so it is not logged.
func logReadError(u discovery.Unit, file string, err error) {
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	log.Printf("Skipping %s of %s: %v", file, u.Name, err)
}
