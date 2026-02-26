package collector

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// PoolCollector collects metrics from `zpool list` and `zpool status`.
type PoolCollector struct {
	sizeBytes        *prometheus.Desc
	allocatedBytes   *prometheus.Desc
	freeBytes        *prometheus.Desc
	fragmentation    *prometheus.Desc
	capacityPercent  *prometheus.Desc
	deduplicationRatio *prometheus.Desc
	healthy          *prometheus.Desc
	readErrors       *prometheus.Desc
	writeErrors      *prometheus.Desc
	checksumErrors   *prometheus.Desc
	logicalReadOps   *prometheus.Desc
	logicalWriteOps  *prometheus.Desc
	logicalReadBytes *prometheus.Desc
	logicalWriteBytes *prometheus.Desc
	physicalReadOps   *prometheus.Desc
	physicalWriteOps  *prometheus.Desc
	physicalReadBytes *prometheus.Desc
	physicalWriteBytes *prometheus.Desc

	opts   Options
	logger *slog.Logger
}

// NewPoolCollector returns a collector that exposes per-pool ZFS metrics.
func NewPoolCollector(logger *slog.Logger, opts Options) *PoolCollector {
	poolLabels := []string{"pool"}
	poolHealthLabels := []string{"pool", "health"}

	return &PoolCollector{
		opts: opts,
		sizeBytes: prometheus.NewDesc(
			"zfs_pool_size_bytes",
			"Total size of the pool (in bytes).",
			poolLabels, nil,
		),
		allocatedBytes: prometheus.NewDesc(
			"zfs_pool_allocated_bytes",
			"Space allocated in the pool (in bytes).",
			poolLabels, nil,
		),
		freeBytes: prometheus.NewDesc(
			"zfs_pool_free_bytes",
			"Free space in the pool (in bytes).",
			poolLabels, nil,
		),
		fragmentation: prometheus.NewDesc(
			"zfs_pool_fragmentation_percent",
			"Pool fragmentation percentage.",
			poolLabels, nil,
		),
		capacityPercent: prometheus.NewDesc(
			"zfs_pool_capacity_percent",
			"Pool capacity used percentage.",
			poolLabels, nil,
		),
		deduplicationRatio: prometheus.NewDesc(
			"zfs_pool_deduplication_ratio",
			"Pool deduplication ratio (e.g. 1.0 means no dedup savings).",
			poolLabels, nil,
		),
		healthy: prometheus.NewDesc(
			"zfs_pool_healthy",
			"Whether the pool is healthy (1 = ONLINE, 0 = degraded/faulted/etc).",
			poolHealthLabels, nil,
		),
		readErrors: prometheus.NewDesc(
			"zfs_pool_read_errors_total",
			"Total read errors for the pool (top-level vdev).",
			poolLabels, nil,
		),
		writeErrors: prometheus.NewDesc(
			"zfs_pool_write_errors_total",
			"Total write errors for the pool (top-level vdev).",
			poolLabels, nil,
		),
		checksumErrors: prometheus.NewDesc(
			"zfs_pool_checksum_errors_total",
			"Total checksum errors for the pool (top-level vdev).",
			poolLabels, nil,
		),
		logicalReadOps: prometheus.NewDesc(
			"zfs_pool_logical_read_ops_total",
			"Total logical read operations for the pool — all reads requested via the ARC subsystem, including both cache hits and disk reads (cumulative counter from iostats).",
			poolLabels, nil,
		),
		logicalWriteOps: prometheus.NewDesc(
			"zfs_pool_logical_write_ops_total",
			"Total logical write operations for the pool — all writes passing through the ARC/DMU path (cumulative counter from iostats).",
			poolLabels, nil,
		),
		logicalReadBytes: prometheus.NewDesc(
			"zfs_pool_logical_read_bytes_total",
			"Total logical bytes read from the pool — includes ARC cache hits and disk reads (cumulative counter from iostats).",
			poolLabels, nil,
		),
		logicalWriteBytes: prometheus.NewDesc(
			"zfs_pool_logical_write_bytes_total",
			"Total logical bytes written to the pool via the ARC/DMU path (cumulative counter from iostats).",
			poolLabels, nil,
		),
		physicalReadOps: prometheus.NewDesc(
			"zfs_pool_physical_read_ops_total",
			"Total physical read operations completed on pool disks (cumulative counter from /proc/diskstats).",
			poolLabels, nil,
		),
		physicalWriteOps: prometheus.NewDesc(
			"zfs_pool_physical_write_ops_total",
			"Total physical write operations completed on pool disks (cumulative counter from /proc/diskstats).",
			poolLabels, nil,
		),
		physicalReadBytes: prometheus.NewDesc(
			"zfs_pool_physical_read_bytes_total",
			"Total physical bytes read from pool disks (cumulative counter from /proc/diskstats).",
			poolLabels, nil,
		),
		physicalWriteBytes: prometheus.NewDesc(
			"zfs_pool_physical_write_bytes_total",
			"Total physical bytes written to pool disks (cumulative counter from /proc/diskstats).",
			poolLabels, nil,
		),
		logger: logger,
	}
}

