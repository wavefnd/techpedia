package systeminfo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCollectReturnsRuntimeAndStorageInformation(t *testing.T) {
	startedAt := time.Now().Add(-2 * time.Minute).UTC()
	info, err := Collect(context.Background(), t.TempDir(), "Seoul", "1.5", startedAt)
	if err != nil {
		t.Fatalf("collect server information: %v", err)
	}
	if info.Location != "Seoul" || info.Runtime.EngineVersion != "1.5" || info.Runtime.GoVersion != runtime.Version() {
		t.Fatalf("unexpected runtime information: %#v", info)
	}
	if info.Runtime.UptimeSeconds < 100 || info.Storage.Total == 0 || info.Host.Architecture == "" || info.Container.Name == "" {
		t.Fatalf("missing host resource information: %#v", info)
	}
}

func TestReadHostIdentityUsesMountedHostMetadata(t *testing.T) {
	dir := t.TempDir()
	osRelease := filepath.Join(dir, "os-release")
	hostname := filepath.Join(dir, "hostname")
	if err := os.WriteFile(osRelease, []byte("NAME=Ubuntu\nID=ubuntu\nVERSION_ID=24.04\nPRETTY_NAME=\"Ubuntu 24.04.3 LTS\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostname, []byte("techpedia-vps\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readHostIdentity(HostInfo{Hostname: "container", Platform: "alpine", PlatformVersion: "3.24.1"}, osRelease, hostname)
	if got.Hostname != "techpedia-vps" || got.Name != "Ubuntu 24.04.3 LTS" || got.Platform != "ubuntu" || got.PlatformVersion != "24.04" {
		t.Fatalf("unexpected host identity: %#v", got)
	}
}
