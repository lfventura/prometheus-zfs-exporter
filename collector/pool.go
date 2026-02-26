package collector

import (
	"bytes"
	"log/slog"
	"os/exec"
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
	ioReadOps        *prometheus.Desc
	ioWriteOps       *prometheus.Desc
	ioReadBytes      *prometheus.Desc
	ioWriteBytes     *prometheus.Desc

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
		ioReadOps: prometheus.NewDesc(
			"zfs_pool_read_ops_total",
			"Total read operations for the pool (cumulative counter).",
			poolLabels, nil,
		),
		ioWriteOps: prometheus.NewDesc(
			"zfs_pool_write_ops_total",
			"Total write operations for the pool (cumulative counter).",
			poolLabels, nil,
		),
		ioReadBytes: prometheus.NewDesc(
			"zfs_pool_read_bytes_total",
			"Total bytes read from the pool (cumulative counter).",
			poolLabels, nil,
		),
		ioWriteBytes: prometheus.NewDesc(
			"zfs_pool_write_bytes_total",
			"Total bytes written to the pool (cumulative counter).",
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
	ch <- c.ioReadOps
	ch <- c.ioWriteOps
	ch <- c.ioReadBytes
	ch <- c.ioWriteBytes
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

	// Collect I/O stats from /proc/spl/kstat/zfs/<pool>/io.
	for _, p := range pools {
		io, err := c.getPoolIO(p.name)
		if err != nil {
			c.logger.Debug("failed to collect pool I/O metrics", "pool", p.name, "error", err)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.ioReadOps, prometheus.CounterValue, float64(io.readOps), p.name)
		ch <- prometheus.MustNewConstMetric(c.ioWriteOps, prometheus.CounterValue, float64(io.writeOps), p.name)
		ch <- prometheus.MustNewConstMetric(c.ioReadBytes, prometheus.CounterValue, float64(io.readBytes), p.name)
		ch <- prometheus.MustNewConstMetric(c.ioWriteBytes, prometheus.CounterValue, float64(io.writeBytes), p.name)
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
	readOps  int64
	writeOps int64
	readBytes  int64
	writeBytes int64
}

// getPoolIO reads cumulative I/O counters from /proc/spl/kstat/zfs/<pool>/io.
// The file format (Linux) has a header line and a data line with space-separated fields:
//   nread  nwritten  reads  writes  wtime  wlentime  wupdate  rtime  rlentime  rupdate  wcnt  rcnt
func (c *PoolCollector) getPoolIO(poolName string) (poolIO, error) {
	ioPath := c.opts.ProcPath + "/spl/kstat/zfs/" + poolName + "/io"
	cmd := exec.Command("cat", ioPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return poolIO{}, err
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	// We need at least 3 lines: header comment, column names, data.
	// Sometimes it's just 2 lines: header and data.
	var dataLine string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "name") || strings.HasPrefix(line, "type") {
			continue
		}
		// First non-header line is the data line.
		dataLine = line
		break
	}

	if dataLine == "" {
		return poolIO{}, nil
	}

	fields := strings.Fields(dataLine)
	if len(fields) < 4 {
		c.logger.Warn("unexpected io kstat format", "pool", poolName, "line", dataLine)
		return poolIO{}, nil
	}

	// Fields: nread nwritten reads writes ...
	return poolIO{
		readBytes:  parseInt64(fields[0]),
		writeBytes: parseInt64(fields[1]),
		readOps:    parseInt64(fields[2]),
		writeOps:   parseInt64(fields[3]),
	}, nil
}
