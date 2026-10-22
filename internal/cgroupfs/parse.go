// Package cgroupfs parses the cgroup v2 interface files of a single cgroup directory.
//
// The Parse* functions take the file content and do no I/O. The Read* functions are thin wrappers
// that read a file below a cgroup directory.
package cgroupfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseKeyed parses a flat keyed file ("key value" per line) as used by cpu.stat, memory.stat and
// memory.events. Keys may contain dots (core_sched.force_idle_usec), so each line is split at its
// last space. Blank lines are ignored; a missing trailing newline is fine.
func ParseKeyed(content string) (map[string]uint64, error) {
	values := make(map[string]uint64)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		if i <= 0 {
			return nil, fmt.Errorf("malformed line %q: want \"key value\"", line)
		}
		v, err := strconv.ParseUint(line[i+1:], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("malformed value in line %q: %w", line, err)
		}
		values[strings.TrimSpace(line[:i])] = v
	}
	return values, nil
}

// ParseUint parses a single-value file such as memory.current or pids.current.
func ParseUint(content string) (uint64, error) {
	s := strings.TrimSpace(content)
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("malformed value %q: %w", s, err)
	}
	return v, nil
}

// ParseLimit parses a single-value file that may hold the string "max" (memory.max, memory.high,
// pids.max). unlimited is true for "max"; value is then 0.
func ParseLimit(content string) (value uint64, unlimited bool, err error) {
	if strings.TrimSpace(content) == "max" {
		return 0, true, nil
	}
	value, err = ParseUint(content)
	return value, false, err
}

// CPUMax is the content of cpu.max: a quota of CPU time per period, both in microseconds.
type CPUMax struct {
	// Unlimited is true when the quota is "max".
	Unlimited bool
	QuotaUsec uint64
	// PeriodUsec is always set; the kernel defaults it to 100000.
	PeriodUsec uint64
}

// CPUs returns the quota in CPUs (200000 per 100000 is 2.0), or 0 if unlimited.
func (m CPUMax) CPUs() float64 {
	if m.Unlimited || m.PeriodUsec == 0 {
		return 0
	}
	return float64(m.QuotaUsec) / float64(m.PeriodUsec)
}

// ParseCPUMax parses cpu.max: "<quota|max> <period>", or just "<quota|max>".
func ParseCPUMax(content string) (CPUMax, error) {
	fields := strings.Fields(content)
	if len(fields) == 0 || len(fields) > 2 {
		return CPUMax{}, fmt.Errorf("malformed cpu.max %q: want \"<quota|max> [<period>]\"", strings.TrimSpace(content))
	}

	m := CPUMax{PeriodUsec: 100000}
	if fields[0] == "max" {
		m.Unlimited = true
	} else {
		q, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return CPUMax{}, fmt.Errorf("malformed cpu.max quota %q: %w", fields[0], err)
		}
		m.QuotaUsec = q
	}
	if len(fields) == 2 {
		p, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return CPUMax{}, fmt.Errorf("malformed cpu.max period %q: %w", fields[1], err)
		}
		m.PeriodUsec = p
	}
	return m, nil
}

// CPUStat holds the fields of cpu.stat used for metrics. Counters are monotonic.
type CPUStat struct {
	UsageUsec     uint64
	NrPeriods     uint64
	NrThrottled   uint64
	ThrottledUsec uint64
}

// ParseCPUStat extracts CPUStat from the content of cpu.stat. usage_usec is required; the other
// keys are optional (the throttling keys are absent without the cpu controller's bandwidth
// support) and stay 0 when missing.
func ParseCPUStat(content string) (CPUStat, error) {
	kv, err := ParseKeyed(content)
	if err != nil {
		return CPUStat{}, err
	}
	usage, ok := kv["usage_usec"]
	if !ok {
		return CPUStat{}, fmt.Errorf("cpu.stat has no usage_usec")
	}
	return CPUStat{
		UsageUsec:     usage,
		NrPeriods:     kv["nr_periods"],
		NrThrottled:   kv["nr_throttled"],
		ThrottledUsec: kv["throttled_usec"],
	}, nil
}

// WorkingSet returns the cAdvisor working set: memory.current minus inactive_file (page cache that
// is cheap to reclaim), never below 0. inactive_file comes from memory.stat; if the key is absent
// the working set equals current.
func WorkingSet(current uint64, memoryStat map[string]uint64) uint64 {
	inactive := memoryStat["inactive_file"]
	if inactive >= current {
		return 0
	}
	return current - inactive
}

// Memory is the memory usage of a cgroup in bytes.
type Memory struct {
	Current    uint64
	WorkingSet uint64
}

