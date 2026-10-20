package discovery

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestDefaultBase(t *testing.T) {
	got := defaultBase("/sys/fs/cgroup", "1002")
	want := "/sys/fs/cgroup/user.slice/user-1002.slice/user@1002.service"
	if got != want {
		t.Errorf("defaultBase() = %q, want %q", got, want)
	}
}

func TestDiscoverUserContext(t *testing.T) {
	mount := t.TempDir()
	mkTree(t, mount, "user.slice/user-1002.slice/user@1002.service/app.slice/a.service/")
	custom := t.TempDir()

	tests := []struct {
		name    string
		custom  string
		uid     int
		want    string
		wantErr bool
	}{
		{
			name: "derived from uid",
			uid:  1002,
			want: filepath.Join(mount, "user.slice/user-1002.slice/user@1002.service"),
		},
		{
			name:   "custom path wins",
			custom: custom,
			uid:    1002,
			want:   custom,
		},
		{
			name:    "no cgroup for this uid",
			uid:     1003,
			wantErr: true,
		},
		{
			name:    "custom path missing",
			custom:  filepath.Join(custom, "nope"),
			uid:     1002,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := discoverUserContext(tt.custom, tt.uid, mount)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if info.BaseCpath != tt.want {
				t.Errorf("BaseCpath = %q, want %q", info.BaseCpath, tt.want)
			}
			if want := strconv.Itoa(tt.uid); info.UID != want {
				t.Errorf("UID = %q, want %q", info.UID, want)
			}
		})
	}
}

func TestUnescapeUnitName(t *testing.T) {
	tests := map[string]string{
		"fixture-a.service":     "fixture-a.service",
		`my\x2dapp.service`:     "my-app.service",
		`a\x2db\x2dc.service`:   "a-b-c.service",
		`weird\xzz.service`:     `weird\xzz.service`,
		`slash\x2fname.service`: "slash/name.service",
		"":                      "",
	}
	for in, want := range tests {
		if got := UnescapeUnitName(in); got != want {
			t.Errorf("UnescapeUnitName(%q) = %q, want %q", in, got, want)
		}
	}
}

// mkTree creates the given directories (trailing "/") and files below root.
func mkTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if p[len(p)-1] == '/' {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func unitNames(units []Unit) []string {
	names := []string{}
	for _, u := range units {
		names = append(names, u.Name)
	}
	return names
}

func TestDiscover(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{
			name: "services in app.slice, sorted, others skipped",
			paths: []string{
				"app.slice/b.service/",
				"app.slice/a.service/",
				"app.slice/session-1.scope/",
				"app.slice/README.txt",
				"app.slice/file.service", // a file, not a cgroup
			},
			want: []string{"a.service", "b.service"},
		},
		{
			name:  "escaped names are decoded",
			paths: []string{`app.slice/my\x2dapp.service/`},
			want:  []string{"my-app.service"},
		},
		{
			name:  "no app.slice falls back to base",
			paths: []string{"c.service/", "init.scope/"},
			want:  []string{"c.service"},
		},
		{
			name:  "empty app.slice",
			paths: []string{"app.slice/"},
			want:  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			mkTree(t, root, tt.paths...)
			u := &UserCgroupInfo{UID: "1002", BaseCpath: root}

			units, err := u.Discover()
			if err != nil {
				t.Fatalf("Discover() error = %v", err)
			}
			if got := unitNames(units); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Discover() names = %v, want %v", got, tt.want)
			}
			for _, unit := range units {
				if unit.UID != "1002" || filepath.Dir(unit.Path) == "" {
					t.Errorf("unexpected unit %+v", unit)
				}
			}
		})
	}
}

func TestDiscoverPicksUpChangesBetweenCalls(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, "app.slice/a.service/")
	u := &UserCgroupInfo{UID: "1002", BaseCpath: root}

	first, err := u.Discover()
	if err != nil || len(first) != 1 {
		t.Fatalf("first Discover() = %v, %v", first, err)
	}

	mkTree(t, root, "app.slice/b.service/")
	if err := os.Remove(filepath.Join(root, "app.slice/a.service")); err != nil {
		t.Fatal(err)
	}

	second, err := u.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := unitNames(second), []string{"b.service"}; !reflect.DeepEqual(got, want) {
		t.Errorf("second Discover() names = %v, want %v", got, want)
	}
}

func TestDiscoverMissingBase(t *testing.T) {
	u := &UserCgroupInfo{UID: "1002", BaseCpath: filepath.Join(t.TempDir(), "nope")}
	if _, err := u.Discover(); err == nil {
		t.Error("Discover() on missing base: expected error")
	}
}

func TestMissingControllers(t *testing.T) {
	tests := []struct {
		name        string
		controllers string // empty: no file
		want        []string
		wantErr     bool
	}{
		{name: "all delegated", controllers: "cpu memory pids\n", want: nil},
		{name: "memory missing", controllers: "cpu pids\n", want: []string{"memory"}},
		{name: "none delegated", controllers: "\n", want: []string{"cpu", "memory"}},
		{name: "file missing", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.controllers != "" {
				if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte(tt.controllers), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			u := &UserCgroupInfo{BaseCpath: root}
			got, err := u.MissingControllers()
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MissingControllers() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The repo fixtures stand in for user@<uid>.service.
func TestDiscoverFakeData(t *testing.T) {
	u := &UserCgroupInfo{UID: "1002", BaseCpath: filepath.Join("..", "..", "fake-data")}

	units, err := u.Discover()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fixture-a.service", "fixture-b.service", "foo.service"}
	if got := unitNames(units); !reflect.DeepEqual(got, want) {
		t.Errorf("Discover() names = %v, want %v", got, want)
	}

	missing, err := u.MissingControllers()
	if err != nil || len(missing) != 0 {
		t.Errorf("MissingControllers() = %v, %v; want none", missing, err)
	}
}