// zpoolCommand builds an *exec.Cmd for running a zpool command.
// When running inside a container, it uses chroot into the host rootfs.
func (c *PoolCollector) zpoolCommand(args ...string) *exec.Cmd {
	if c.opts.IsContainer() {
		chrootArgs := append([]string{c.opts.RootfsPath, "zpool"}, args...)
		return exec.Command("chroot", chrootArgs...)
	}
	return exec.Command("zpool", args...)
}

// Describe implements prometheus.Collector.
func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sizeBytes
	ch <- c.allocatedBytes
	ch <- c.freeBytes
	ch <- c.fragmentation
	ch <- c.capacityPercent
	ch <- c.deduplicationRatio
	ch <- c.healthy
	ch <- c.readErrors
	ch <- c.writeErrors
	ch <- c.checksumErrors
	ch <- c.logicalReadOps
	ch <- c.logicalWriteOps
	ch <- c.logicalReadBytes
	ch <- c.logicalWriteBytes
	ch <- c.physicalReadOps
	ch <- c.physicalWriteOps
	ch <- c.physicalReadBytes
	ch <- c.physicalWriteBytes
}

// Collect implements prometheus.Collector.
func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	pools, err := c.getPoolList()
	if err != nil {
		c.logger.Error("failed to collect pool list metrics", "error", err)
		return
	}
	for _, p := range pools {
		ch <- prometheus.MustNewConstMetric(c.sizeBytes, prometheus.GaugeValue, float64(p.size), p.name)
		ch <- prometheus.MustNewConstMetric(c.allocatedBytes, prometheus.GaugeValue, float64(p.allocated), p.name)
		ch <- prometheus.MustNewConstMetric(c.freeBytes, prometheus.GaugeValue, float64(p.free), p.name)
		ch <- prometheus.MustNewConstMetric(c.fragmentation, prometheus.GaugeValue, float64(p.fragmentation), p.name)
		ch <- prometheus.MustNewConstMetric(c.capacityPercent, prometheus.GaugeValue, float64(p.capacity), p.name)
		ch <- prometheus.MustNewConstMetric(c.deduplicationRatio, prometheus.GaugeValue, p.dedupRatio, p.name)

		healthyVal := 0.0
		if p.health == "ONLINE" {
			healthyVal = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.healthy, prometheus.GaugeValue, healthyVal, p.name, p.health)
	}

	// Parse zpool status for error counters.
	errors, err := c.getPoolErrors()
	if err != nil {
		c.logger.Error("failed to collect pool error metrics", "error", err)
		return
	}
	for poolName, e := range errors {
		ch <- prometheus.MustNewConstMetric(c.readErrors, prometheus.GaugeValue, float64(e.read), poolName)
		ch <- prometheus.MustNewConstMetric(c.writeErrors, prometheus.GaugeValue, float64(e.write), poolName)
		ch <- prometheus.MustNewConstMetric(c.checksumErrors, prometheus.GaugeValue, float64(e.checksum), poolName)
	}

	// Collect logical I/O stats from /proc/spl/kstat/zfs/<pool>/iostats.
	// These represent all I/O requests through the ARC subsystem (cache hits + disk reads).
	for _, p := range pools {
		io, err := c.getPoolLogicalIO(p.name)
		if err != nil {
			c.logger.Debug("failed to collect pool logical I/O metrics", "pool", p.name, "error", err)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.logicalReadOps, prometheus.CounterValue, float64(io.readOps), p.name)
		ch <- prometheus.MustNewConstMetric(c.logicalWriteOps, prometheus.CounterValue, float64(io.writeOps), p.name)
		ch <- prometheus.MustNewConstMetric(c.logicalReadBytes, prometheus.CounterValue, float64(io.readBytes), p.name)
		ch <- prometheus.MustNewConstMetric(c.logicalWriteBytes, prometheus.CounterValue, float64(io.writeBytes), p.name)
	}

	// Collect physical disk I/O stats from /proc/diskstats.
	// Maps each disk to its pool via zpool status, then sums per pool.
	physIO, err := c.getPoolPhysicalIO(pools)
	if err != nil {
		c.logger.Debug("failed to collect pool physical I/O metrics", "error", err)
	} else {
		for poolName, pio := range physIO {
			ch <- prometheus.MustNewConstMetric(c.physicalReadOps, prometheus.CounterValue, float64(pio.readOps), poolName)
			ch <- prometheus.MustNewConstMetric(c.physicalWriteOps, prometheus.CounterValue, float64(pio.writeOps), poolName)
			ch <- prometheus.MustNewConstMetric(c.physicalReadBytes, prometheus.CounterValue, float64(pio.readBytes), poolName)
			ch <- prometheus.MustNewConstMetric(c.physicalWriteBytes, prometheus.CounterValue, float64(pio.writeBytes), poolName)
		}
	}
}

