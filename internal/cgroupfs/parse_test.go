package cgroupfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fixtures are the repo's fake cgroup tree (fixture-a is a real capture).
var (
	fixtureA = filepath.Join("..", "..", "fake-data", "app.slice", "fixture-a.service")
	fixtureB = filepath.Join("..", "..", "fake-data", "app.slice", "fixture-b.service")
	fooUnit  = filepath.Join("..", "..", "fake-data", "app.slice", "foo.service")
)

func TestParseKeyed(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    map[string]uint64
		wantErr bool
	}{
		{name: "simple", in: "a 1\nb 2\n", want: map[string]uint64{"a": 1, "b": 2}},
		{name: "no trailing newline", in: "a 1\nb 2", want: map[string]uint64{"a": 1, "b": 2}},
		{name: "blank lines", in: "\na 1\n\n", want: map[string]uint64{"a": 1}},
		{name: "dotted key", in: "core_sched.force_idle_usec 7\n", want: map[string]uint64{"core_sched.force_idle_usec": 7}},
		{name: "empty", in: "", want: map[string]uint64{}},
		{name: "no value", in: "lonely\n", wantErr: true},
		{name: "non-numeric", in: "a x\n", wantErr: true},
		{name: "negative", in: "a -1\n", wantErr: true},
		{name: "max in keyed file", in: "a max\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKeyed(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseUint(t *testing.T) {
	if v, err := ParseUint("279363584\n"); err != nil || v != 279363584 {
		t.Errorf("ParseUint = %d, %v", v, err)
	}
	for _, in := range []string{"", "\n", "max", "-5", "1.5"} {
		if _, err := ParseUint(in); err == nil {
			t.Errorf("ParseUint(%q): expected error", in)
		}
	}
}

func TestParseLimit(t *testing.T) {
	tests := []struct {
		in        string
		value     uint64
		unlimited bool
		wantErr   bool
	}{
		{in: "4294967296\n", value: 4294967296},
		{in: "max\n", unlimited: true},
		{in: "max", unlimited: true},
		{in: "0\n", value: 0},
		{in: "junk", wantErr: true},
	}
	for _, tt := range tests {
		v, unlimited, err := ParseLimit(tt.in)
		if (err != nil) != tt.wantErr || v != tt.value || unlimited != tt.unlimited {
			t.Errorf("ParseLimit(%q) = %d, %v, %v; want %d, %v, err=%v", tt.in, v, unlimited, err, tt.value, tt.unlimited, tt.wantErr)
		}
	}
}

func TestParseCPUMax(t *testing.T) {
	tests := []struct {
		in       string
		want     CPUMax
		wantCPUs float64
		wantErr  bool
	}{
		{in: "200000 100000\n", want: CPUMax{QuotaUsec: 200000, PeriodUsec: 100000}, wantCPUs: 2},
		{in: "50000 100000", want: CPUMax{QuotaUsec: 50000, PeriodUsec: 100000}, wantCPUs: 0.5},
		{in: "max 100000\n", want: CPUMax{Unlimited: true, PeriodUsec: 100000}},
		{in: "max\n", want: CPUMax{Unlimited: true, PeriodUsec: 100000}},
		{in: "", wantErr: true},
		{in: "1 2 3", wantErr: true},
		{in: "x 100000", wantErr: true},
		{in: "200000 y", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseCPUMax(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseCPUMax(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		if got != tt.want || got.CPUs() != tt.wantCPUs {
			t.Errorf("ParseCPUMax(%q) = %+v (%v CPUs), want %+v (%v CPUs)", tt.in, got, got.CPUs(), tt.want, tt.wantCPUs)
		}
	}
}

func TestParseCPUStat(t *testing.T) {
	t.Run("missing usage_usec", func(t *testing.T) {
		if _, err := ParseCPUStat("user_usec 1\n"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("optional keys absent", func(t *testing.T) {
		got, err := ParseCPUStat("usage_usec 5\n")
		if err != nil || got != (CPUStat{UsageUsec: 5}) {
			t.Errorf("got %+v, %v", got, err)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		if _, err := ParseCPUStat("usage_usec x\n"); err == nil {
			t.Error("expected error")
		}
	})
}

func TestWorkingSet(t *testing.T) {
	tests := []struct {
		name    string
		current uint64
		stat    map[string]uint64
		want    uint64
	}{
		{name: "subtracts inactive_file", current: 1000, stat: map[string]uint64{"inactive_file": 300}, want: 700},
		{name: "no inactive_file key", current: 1000, stat: map[string]uint64{"anon": 5}, want: 1000},
		{name: "nil stat", current: 1000, want: 1000},
		{name: "inactive larger than current", current: 100, stat: map[string]uint64{"inactive_file": 300}, want: 0},
		{name: "equal", current: 100, stat: map[string]uint64{"inactive_file": 100}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorkingSet(tt.current, tt.stat); got != tt.want {
				t.Errorf("WorkingSet() = %d, want %d", got, tt.want)
			}
		})
	}
}

// Real capture of auth-ldap, stored as fixture-a.
func TestReadFixtureA(t *testing.T) {
	cpu, err := ReadCPUStat(fixtureA)
	if err != nil {
		t.Fatal(err)
	}
	want := CPUStat{
		UsageUsec:     522428541,
		UserUsec:      335361084,
		SystemUsec:    187067457,
		NrPeriods:     849370,
		NrThrottled:   2,
		ThrottledUsec: 15500,
	}
	if cpu != want {
		t.Errorf("ReadCPUStat() = %+v, want %+v", cpu, want)
	}

	max, err := ReadCPUMax(fixtureA)
	if err != nil || max.CPUs() != 2 {
		t.Errorf("ReadCPUMax() = %+v, %v; want 2 CPUs", max, err)
	}

	mem, err := ReadMemory(fixtureA)
	if err != nil {
		t.Fatal(err)
	}
	// 279363584 - inactive_file 2998272
	if want := (Memory{Current: 279363584, WorkingSet: 276365312}); mem != want {
		t.Errorf("ReadMemory() = %+v, want %+v", mem, want)
	}

	data, err := os.ReadFile(filepath.Join(fixtureA, "memory.stat"))
	if err != nil {
		t.Fatal(err)
	}
	stat, err := ParseKeyed(string(data))
	if err != nil {
		t.Fatalf("memory.stat: %v", err)
	}
	if stat["anon"] != 274571264 || stat["thp_swpout_fallback"] != 0 {
		t.Errorf("unexpected memory.stat values: anon=%d", stat["anon"])
	}

	for name, want := range map[string]uint64{"memory.peak": 324227072, "memory.min": 268435456, "cpu.weight": 10} {
		if got, err := ReadUint(fixtureA, name); err != nil || got != want {
			t.Errorf("ReadUint(%s) = %d, %v; want %d", name, got, err, want)
		}
	}
	data, _ = os.ReadFile(filepath.Join(fixtureA, "memory.max"))
	if v, unlimited, err := ParseLimit(string(data)); err != nil || unlimited || v != 4294967296 {
		t.Errorf("memory.max = %d, %v, %v", v, unlimited, err)
	}
}

func TestReadFixtureB(t *testing.T) {
	max, err := ReadCPUMax(fixtureB)
	if err != nil || !max.Unlimited {
		t.Errorf("ReadCPUMax() = %+v, %v; want unlimited", max, err)
	}

	cpu, err := ReadCPUStat(fixtureB)
	if err != nil || cpu.UsageUsec != 1500000 || cpu.NrThrottled != 0 {
		t.Errorf("ReadCPUStat() = %+v, %v", cpu, err)
	}

	mem, err := ReadMemory(fixtureB)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Memory{Current: 104857600, WorkingSet: 104857600 - 4857600}); mem != want {
		t.Errorf("ReadMemory() = %+v, want %+v", mem, want)
	}

	data, _ := os.ReadFile(filepath.Join(fixtureB, "memory.max"))
	if _, unlimited, err := ParseLimit(string(data)); err != nil || !unlimited {
		t.Errorf("memory.max unlimited = %v, %v", unlimited, err)
	}
}

// foo.service predates the fixtures: its cpu.stat has no trailing newline and few keys, and it has
// no memory.stat, so the working set falls back to memory.current.
func TestReadFoo(t *testing.T) {
	cpu, err := ReadCPUStat(fooUnit)
	if err != nil || cpu.UsageUsec != 1201000 || cpu.ThrottledUsec != 35000 {
		t.Errorf("ReadCPUStat() = %+v, %v", cpu, err)
	}

	mem, err := ReadMemory(fooUnit)
	if err != nil || mem != (Memory{Current: 536870912, WorkingSet: 536870912}) {
		t.Errorf("ReadMemory() = %+v, %v", mem, err)
	}

	if _, err := ReadCPUMax(fooUnit); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadCPUMax() on missing file: error = %v, want not-exist", err)
	}
}

func TestReadErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := ReadMemory(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadMemory() without files: %v, want not-exist", err)
	}

	write("memory.current", "junk\n")
	if _, err := ReadMemory(dir); err == nil {
		t.Error("ReadMemory() with junk memory.current: expected error")
	}

	write("memory.current", "100\n")
	write("memory.stat", "inactive_file nope\n")
	if _, err := ReadMemory(dir); err == nil {
		t.Error("ReadMemory() with junk memory.stat: expected error")
	}

	write("cpu.stat", "user_usec 1\n")
	if _, err := ReadCPUStat(dir); err == nil {
		t.Error("ReadCPUStat() without usage_usec: expected error")
	}

	write("cpu.max", "garbage")
	if _, err := ReadCPUMax(dir); err == nil {
		t.Error("ReadCPUMax() with garbage: expected error")
	}

	write("pids.current", "x")
	if _, err := ReadUint(dir, "pids.current"); err == nil {
		t.Error("ReadUint() with junk: expected error")
	}
}

func TestParseIOStat(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []IOStat
		wantErr bool
	}{
		{name: "empty", in: "", want: nil},
		{name: "only newline", in: "\n", want: nil},
		{
			name: "two devices, unknown keys ignored",
			in:   "8:16 rbytes=10 wbytes=20 rios=3 wios=4 dbytes=0 dios=0 future=9\n253:0 rbytes=1 wbytes=2 rios=3 wios=4",
			want: []IOStat{
				{Device: "8:16", ReadBytes: 10, WriteBytes: 20, ReadOps: 3, WriteOps: 4},
				{Device: "253:0", ReadBytes: 1, WriteBytes: 2, ReadOps: 3, WriteOps: 4},
			},
		},
		{name: "device without counters", in: "8:0\n", want: []IOStat{{Device: "8:0"}}},
		{name: "no device", in: "rbytes=1\n", wantErr: true},
		{name: "field without equals", in: "8:0 rbytes\n", wantErr: true},
		{name: "non-numeric", in: "8:0 rbytes=x\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIOStat(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReadM4Fixtures(t *testing.T) {
	// fixture-a: real capture.
	io, err := ReadIOStat(fixtureA)
	if err != nil || len(io) != 4 {
		t.Fatalf("ReadIOStat() = %+v, %v", io, err)
	}
	if want := (IOStat{Device: "253:7", WriteBytes: 1096257536, WriteOps: 166554}); io[3] != want {
		t.Errorf("io[3] = %+v, want %+v", io[3], want)
	}

	ev, err := ReadKeyed(fixtureA, "memory.events")
	if err != nil || len(ev) != 6 || ev["oom_kill"] != 0 {
		t.Errorf("ReadKeyed(memory.events) = %v, %v", ev, err)
	}

	if v, unlimited, err := ReadLimit(fixtureA, "pids.max"); err != nil || unlimited || v != 10000 {
		t.Errorf("pids.max = %d, %v, %v", v, unlimited, err)
	}
	if v, unlimited, err := ReadLimit(fixtureA, "memory.high"); err != nil || unlimited || v != 4080218112 {
		t.Errorf("memory.high = %d, %v, %v", v, unlimited, err)
	}

	// fixture-b: synthetic. Unlimited pids, nonzero memory events, no io.stat.
	if _, unlimited, err := ReadLimit(fixtureB, "pids.max"); err != nil || !unlimited {
		t.Errorf("fixture-b pids.max unlimited = %v, %v", unlimited, err)
	}
	if ev, err := ReadKeyed(fixtureB, "memory.events"); err != nil || ev["oom_kill"] != 1 || ev["high"] != 5 {
		t.Errorf("fixture-b memory.events = %v, %v", ev, err)
	}
	if _, err := ReadIOStat(fixtureB); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("fixture-b ReadIOStat() error = %v, want not-exist", err)
	}
}

func TestReadKeyedAndLimitErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "memory.events"), []byte("oom x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pids.max"), []byte("many\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "io.stat"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadKeyed(dir, "memory.events"); err == nil {
		t.Error("ReadKeyed() with junk: expected error")
	}
	if _, _, err := ReadLimit(dir, "pids.max"); err == nil {
		t.Error("ReadLimit() with junk: expected error")
	}
	if _, err := ReadIOStat(dir); err == nil {
		t.Error("ReadIOStat() with junk: expected error")
	}
	if _, err := ReadKeyed(dir, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadKeyed() on missing file: %v, want not-exist", err)
	}
	if _, _, err := ReadLimit(dir, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadLimit() on missing file: %v, want not-exist", err)
	}
}
