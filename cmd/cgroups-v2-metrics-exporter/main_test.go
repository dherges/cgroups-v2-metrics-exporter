package main

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"cgroups-v2-metrics-exporter/internal/collector"
	"cgroups-v2-metrics-exporter/internal/discovery"
)

func TestNewServer(t *testing.T) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector.NewCgroupCollector(func() ([]discovery.Unit, error) { return nil, nil }))

	srv := newServer("127.0.0.1:0", registry)

	if srv.Addr != "127.0.0.1:0" {
		t.Errorf("Addr = %q, want 127.0.0.1:0", srv.Addr)
	}
	// A client that never finishes its request headers must not hold a connection open forever.
	if srv.ReadHeaderTimeout <= 0 {
		t.Errorf("ReadHeaderTimeout = %v, want a positive timeout", srv.ReadHeaderTimeout)
	}

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/", http.StatusOK, "Metrics Endpoint"},
		{"/metrics", http.StatusOK, "cgroups_v2_discovery_success 1"},
		{"/nope", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body does not contain %q:\n%s", tt.wantBody, rec.Body.String())
			}
		})
	}
}

func TestLogStartup(t *testing.T) {
	// withControllers resolves to a user manager whose cgroup.controllers has the given content.
	withControllers := func(controllers string) resolveFunc {
		return func() (*discovery.UserCgroupInfo, error) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "cgroup.controllers"), []byte(controllers), 0o644); err != nil {
				t.Fatal(err)
			}
			return &discovery.UserCgroupInfo{UID: "7", BaseCpath: dir}, nil
		}
	}

	tests := []struct {
		name    string
		resolve resolveFunc
		want    []string
		notWant []string
	}{
		{
			name:    "no cgroups v2 hierarchy",
			resolve: func() (*discovery.UserCgroupInfo, error) { return nil, errors.New("boom") },
			want:    []string{"Warning: no usable cgroups v2 user hierarchy", "boom"},
			notWant: []string{"Discovered environment"},
		},
		{
			name:    "all controllers delegated",
			resolve: withControllers("cpuset cpu io memory pids\n"),
			want:    []string{"Discovered environment: UID=7"},
			notWant: []string{"Warning"},
		},
		{
			name:    "io not delegated",
			resolve: withControllers("cpu memory pids\n"),
			want:    []string{"Discovered environment: UID=7", "controllers [io] are not delegated"},
		},
		{
			name: "controllers cannot be read",
			resolve: func() (*discovery.UserCgroupInfo, error) {
				return &discovery.UserCgroupInfo{UID: "7", BaseCpath: t.TempDir()}, nil
			},
			want: []string{"Warning: cannot check delegated cgroup controllers"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)
			t.Cleanup(func() { log.SetOutput(os.Stderr) })

			logStartup(tt.resolve)

			for _, w := range tt.want {
				if !strings.Contains(buf.String(), w) {
					t.Errorf("log does not contain %q:\n%s", w, buf.String())
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(buf.String(), nw) {
					t.Errorf("log unexpectedly contains %q:\n%s", nw, buf.String())
				}
			}
		})
	}
}
