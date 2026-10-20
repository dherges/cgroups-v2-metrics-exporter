package collector

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"cgroups-v2-metrics-exporter/pkg/discovery"
)

const fakeData = "../../fake-data"

func fakeDiscover() ([]discovery.Unit, error) {
	u := &discovery.UserCgroupInfo{UID: "1002", BaseCpath: fakeData}
	return u.Discover()
}

// gather returns all samples as "name{labels}" -> value.
func gather(t *testing.T, c prometheus.Collector) map[string]float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			key := f.GetName() + "{"
			for i, l := range m.GetLabel() {
				if i > 0 {
					key += ","
				}
				key += l.GetName() + "=" + l.GetValue()
			}
			key += "}"
			switch {
			case m.GetCounter() != nil:
				out[key] = m.GetCounter().GetValue()
			case m.GetGauge() != nil:
				out[key] = m.GetGauge().GetValue()
			}
		}
	}
	return out
}

func checkSamples(t *testing.T, got, want map[string]float64) {
	t.Helper()
	for k, v := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("missing sample %s", k)
		} else if g != v {
			t.Errorf("%s = %v, want %v", k, g, v)
		}
	}
}

func checkAbsent(t *testing.T, got map[string]float64, prefixes ...string) {
	t.Helper()
	for k := range got {
		for _, p := range prefixes {
			if strings.HasPrefix(k, p) {
				t.Errorf("unexpected sample %s", k)
			}
		}
	}
}

const (
	ua = "{systemd_user_uid=1002,systemd_user_unit=fixture-a.service}"
	ub = "{systemd_user_uid=1002,systemd_user_unit=fixture-b.service}"
	uf = "{systemd_user_uid=1002,systemd_user_unit=foo.service}"
)

// fixture-a is the real capture: all limits set, io.stat with four devices.
func TestCollectFixtureA(t *testing.T) {
	got := gather(t, NewCgroupCollector(fakeDiscover))
	checkSamples(t, got, map[string]float64{
		"cgroups_v2_cpu_usage_seconds_total" + ua:     522.428541,
		"cgroups_v2_cpu_periods_total" + ua:           849370,
		"cgroups_v2_cpu_throttled_periods_total" + ua: 2,
		"cgroups_v2_cpu_throttled_seconds_total" + ua: 0.0155,
		"cgroups_v2_cpu_limit_cores" + ua:             2,
		"cgroups_v2_memory_usage_bytes" + ua:          279363584,
		"cgroups_v2_memory_working_set_bytes" + ua:    276365312,
		"cgroups_v2_memory_limit_bytes" + ua:          4294967296,
		"cgroups_v2_memory_high_bytes" + ua:           4080218112,
		"cgroups_v2_pids_current" + ua:                24,
		"cgroups_v2_pids_limit" + ua:                  10000,
		"cgroups_v2_memory_events_total{event=oom_kill,systemd_user_uid=1002,systemd_user_unit=fixture-a.service}": 0,
	})
	// 4 devices in io.stat
	for dev, wbytes := range map[string]float64{"253:7": 1096257536, "8:16": 0} {
		key := "cgroups_v2_io_write_bytes_total{device=" + dev + ",systemd_user_uid=1002,systemd_user_unit=fixture-a.service}"
		if g, ok := got[key]; !ok || g != wbytes {
			t.Errorf("%s = %v (present %v), want %v", key, g, ok, wbytes)
		}
	}
}

// fixture-b has no limits ("max"): limit metrics must be absent, usage metrics present.
func TestCollectUnlimitedUnit(t *testing.T) {
	got := gather(t, NewCgroupCollector(fakeDiscover))
	checkSamples(t, got, map[string]float64{
		"cgroups_v2_cpu_usage_seconds_total" + ub:  1.5,
		"cgroups_v2_memory_usage_bytes" + ub:       104857600,
		"cgroups_v2_memory_working_set_bytes" + ub: 100000000,
		"cgroups_v2_pids_current" + ub:             3,
		"cgroups_v2_memory_events_total{event=oom_kill,systemd_user_uid=1002,systemd_user_unit=fixture-b.service}": 1,
	})
	for _, name := range []string{"cpu_limit_cores", "memory_limit_bytes", "memory_high_bytes", "pids_limit"} {
		if _, ok := got["cgroups_v2_"+name+ub]; ok {
			t.Errorf("cgroups_v2_%s must be absent for an unlimited unit", name)
		}
	}
	// no io.stat for fixture-b
	for k := range got {
		if strings.HasPrefix(k, "cgroups_v2_io_") && strings.Contains(k, "fixture-b") {
			t.Errorf("unexpected io sample %s", k)
		}
	}
}

// foo.service is the oldest fixture: only cpu.stat (no trailing newline), memory.current and cgroup.procs.
func TestCollectMinimalUnit(t *testing.T) {
	got := gather(t, NewCgroupCollector(fakeDiscover))
	checkSamples(t, got, map[string]float64{
		"cgroups_v2_cpu_usage_seconds_total" + uf:  1.201,
		"cgroups_v2_memory_working_set_bytes" + uf: 536870912,
		"cgroups_v2_memory_usage_bytes" + uf:       536870912,
	})
	for k := range got {
		if strings.Contains(k, "foo.service") && (strings.Contains(k, "limit") || strings.Contains(k, "pids") || strings.Contains(k, "io_")) {
			t.Errorf("unexpected sample %s for a unit without those files", k)
		}
	}
}

func TestCollectIsLintClean(t *testing.T) {
	problems, err := testutil.CollectAndLint(NewCgroupCollector(fakeDiscover))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Errorf("lint: %s: %s", p.Metric, p.Text)
	}
}

