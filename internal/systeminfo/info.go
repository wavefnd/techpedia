package systeminfo

import (
	"bufio"
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

type Info struct {
	Location  string        `json:"location"`
	Runtime   RuntimeInfo   `json:"runtime"`
	Host      HostInfo      `json:"host"`
	Container ContainerInfo `json:"container"`
	CPU       CPUInfo       `json:"cpu"`
	Memory    MemoryInfo    `json:"memory"`
	Storage   StorageInfo   `json:"storage"`
}

type RuntimeInfo struct {
	EngineVersion string `json:"engine_version"`
	GoVersion     string `json:"go_version"`
	StartedAt     string `json:"started_at"`
	UptimeSeconds uint64 `json:"uptime_seconds"`
	Goroutines    int    `json:"goroutines"`
}

type HostInfo struct {
	Hostname           string `json:"hostname"`
	Name               string `json:"name"`
	OS                 string `json:"os"`
	Platform           string `json:"platform"`
	PlatformVersion    string `json:"platform_version"`
	KernelVersion      string `json:"kernel_version"`
	Architecture       string `json:"architecture"`
	Virtualization     string `json:"virtualization"`
	VirtualizationRole string `json:"virtualization_role"`
}

type ContainerInfo struct {
	Name               string `json:"name"`
	OS                 string `json:"os"`
	Platform           string `json:"platform"`
	PlatformVersion    string `json:"platform_version"`
	Architecture       string `json:"architecture"`
	Virtualization     string `json:"virtualization"`
	VirtualizationRole string `json:"virtualization_role"`
}

type CPUInfo struct {
	Model         string `json:"model"`
	PhysicalCores int    `json:"physical_cores"`
	LogicalCores  int    `json:"logical_cores"`
}

type MemoryInfo struct {
	Total       uint64  `json:"total"`
	Available   uint64  `json:"available"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
}

type StorageInfo struct {
	Path        string  `json:"path"`
	Total       uint64  `json:"total"`
	Free        uint64  `json:"free"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
}

func Collect(ctx context.Context, dataPath string, location string, engineVersion string, startedAt time.Time) (Info, error) {
	if dataPath == "" {
		dataPath = "."
	}
	hostValue, err := host.InfoWithContext(ctx)
	if err != nil {
		return Info{}, err
	}
	physical, err := cpu.CountsWithContext(ctx, false)
	if err != nil {
		return Info{}, err
	}
	logical, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return Info{}, err
	}
	cpuValues, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return Info{}, err
	}
	memory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return Info{}, err
	}
	storage, err := disk.UsageWithContext(ctx, dataPath)
	if err != nil {
		return Info{}, err
	}
	model := ""
	for _, value := range cpuValues {
		if value.ModelName != "" {
			model = value.ModelName
			break
		}
	}
	uptime := uint64(0)
	if !startedAt.IsZero() {
		uptime = uint64(time.Since(startedAt).Seconds())
	}
	hostInfo := HostInfo{Hostname: hostValue.Hostname, Name: platformDisplayName(hostValue.Platform, hostValue.PlatformVersion), OS: hostValue.OS, Platform: hostValue.Platform, PlatformVersion: hostValue.PlatformVersion, KernelVersion: hostValue.KernelVersion, Architecture: hostValue.KernelArch, Virtualization: hostValue.VirtualizationSystem, VirtualizationRole: hostValue.VirtualizationRole}
	hostInfo = readHostIdentity(hostInfo, "/host/etc/os-release", "/host/etc/hostname")
	containerInfo := ContainerInfo{Name: platformDisplayName(hostValue.Platform, hostValue.PlatformVersion), OS: hostValue.OS, Platform: hostValue.Platform, PlatformVersion: hostValue.PlatformVersion, Architecture: hostValue.KernelArch, Virtualization: hostValue.VirtualizationSystem, VirtualizationRole: hostValue.VirtualizationRole}
	if name, platform, version, err := readOSRelease("/etc/os-release"); err == nil {
		containerInfo.Name = name
		containerInfo.Platform = platform
		containerInfo.PlatformVersion = version
	}
	return Info{
		Location:  location,
		Runtime:   RuntimeInfo{EngineVersion: engineVersion, GoVersion: runtime.Version(), StartedAt: startedAt.Format(time.RFC3339), UptimeSeconds: uptime, Goroutines: runtime.NumGoroutine()},
		Host:      hostInfo,
		Container: containerInfo,
		CPU:       CPUInfo{Model: model, PhysicalCores: physical, LogicalCores: logical},
		Memory:    MemoryInfo{Total: memory.Total, Available: memory.Available, Used: memory.Used, UsedPercent: memory.UsedPercent},
		Storage:   StorageInfo{Path: dataPath, Total: storage.Total, Free: storage.Free, Used: storage.Used, UsedPercent: storage.UsedPercent},
	}, nil
}

func readHostIdentity(current HostInfo, osReleasePath string, hostnamePath string) HostInfo {
	if name, platform, version, err := readOSRelease(osReleasePath); err == nil {
		current.Name = name
		current.Platform = platform
		current.PlatformVersion = version
	}
	if value, err := os.ReadFile(hostnamePath); err == nil && strings.TrimSpace(string(value)) != "" {
		current.Hostname = strings.TrimSpace(string(value))
	}
	return current
}

func readOSRelease(path string) (string, string, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", "", err
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		} else if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}
		values[strings.TrimSpace(key)] = value
	}
	if err := scanner.Err(); err != nil {
		return "", "", "", err
	}
	name := values["PRETTY_NAME"]
	if name == "" {
		name = platformDisplayName(values["NAME"], values["VERSION_ID"])
	}
	return name, values["ID"], values["VERSION_ID"], nil
}

func platformDisplayName(platform string, version string) string {
	return strings.TrimSpace(strings.Join([]string{platform, version}, " "))
}