type pool struct {
	name          string
	size          int64
	allocated     int64
	free          int64
	fragmentation int64
	capacity      int64
	dedupRatio    float64
	health        string
}

const poolProperties = "name,size,alloc,free,frag,cap,dedup,health"

func (c *PoolCollector) getPoolList() ([]pool, error) {
	cmd := c.zpoolCommand("list", "-Hp", "-o", poolProperties)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		c.logger.Error("zpool list failed", "stderr", stderr.String(), "error", err)
		return nil, err
	}

	var pools []pool
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			c.logger.Warn("unexpected number of fields in zpool list", "line", line, "fields", len(fields))
			continue
		}

		p := pool{
			name:   fields[0],
			health: fields[7],
		}
		p.size = parseInt64(fields[1])
		p.allocated = parseInt64(fields[2])
		p.free = parseInt64(fields[3])
		p.fragmentation = parseInt64(strings.TrimSuffix(fields[4], "%"))
		p.capacity = parseInt64(strings.TrimSuffix(fields[5], "%"))
		p.dedupRatio = parseFloat64(fields[6])

		pools = append(pools, p)
	}

	return pools, nil
}

type poolErrors struct {
	read     int64
	write    int64
	checksum int64
}

// getPoolErrors parses `zpool status -p` output looking for pool-level error counters.
// The line for the pool vdev looks like:
//
//	pool_name  ONLINE  0  0  0
func (c *PoolCollector) getPoolErrors() (map[string]poolErrors, error) {
	cmd := c.zpoolCommand("status", "-p")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		c.logger.Error("zpool status failed", "stderr", stderr.String(), "error", err)
		return nil, err
	}

	result := make(map[string]poolErrors)
	var currentPool string

	for _, line := range strings.Split(stdout.String(), "\n") {
		trimmed := strings.TrimSpace(line)

		// Detect pool name from "pool: <name>" line.
		if strings.HasPrefix(trimmed, "pool:") {
			currentPool = strings.TrimSpace(strings.TrimPrefix(trimmed, "pool:"))
			continue
		}

		// Look for the line that matches the pool name as the first token
		// inside the config section (NAME STATE READ WRITE CKSUM).
		if currentPool == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 5 && fields[0] == currentPool {
			e := poolErrors{
				read:     parseInt64(fields[2]),
				write:    parseInt64(fields[3]),
				checksum: parseInt64(fields[4]),
			}
			result[currentPool] = e
		}
	}

	return result, nil
}

type poolIO struct {
	readOps    int64
	writeOps   int64
	readBytes  int64
	writeBytes int64
}

// getPoolLogicalIO reads cumulative logical I/O counters from /proc/spl/kstat/zfs/<pool>/iostats.
// These counters track all I/O requests that pass through the ZFS ARC subsystem.
// "arc_*" fields represent the normal I/O path (includes both cache hits and disk reads).
// "direct_*" fields represent I/O that bypasses the ARC (rarely used, usually 0).
// This is LOGICAL I/O — what applications requested from the pool, not what hit physical disks.
//
// The file is a key-value format:
//
//	name                            type data
//	arc_read_count                  4    199036228
//	arc_read_bytes                  4    1214200522240
//	arc_write_count                 4    1320100954
//	arc_write_bytes                 4    5710941484686
//	direct_read_count               4    0
//	direct_read_bytes               4    0
//	direct_write_count              4    0
//	direct_write_bytes              4    0
func (c *PoolCollector) getPoolLogicalIO(poolName string) (poolIO, error) {
	ioPath := c.opts.ProcPath + "/spl/kstat/zfs/" + poolName + "/iostats"
	cmd := exec.Command("cat", ioPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return poolIO{}, err
	}

	stats := make(map[string]int64)
	for _, line := range strings.Split(stdout.String(), "\n") {
		fields := strings.Fields(line)
		// Format: name  type  value
		if len(fields) != 3 {
			continue
		}
		// Skip header line
		if fields[0] == "name" {
			continue
		}
		stats[fields[0]] = parseInt64(fields[2])
	}

	return poolIO{
		readOps:    stats["arc_read_count"] + stats["direct_read_count"],
		writeOps:   stats["arc_write_count"] + stats["direct_write_count"],
		readBytes:  stats["arc_read_bytes"] + stats["direct_read_bytes"],
		writeBytes: stats["arc_write_bytes"] + stats["direct_write_bytes"],
	}, nil
}