func TestRegisters(t *testing.T) {
	if err := prometheus.NewRegistry().Register(NewCgroupCollector(fakeDiscover)); err != nil {
		t.Fatal(err)
	}
}

func TestCollectDiscoveryError(t *testing.T) {
	c := NewCgroupCollector(func() ([]discovery.Unit, error) { return nil, errors.New("boom") })
	if n := testutil.CollectAndCount(c); n != 0 {
		t.Errorf("got %d metrics, want 0", n)
	}
}

// writeTree creates files below root: a map of "unit/file" to content.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func unitsAt(root string, names ...string) DiscoverFunc {
	return func() ([]discovery.Unit, error) {
		var units []discovery.Unit
		for _, n := range names {
			units = append(units, discovery.Unit{Name: n, Path: filepath.Join(root, n), UID: "7"})
		}
		return units, nil
	}
}

// A unit without the cpu controller still exports memory and pids; one without the memory
// controller still exports cpu; unreadable and vanished units never break the others.
func TestCollectToleratesMissingControllersAndBrokenFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"full.service/cpu.stat":        "usage_usec 2000000\n",
		"full.service/cpu.max":         "100000 100000\n",
		"full.service/memory.current":  "100\n",
		"full.service/memory.max":      "200\n",
		"full.service/pids.current":    "4\n",
		"nocpu.service/memory.current": "300\n",
		"nocpu.service/pids.current":   "2\n",
		"nomem.service/cpu.stat":       "usage_usec 1000000\n",
		"empty.service/cgroup.procs":   "",
		"junk.service/cpu.stat":        "usage_usec x\n",
		"junk.service/cpu.max":         "garbage",
		"junk.service/memory.current":  "50\n",
		"junk.service/memory.max":      "lots\n",
		"junk.service/pids.current":    "-1\n",
		"junk.service/io.stat":         "not a device\n",
		"junk.service/memory.events":   "oom x\n",
	})

	got := gather(t, NewCgroupCollector(unitsAt(root, "full.service", "nocpu.service", "nomem.service", "empty.service", "junk.service", "gone.service")))

	u := func(name string) string { return "{systemd_user_uid=7,systemd_user_unit=" + name + "}" }
	checkSamples(t, got, map[string]float64{
		"cgroups_v2_cpu_usage_seconds_total" + u("full.service"):  2,
		"cgroups_v2_cpu_limit_cores" + u("full.service"):          1,
		"cgroups_v2_memory_usage_bytes" + u("full.service"):       100,
		"cgroups_v2_memory_limit_bytes" + u("full.service"):       200,
		"cgroups_v2_pids_current" + u("full.service"):             4,
		"cgroups_v2_memory_usage_bytes" + u("nocpu.service"):      300,
		"cgroups_v2_pids_current" + u("nocpu.service"):            2,
		"cgroups_v2_cpu_usage_seconds_total" + u("nomem.service"): 1,
		"cgroups_v2_memory_usage_bytes" + u("junk.service"):       50,
	})
	for k := range got {
		switch {
		case strings.Contains(k, "nocpu.service") && strings.HasPrefix(k, "cgroups_v2_cpu_"):
			t.Errorf("unit without cpu controller exports %s", k)
		case strings.Contains(k, "nomem.service") && strings.HasPrefix(k, "cgroups_v2_memory_"):
			t.Errorf("unit without memory controller exports %s", k)
		case strings.Contains(k, "empty.service"), strings.Contains(k, "gone.service"):
			t.Errorf("unit without files exports %s", k)
		case strings.Contains(k, "junk.service") && (strings.HasPrefix(k, "cgroups_v2_cpu_") ||
			strings.HasPrefix(k, "cgroups_v2_pids_") || strings.HasPrefix(k, "cgroups_v2_io_") ||
			strings.HasPrefix(k, "cgroups_v2_memory_limit") || strings.HasPrefix(k, "cgroups_v2_memory_events")):
			t.Errorf("unit with junk file exports %s", k)
		}
	}
}

// Hosts without cgroups v2 or without a user manager: discovery fails, the scrape succeeds empty.
func TestCollectWithoutCgroupsV2(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	info := &discovery.UserCgroupInfo{UID: "1", BaseCpath: missing}
	c := NewCgroupCollector(info.Discover)

	if n := testutil.CollectAndCount(c); n != 0 {
		t.Errorf("got %d metrics, want 0", n)
	}
	// And it still serves a valid exposition.
	if _, err := prometheus.NewRegistry().Gather(); err != nil {
		t.Fatal(err)
	}
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	if _, err := reg.Gather(); err != nil {
		t.Errorf("Gather() = %v, want no error", err)
	}
}

// Units appearing and disappearing between scrapes are reflected immediately.
func TestCollectFollowsUnitChanges(t *testing.T) {
	root := t.TempDir()
	unit := filepath.Join(root, "app.slice", "late.service")
	info := &discovery.UserCgroupInfo{UID: "1", BaseCpath: root}
	c := NewCgroupCollector(info.Discover)

	if err := os.MkdirAll(filepath.Join(root, "app.slice"), 0o755); err != nil {
		t.Fatal(err)
	}
	if n := testutil.CollectAndCount(c); n != 0 {
		t.Fatalf("before start: %d metrics, want 0", n)
	}

	if err := os.MkdirAll(unit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unit, "cpu.stat"), []byte("usage_usec 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unit, "memory.current"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := testutil.CollectAndCount(c); n != 6 {
		t.Fatalf("after start: %d metrics, want 6", n)
	}

	if err := os.RemoveAll(unit); err != nil {
		t.Fatal(err)
	}
	if n := testutil.CollectAndCount(c); n != 0 {
		t.Errorf("after stop: %d metrics, want 0", n)
	}
}