func readFile(dir, name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ReadCPUStat reads <dir>/cpu.stat.
func ReadCPUStat(dir string) (CPUStat, error) {
	content, err := readFile(dir, "cpu.stat")
	if err != nil {
		return CPUStat{}, err
	}
	s, err := ParseCPUStat(content)
	if err != nil {
		return CPUStat{}, fmt.Errorf("%s/cpu.stat: %w", dir, err)
	}
	return s, nil
}

// ReadCPUMax reads <dir>/cpu.max.
func ReadCPUMax(dir string) (CPUMax, error) {
	content, err := readFile(dir, "cpu.max")
	if err != nil {
		return CPUMax{}, err
	}
	m, err := ParseCPUMax(content)
	if err != nil {
		return CPUMax{}, fmt.Errorf("%s/cpu.max: %w", dir, err)
	}
	return m, nil
}

// ReadMemory reads <dir>/memory.current and, for the working set, <dir>/memory.stat.
// A missing memory.stat is not an error: the working set then equals memory.current.
func ReadMemory(dir string) (Memory, error) {
	content, err := readFile(dir, "memory.current")
	if err != nil {
		return Memory{}, err
	}
	current, err := ParseUint(content)
	if err != nil {
		return Memory{}, fmt.Errorf("%s/memory.current: %w", dir, err)
	}

	var stat map[string]uint64
	if content, err := readFile(dir, "memory.stat"); err == nil {
		if stat, err = ParseKeyed(content); err != nil {
			return Memory{}, fmt.Errorf("%s/memory.stat: %w", dir, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Memory{}, err
	}

	return Memory{Current: current, WorkingSet: WorkingSet(current, stat)}, nil
}

// ReadUint reads a single-value file like <dir>/pids.current.
func ReadUint(dir, name string) (uint64, error) {
	content, err := readFile(dir, name)
	if err != nil {
		return 0, err
	}
	v, err := ParseUint(content)
	if err != nil {
		return 0, fmt.Errorf("%s/%s: %w", dir, name, err)
	}
	return v, nil
}

// ReadKeyed reads and parses the flat keyed file <dir>/<name> (e.g. memory.events).
func ReadKeyed(dir, name string) (map[string]uint64, error) {
	content, err := readFile(dir, name)
	if err != nil {
		return nil, err
	}
	kv, err := ParseKeyed(content)
	if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", dir, name, err)
	}
	return kv, nil
}

// ReadLimit reads a single-value file that may hold "max" (memory.max, memory.high, pids.max).
func ReadLimit(dir, name string) (value uint64, unlimited bool, err error) {
	content, err := readFile(dir, name)
	if err != nil {
		return 0, false, err
	}
	value, unlimited, err = ParseLimit(content)
	if err != nil {
		return 0, false, fmt.Errorf("%s/%s: %w", dir, name, err)
	}
	return value, unlimited, nil
}

// IOStat is one line of io.stat: the counters of a block device ("major:minor").
type IOStat struct {
	Device     string
	ReadBytes  uint64
	WriteBytes uint64
	ReadOps    uint64
	WriteOps   uint64
}

// ParseIOStat parses io.stat: one line per device, "<major>:<minor> rbytes=N wbytes=N rios=N ...".
// Unknown keys (dbytes, dios, future ones) are ignored. An empty file yields no devices.
func ParseIOStat(content string) ([]IOStat, error) {
	var devices []IOStat
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if !strings.Contains(fields[0], ":") {
			return nil, fmt.Errorf("malformed io.stat line %q: want \"<major>:<minor> key=value ...\"", line)
		}
		dev := IOStat{Device: fields[0]}
		for _, kv := range fields[1:] {
			key, val, ok := strings.Cut(kv, "=")
			if !ok {
				return nil, fmt.Errorf("malformed io.stat field %q in line %q", kv, line)
			}
			n, err := strconv.ParseUint(val, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("malformed io.stat value %q in line %q: %w", kv, line, err)
			}
			switch key {
			case "rbytes":
				dev.ReadBytes = n
			case "wbytes":
				dev.WriteBytes = n
			case "rios":
				dev.ReadOps = n
			case "wios":
				dev.WriteOps = n
			}
		}
		devices = append(devices, dev)
	}
	return devices, nil
}

// ReadIOStat reads <dir>/io.stat.
func ReadIOStat(dir string) ([]IOStat, error) {
	content, err := readFile(dir, "io.stat")
	if err != nil {
		return nil, err
	}
	devices, err := ParseIOStat(content)
	if err != nil {
		return nil, fmt.Errorf("%s/io.stat: %w", dir, err)
	}
	return devices, nil
}