// getPoolDisks parses `zpool status` to extract the disk device names for each pool.
// It identifies disk lines in the config section by excluding known ZFS keywords (mirror,
// raidz, spare, log, cache, etc.) and the pool name itself. Each remaining name is resolved
// to a short Linux block device name (e.g. "sda") via /dev/disk/by-id/ symlink resolution.
// Returns a map of pool name → list of short device names.
func (c *PoolCollector) getPoolDisks() (map[string][]string, error) {
	cmd := c.zpoolCommand("status")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("zpool status failed: %w (%s)", err, stderr.String())
	}

	// ZFS keywords that appear in the config section but are NOT disk names.
	zpoolKeywords := map[string]bool{
		"NAME": true, "STATE": true, "READ": true, "WRITE": true, "CKSUM": true,
		"mirror": true, "raidz1": true, "raidz2": true, "raidz3": true,
		"stripe": true, "cache": true, "log": true, "spare": true, "special": true,
		"dedup": true, "replacing": true, "logs": true, "spares": true,
	}

	result := make(map[string][]string)
	var currentPool string
	inConfig := false

	for _, line := range strings.Split(stdout.String(), "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "pool:") {
			currentPool = strings.TrimSpace(strings.TrimPrefix(trimmed, "pool:"))
			inConfig = false
			continue
		}

		if strings.HasPrefix(trimmed, "config:") {
			inConfig = true
			continue
		}

		if strings.HasPrefix(trimmed, "errors:") {
			inConfig = false
			continue
		}

		if !inConfig || currentPool == "" {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		devName := fields[0]

		// Skip the pool name itself (appears as the top-level vdev)
		if devName == currentPool {
			continue
		}

		// Skip known ZFS keywords (vdev types, header columns)
		// Also skip mirror-N, raidz1-N variants
		baseName := devName
		if idx := strings.LastIndex(baseName, "-"); idx != -1 {
			prefix := baseName[:idx]
			if zpoolKeywords[prefix] {
				continue
			}
		}
		if zpoolKeywords[devName] {
			continue
		}

		// Try to resolve this name to a short block device name
		resolved := c.resolveDeviceName(devName)
		if resolved != "" {
			result[currentPool] = append(result[currentPool], resolved)
			c.logger.Debug("mapped disk to pool", "disk", resolved, "pool", currentPool, "original", devName)
		} else {
			c.logger.Debug("could not resolve disk name", "name", devName, "pool", currentPool)
		}
	}

	return result, nil
}

// resolveDeviceName tries to resolve a device identifier (from zpool status) to a short
// device name like "sda". It handles:
//   - Short names already (sda, nvme0n1) → returned as-is
//   - /dev/disk/by-id/ style names → resolved via symlink
//   - Names with partition suffixes (-partN) → stripped to get the whole device
//   - TrueNAS GUID/serial style names → tries all /dev/disk/ subdirectories
func (c *PoolCollector) resolveDeviceName(devName string) string {
	// Remove partition suffix if present (e.g. "-part2", "-part3", "p1")
	baseDev := devName
	for i := 1; i <= 9; i++ {
		baseDev = strings.TrimSuffix(baseDev, fmt.Sprintf("-part%d", i))
		baseDev = strings.TrimSuffix(baseDev, fmt.Sprintf("p%d", i))
	}

	// If it's already a short name like sda, nvme0n1, etc.
	if isShortDevName(baseDev) {
		return baseDev
	}

	// Determine the dev root path
	devRoot := "/dev"
	if c.opts.IsContainer() {
		devRoot = filepath.Join(c.opts.RootfsPath, "dev")
	}

	// Try resolving via multiple /dev/disk/ subdirectories
	// TrueNAS uses GPT partition UUIDs in zpool status, so by-partuuid is critical
	for _, subDir := range []string{"by-partuuid", "by-id", "by-vdev", "by-path"} {
		// Try the full name first, then just the base (without partition)
		for _, candidate := range []string{devName, baseDev} {
			linkPath := filepath.Join(devRoot, "disk", subDir, candidate)
			target, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}

			// target is relative like ../../sda or ../../sda2
			shortName := filepath.Base(target)

			// Strip partition number from resolved name (e.g. sda2 → sda)
			shortName = stripPartition(shortName)

			if isShortDevName(shortName) {
				return shortName
			}
		}
	}

	// Last resort: scan /dev/disk/by-id/ for any symlink containing our device name
	byIDDir := filepath.Join(devRoot, "disk", "by-id")
	entries, err := os.ReadDir(byIDDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !strings.Contains(entry.Name(), baseDev) {
			continue
		}
		linkPath := filepath.Join(byIDDir, entry.Name())
		target, err := os.Readlink(linkPath)
		if err != nil {
			continue
		}
		shortName := stripPartition(filepath.Base(target))
		if isShortDevName(shortName) {
			return shortName
		}
	}

	return ""
}

