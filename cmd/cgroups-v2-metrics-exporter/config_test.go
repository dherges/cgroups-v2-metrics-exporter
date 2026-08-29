package main

import (
	"flag"
	"os"
	"testing"
)

func TestGetConfig(t *testing.T) {
	// Define all of our test scenarios
	tests := []struct {
		name       string
		envVars    map[string]string
		args       []string
		wantListen string
		wantCgroup string
	}{
		{
			name:       "Default values when no env or flags are provided",
			envVars:    map[string]string{},
			args:       []string{},
			wantListen: "0.0.0.0:9100",
			wantCgroup: "",
		},
		{
			name: "Primary environment variables override defaults",
			envVars: map[string]string{
				"METRICS_HOST":             "127.0.0.1",
				"METRICS_PORT":             "8080",
				"METRICS_CGROUP_BASE_PATH": "/sys/fs/cgroup/user.slice",
			},
			args:       []string{},
			wantListen: "127.0.0.1:8080",
			wantCgroup: "/sys/fs/cgroup/user.slice",
		},
		{
			name: "Fallback environment variables work",
			envVars: map[string]string{
				"HOST":             "10.0.0.5",
				"PORT":             "9999",
				"CGROUP_BASE_PATH": "/tmp/cgroup",
			},
			args:       []string{},
			wantListen: "10.0.0.5:9999",
			wantCgroup: "/tmp/cgroup",
		},
		{
			name:    "Command line flags take ultimate precedence over env vars",
			envVars: map[string]string{"METRICS_PORT": "8080"},
			args: []string{
				"-host", "192.168.1.100",
				"-port", "4444",
				"-cgroup-base-path", "/custom/flag/path",
			},
			wantListen: "192.168.1.100:4444",
			wantCgroup: "/custom/flag/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Reset the global flag state so flag.String() doesn't panic on re-registration
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			// 2. Mock the command-line arguments passed to the binary
			os.Args = append([]string{"cmd"}, tt.args...)

			// 3. Mock environment variables safely (automatically cleaned up after this subtest)
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			// 4. Execute the code under test
			got := GetConfig()

			// 5. Assert the outcomes
			if got.ListenAddr != tt.wantListen {
				t.Errorf("GetConfig() ListenAddr = %q, want %q", got.ListenAddr, tt.wantListen)
			}
			if got.CgroupPath != tt.wantCgroup {
				t.Errorf("GetConfig() CgroupPath = %q, want %q", got.CgroupPath, tt.wantCgroup)
			}
		})
	}
}
