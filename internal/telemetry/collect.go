package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

// SessionCounter abstracts the daemon's session registry.
type SessionCounter interface {
	Count() SessionStat
}

type noopCounter struct{}

func (noopCounter) Count() SessionStat { return SessionStat{} }

// Sampler holds state for computing inter-tick CPU percent.
type Sampler struct {
	prevCPUTimes cpu.TimesStat
	prevTime     time.Time
	counter      SessionCounter
}

// NewSampler returns a Sampler with a no-op session counter.
func NewSampler() *Sampler {
	return &Sampler{counter: noopCounter{}}
}

// SetSessionCounter overrides the default no-op counter.
func (s *Sampler) SetSessionCounter(c SessionCounter) {
	if c == nil {
		c = noopCounter{}
	}
	s.counter = c
}

// Collect gathers one telemetry sample.
func (s *Sampler) Collect(ctx context.Context) (Sample, error) {
	var sample Sample
	sample.TS = time.Now().UnixNano()

	// Host info
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return sample, fmt.Errorf("host info: %w", err)
	}
	sample.Host = info.Hostname
	sample.UptimeSec = info.Uptime

	// Load average
	avg, err := load.AvgWithContext(ctx)
	if err != nil {
		return sample, fmt.Errorf("load avg: %w", err)
	}
	sample.Load = [3]float64{avg.Load1, avg.Load5, avg.Load15}

	// CPU percent (inter-tick delta)
	cpuPct, err := s.cpuPercent(ctx)
	if err != nil {
		return sample, fmt.Errorf("cpu percent: %w", err)
	}
	sample.CPUPct = cpuPct

	// Memory
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return sample, fmt.Errorf("virtual memory: %w", err)
	}
	sample.Mem = MemStat{Total: vm.Total, Used: vm.Used, Free: vm.Free}

	swap, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return sample, fmt.Errorf("swap memory: %w", err)
	}
	sample.Swap = MemStat{Total: swap.Total, Used: swap.Used, Free: swap.Free}

	// Disks (filtered)
	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return sample, fmt.Errorf("disk partitions: %w", err)
	}
	for _, p := range partitions {
		if !isRealMount(p.Mountpoint, p.Fstype) {
			continue
		}
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue // skip partitions that fail to stat
		}
		sample.Disks = append(sample.Disks, DiskStat{
			Mount: p.Mountpoint,
			Total: usage.Total,
			Used:  usage.Used,
			Free:  usage.Free,
		})
	}

	// Network
	netIO, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return sample, fmt.Errorf("net io counters: %w", err)
	}
	for _, n := range netIO {
		sample.Net = append(sample.Net, NetStat{
			Name:    n.Name,
			RxBytes: n.BytesRecv,
			TxBytes: n.BytesSent,
		})
	}

	// Sensors — errors degrade to empty, never fail the sample
	sample.Temps = processSensors(sensors.TemperaturesWithContext(ctx))

	// Sessions from the registry
	sample.Sessions = s.counter.Count()

	return sample, nil
}

// cpuPercent computes CPU usage since the previous call.
func (s *Sampler) cpuPercent(ctx context.Context) (float64, error) {
	times, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return 0, err
	}
	if len(times) == 0 {
		return 0, fmt.Errorf("no cpu times returned")
	}
	current := times[0]
	now := time.Now()

	if s.prevTime.IsZero() {
		s.prevCPUTimes = current
		s.prevTime = now
		return 0, nil
	}

	totalDiff := totalCPUTime(current) - totalCPUTime(s.prevCPUTimes)
	idleDiff := current.Idle - s.prevCPUTimes.Idle
	if totalDiff <= 0 {
		return 0, nil
	}

	percent := 100 * (1 - float64(idleDiff)/float64(totalDiff))
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	s.prevCPUTimes = current
	s.prevTime = now
	return percent, nil
}

func totalCPUTime(t cpu.TimesStat) float64 {
	return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice
}

// isRealMount filters out virtual, ephemeral, and system mountpoints.
func isRealMount(mountpoint, fstype string) bool {
	switch fstype {
	case "tmpfs", "devtmpfs", "overlay", "squashfs":
		return false
	}
	for _, prefix := range []string{"/snap", "/System/Volumes/", "/proc"} {
		if len(mountpoint) >= len(prefix) && mountpoint[:len(prefix)] == prefix {
			return false
		}
	}
	return true
}

// processSensors converts sensor results, treating any error as "no temps".
func processSensors(temps []sensors.TemperatureStat, err error) []TempStat {
	if err != nil {
		return []TempStat{}
	}
	out := make([]TempStat, 0, len(temps))
	for _, t := range temps {
		out = append(out, TempStat{Name: t.SensorKey, TempC: t.Temperature})
	}
	return out
}