// stripPartition removes trailing partition numbers from device names.
// e.g., "sda2" → "sda", "nvme0n1p3" → "nvme0n1", "sdb" → "sdb"
func stripPartition(name string) string {
	// Handle NVMe partitions: nvme0n1p1 → nvme0n1
	if strings.HasPrefix(name, "nvme") {
		if idx := strings.LastIndex(name, "p"); idx > 0 {
			suffix := name[idx+1:]
			allDigits := len(suffix) > 0
			for _, c := range suffix {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return name[:idx]
			}
		}
		return name
	}

	// Handle sd/vd/xvd/da partitions: sda2 → sda, vdb1 → vdb
	// Remove trailing digits
	trimmed := strings.TrimRight(name, "0123456789")
	if isShortDevName(trimmed) && trimmed != name {
		return trimmed
	}
	return name
}

// isShortDevName checks if a name looks like a Linux block device (sda, nvme0n1, etc.)
func isShortDevName(name string) bool {
	return strings.HasPrefix(name, "sd") ||
		strings.HasPrefix(name, "nvme") ||
		strings.HasPrefix(name, "da") ||
		strings.HasPrefix(name, "vd") ||
		strings.HasPrefix(name, "xvd")
}

// diskStats holds physical I/O counters from /proc/diskstats for a single device.
type diskStats struct {
	readsCompleted  int64
	readBytes       int64
	writesCompleted int64
	writeBytes      int64
}

// parseDiskStats reads /proc/diskstats and returns a map of device name → stats.
// /proc/diskstats format (kernel 4.18+):
//
//	major minor name reads_completed reads_merged read_sectors read_ms
//	writes_completed writes_merged write_sectors write_ms ...
//
// Sectors are always 512 bytes.
func (c *PoolCollector) parseDiskStats() (map[string]diskStats, error) {
	diskStatsPath := filepath.Join(c.opts.ProcPath, "diskstats")

	file, err := os.Open(diskStatsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", diskStatsPath, err)
	}
	defer file.Close()

	result := make(map[string]diskStats)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// /proc/diskstats has at least 14 fields per line
		if len(fields) < 14 {
			continue
		}

		name := fields[2]
		ds := diskStats{
			readsCompleted:  parseInt64(fields[3]),
			readBytes:       parseInt64(fields[5]) * 512, // sectors → bytes
			writesCompleted: parseInt64(fields[7]),
			writeBytes:      parseInt64(fields[9]) * 512, // sectors → bytes
		}
		result[name] = ds
	}

	return result, scanner.Err()
}

// getPoolPhysicalIO maps disks to pools and sums physical I/O from /proc/diskstats.
// Returns a map of pool name → aggregated physical I/O counters.
func (c *PoolCollector) getPoolPhysicalIO(pools []pool) (map[string]poolIO, error) {
	poolDisks, err := c.getPoolDisks()
	if err != nil {
		return nil, err
	}

	allDiskStats, err := c.parseDiskStats()
	if err != nil {
		return nil, err
	}

	result := make(map[string]poolIO)
	for _, p := range pools {
		disks, ok := poolDisks[p.name]
		if !ok {
			continue
		}

		var pio poolIO
		for _, diskName := range disks {
			ds, ok := allDiskStats[diskName]
			if !ok {
				c.logger.Debug("disk not found in /proc/diskstats", "disk", diskName, "pool", p.name)
				continue
			}
			pio.readOps += ds.readsCompleted
			pio.writeOps += ds.writesCompleted
			pio.readBytes += ds.readBytes
			pio.writeBytes += ds.writeBytes
		}

		result[p.name] = pio
	}

	return result, nil
}
