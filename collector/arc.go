package collector

import (
	"bytes"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// ARCCollector collects ZFS ARC (Adaptive Replacement Cache) statistics
// from /proc/spl/kstat/zfs/arcstats (Linux).
type ARCCollector struct {
	arcSize      *prometheus.Desc
	arcMaxSize   *prometheus.Desc
	arcMinSize   *prometheus.Desc
	arcHits      *prometheus.Desc
	arcMisses    *prometheus.Desc
	arcHitRatio  *prometheus.Desc
	l2Size       *prometheus.Desc
	l2Hits       *prometheus.Desc
	l2Misses     *prometheus.Desc

	opts   Options
	logger *slog.Logger
}

// NewARCCollector returns a collector that exposes ZFS ARC cache statistics.
func NewARCCollector(logger *slog.Logger, opts Options) *ARCCollector {
	return &ARCCollector{
		opts: opts,
		arcSize: prometheus.NewDesc(
			"zfs_arc_size_bytes",
			"Current size of the ARC in bytes.",
			nil, nil,
		),
		arcMaxSize: prometheus.NewDesc(
			"zfs_arc_max_size_bytes",
			"Maximum target size of the ARC in bytes.",
			nil, nil,
		),
		arcMinSize: prometheus.NewDesc(
			"zfs_arc_min_size_bytes",
			"Minimum target size of the ARC in bytes.",
			nil, nil,
		),
		arcHits: prometheus.NewDesc(
			"zfs_arc_hits_total",
			"Total number of ARC hits.",
			nil, nil,
		),
		arcMisses: prometheus.NewDesc(
			"zfs_arc_misses_total",
			"Total number of ARC misses.",
			nil, nil,
		),
		arcHitRatio: prometheus.NewDesc(
			"zfs_arc_hit_ratio",
			"ARC hit ratio (hits / (hits + misses)).",
			nil, nil,
		),
		l2Size: prometheus.NewDesc(
			"zfs_arc_l2_size_bytes",
			"Current size of the L2ARC in bytes.",
			nil, nil,
		),
		l2Hits: prometheus.NewDesc(
			"zfs_arc_l2_hits_total",
			"Total L2ARC hits.",
			nil, nil,
		),
		l2Misses: prometheus.NewDesc(
			"zfs_arc_l2_misses_total",
			"Total L2ARC misses.",
			nil, nil,
		),
		logger: logger,
	}
}

// Describe implements prometheus.Collector.
func (c *ARCCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.arcSize
	ch <- c.arcMaxSize
	ch <- c.arcMinSize
	ch <- c.arcHits
	ch <- c.arcMisses
	ch <- c.arcHitRatio
	ch <- c.l2Size
	ch <- c.l2Hits
	ch <- c.l2Misses
}

// Collect implements prometheus.Collector.
func (c *ARCCollector) Collect(ch chan<- prometheus.Metric) {
	stats, err := c.readARCStats()
	if err != nil {
		c.logger.Error("failed to collect ARC metrics", "error", err)
		return
	}

	size := lookupStat(stats, "size")
	maxSize := lookupStat(stats, "c_max")
	minSize := lookupStat(stats, "c_min")
	hits := lookupStat(stats, "hits")
	misses := lookupStat(stats, "misses")

	ch <- prometheus.MustNewConstMetric(c.arcSize, prometheus.GaugeValue, float64(size))
	ch <- prometheus.MustNewConstMetric(c.arcMaxSize, prometheus.GaugeValue, float64(maxSize))
	ch <- prometheus.MustNewConstMetric(c.arcMinSize, prometheus.GaugeValue, float64(minSize))
	ch <- prometheus.MustNewConstMetric(c.arcHits, prometheus.CounterValue, float64(hits))
	ch <- prometheus.MustNewConstMetric(c.arcMisses, prometheus.CounterValue, float64(misses))

	total := hits + misses
	ratio := 0.0
	if total > 0 {
		ratio = float64(hits) / float64(total)
	}
	ch <- prometheus.MustNewConstMetric(c.arcHitRatio, prometheus.GaugeValue, ratio)

	l2Size := lookupStat(stats, "l2_size")
	l2Hits := lookupStat(stats, "l2_hits")
	l2Misses := lookupStat(stats, "l2_misses")
	ch <- prometheus.MustNewConstMetric(c.l2Size, prometheus.GaugeValue, float64(l2Size))
	ch <- prometheus.MustNewConstMetric(c.l2Hits, prometheus.CounterValue, float64(l2Hits))
	ch <- prometheus.MustNewConstMetric(c.l2Misses, prometheus.CounterValue, float64(l2Misses))
}

func (c *ARCCollector) readARCStats() (map[string]int64, error) {
	// Read arcstats from configured procfs path.
	arcstatsPath := c.opts.ProcPath + "/spl/kstat/zfs/arcstats"
	cmd := exec.Command("cat", arcstatsPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		c.logger.Debug("could not read /proc/spl/kstat/zfs/arcstats", "error", err)
		return nil, err
	}

	stats := make(map[string]int64)
	for _, line := range strings.Split(stdout.String(), "\n") {
		fields := strings.Fields(line)
		// Format: name  type  value
		if len(fields) != 3 {
			continue
		}
		name := fields[0]
		value := parseInt64(fields[2])
		stats[name] = value
	}
	return stats, nil
}

func lookupStat(stats map[string]int64, key string) int64 {
	if v, ok := stats[key]; ok {
		return v
	}
	return 0
}
