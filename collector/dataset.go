package collector

import (
	"bytes"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// DatasetCollector collects metrics from `zfs list` for every dataset/filesystem.
type DatasetCollector struct {
	usedBytes              *prometheus.Desc
	availableBytes         *prometheus.Desc
	referencedBytes        *prometheus.Desc
	logicalUsed            *prometheus.Desc
	logicalRef             *prometheus.Desc
	written                *prometheus.Desc
	snapCount              *prometheus.Desc
	compressRatio          *prometheus.Desc
	quota                  *prometheus.Desc
	reservation            *prometheus.Desc
	recordSize             *prometheus.Desc
	usedByChildren         *prometheus.Desc
	usedByDataset          *prometheus.Desc
	usedBySnapshots        *prometheus.Desc
	usedByRefreservation   *prometheus.Desc

	opts   Options
	logger *slog.Logger
}

// NewDatasetCollector returns a collector that exposes per-dataset ZFS metrics.
func NewDatasetCollector(logger *slog.Logger, opts Options) *DatasetCollector {
	labels := []string{"name", "mountpoint", "type", "pool"}

	return &DatasetCollector{
		opts: opts,
		usedBytes: prometheus.NewDesc(
			"zfs_dataset_used_bytes",
			"Space consumed by the dataset and all its descendants (in bytes).",
			labels, nil,
		),
		availableBytes: prometheus.NewDesc(
			"zfs_dataset_available_bytes",
			"Space available to the dataset and its children (in bytes).",
			labels, nil,
		),
		referencedBytes: prometheus.NewDesc(
			"zfs_dataset_referenced_bytes",
			"Data referenced by this dataset, which would be freed if the dataset were destroyed (in bytes).",
			labels, nil,
		),
		logicalUsed: prometheus.NewDesc(
			"zfs_dataset_logical_used_bytes",
			"Logical space used by this dataset before compression/dedup (in bytes).",
			labels, nil,
		),
		logicalRef: prometheus.NewDesc(
			"zfs_dataset_logical_referenced_bytes",
			"Logical space referenced by this dataset before compression/dedup (in bytes).",
			labels, nil,
		),
		written: prometheus.NewDesc(
			"zfs_dataset_written_bytes",
			"Bytes written to this dataset since the last snapshot (in bytes).",
			labels, nil,
		),
		snapCount: prometheus.NewDesc(
			"zfs_dataset_snapshot_count",
			"Number of snapshots for this dataset.",
			labels, nil,
		),
		compressRatio: prometheus.NewDesc(
			"zfs_dataset_compression_ratio",
			"Compression ratio achieved for this dataset (e.g. 1.5 means 1.5x).",
			labels, nil,
		),
		quota: prometheus.NewDesc(
			"zfs_dataset_quota_bytes",
			"Quota set on this dataset (0 means none).",
			labels, nil,
		),
		reservation: prometheus.NewDesc(
			"zfs_dataset_reservation_bytes",
			"Reservation set on this dataset (0 means none).",
			labels, nil,
		),
		recordSize: prometheus.NewDesc(
			"zfs_dataset_record_size_bytes",
			"Record size of the dataset.",
			labels, nil,
		),
		usedByChildren: prometheus.NewDesc(
			"zfs_dataset_used_by_children_bytes",
			"Space used by children of this dataset (in bytes).",
			labels, nil,
		),
		usedByDataset: prometheus.NewDesc(
			"zfs_dataset_used_by_dataset_bytes",
			"Space used by this dataset itself, excluding children and snapshots (in bytes).",
			labels, nil,
		),
		usedBySnapshots: prometheus.NewDesc(
			"zfs_dataset_used_by_snapshots_bytes",
			"Space used by snapshots of this dataset (in bytes).",
			labels, nil,
		),
		usedByRefreservation: prometheus.NewDesc(
			"zfs_dataset_used_by_refreservation_bytes",
			"Space used by the refreservation set on this dataset (in bytes).",
			labels, nil,
		),
		logger: logger,
	}
}

// zfsCommand builds an *exec.Cmd for running a ZFS command.
// When running inside a container, it uses chroot into the host rootfs.
func (c *DatasetCollector) zfsCommand(args ...string) *exec.Cmd {
	if c.opts.IsContainer() {
		chrootArgs := append([]string{c.opts.RootfsPath, "zfs"}, args...)
		return exec.Command("chroot", chrootArgs...)
	}
	return exec.Command("zfs", args...)
}

// Describe implements prometheus.Collector.
func (c *DatasetCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.usedBytes
	ch <- c.availableBytes
	ch <- c.referencedBytes
	ch <- c.logicalUsed
	ch <- c.logicalRef
	ch <- c.written
	ch <- c.snapCount
	ch <- c.compressRatio
	ch <- c.quota
	ch <- c.reservation
	ch <- c.recordSize
	ch <- c.usedByChildren
	ch <- c.usedByDataset
	ch <- c.usedBySnapshots
	ch <- c.usedByRefreservation
}

// Collect implements prometheus.Collector.
func (c *DatasetCollector) Collect(ch chan<- prometheus.Metric) {
	datasets, err := c.getDatasets()
	if err != nil {
		c.logger.Error("failed to collect dataset metrics", "error", err)
		return
	}

	for _, ds := range datasets {
		labels := []string{ds.name, ds.mountpoint, ds.dsType, ds.pool}

		ch <- prometheus.MustNewConstMetric(c.usedBytes, prometheus.GaugeValue, float64(ds.used), labels...)
		ch <- prometheus.MustNewConstMetric(c.availableBytes, prometheus.GaugeValue, float64(ds.available), labels...)
		ch <- prometheus.MustNewConstMetric(c.referencedBytes, prometheus.GaugeValue, float64(ds.referenced), labels...)
		ch <- prometheus.MustNewConstMetric(c.logicalUsed, prometheus.GaugeValue, float64(ds.logicalUsed), labels...)
		ch <- prometheus.MustNewConstMetric(c.logicalRef, prometheus.GaugeValue, float64(ds.logicalRef), labels...)
		ch <- prometheus.MustNewConstMetric(c.written, prometheus.GaugeValue, float64(ds.written), labels...)
		ch <- prometheus.MustNewConstMetric(c.snapCount, prometheus.GaugeValue, float64(ds.snapCount), labels...)
		ch <- prometheus.MustNewConstMetric(c.compressRatio, prometheus.GaugeValue, ds.compressRatio, labels...)
		ch <- prometheus.MustNewConstMetric(c.quota, prometheus.GaugeValue, float64(ds.quota), labels...)
		ch <- prometheus.MustNewConstMetric(c.reservation, prometheus.GaugeValue, float64(ds.reservation), labels...)
		ch <- prometheus.MustNewConstMetric(c.recordSize, prometheus.GaugeValue, float64(ds.recordSize), labels...)
		ch <- prometheus.MustNewConstMetric(c.usedByChildren, prometheus.GaugeValue, float64(ds.usedByChildren), labels...)
		ch <- prometheus.MustNewConstMetric(c.usedByDataset, prometheus.GaugeValue, float64(ds.usedByDataset), labels...)
		ch <- prometheus.MustNewConstMetric(c.usedBySnapshots, prometheus.GaugeValue, float64(ds.usedBySnapshots), labels...)
		ch <- prometheus.MustNewConstMetric(c.usedByRefreservation, prometheus.GaugeValue, float64(ds.usedByRefreservation), labels...)
	}
}

type dataset struct {
	name          string
	pool          string
	mountpoint    string
	dsType        string
	used          int64
	available     int64
	referenced    int64
	logicalUsed   int64
	logicalRef    int64
	written       int64
	snapCount     int64
	compressRatio float64
	quota                int64
	reservation          int64
	recordSize           int64
	usedByChildren       int64
	usedByDataset        int64
	usedBySnapshots      int64
	usedByRefreservation int64
}

// zfs list properties collected with -p (parseable / exact bytes).
const datasetProperties = "name,used,available,refer,mountpoint,type,logicalused,logicalreferenced,written,snapshot_count,compressratio,quota,reservation,recordsize,usedbychildren,usedbydataset,usedbysnapshots,usedbyrefreservation"

func (c *DatasetCollector) getDatasets() ([]dataset, error) {
	// -Hp: scripting mode (tab-separated, no headers), parseable exact-byte values.
	cmd := c.zfsCommand("list", "-Hp", "-o", datasetProperties, "-t", "filesystem,volume")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		c.logger.Error("zfs list failed", "stderr", stderr.String(), "error", err)
		return nil, err
	}

	var datasets []dataset
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 18 {
			c.logger.Warn("unexpected number of fields", "line", line, "fields", len(fields))
			continue
		}

		pool := fields[0]
		if idx := strings.Index(pool, "/"); idx != -1 {
			pool = pool[:idx]
		}

		ds := dataset{
			name:       fields[0],
			pool:       pool,
			mountpoint: fields[4],
			dsType:     fields[5],
		}

		ds.used = parseInt64(fields[1])
		ds.available = parseInt64(fields[2])
		ds.referenced = parseInt64(fields[3])
		ds.logicalUsed = parseInt64(fields[6])
		ds.logicalRef = parseInt64(fields[7])
		ds.written = parseInt64(fields[8])
		ds.snapCount = parseInt64(fields[9])
		ds.compressRatio = parseFloat64(fields[10])
		ds.quota = parseInt64(fields[11])
		ds.reservation = parseInt64(fields[12])
		ds.recordSize = parseInt64(fields[13])
		ds.usedByChildren = parseInt64(fields[14])
		ds.usedByDataset = parseInt64(fields[15])
		ds.usedBySnapshots = parseInt64(fields[16])
		ds.usedByRefreservation = parseInt64(fields[17])

		datasets = append(datasets, ds)
	}

	return datasets, nil
}

func parseInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "-" || s == "none" || s == "" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseFloat64(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "-" || s == "none" || s == "" {
		return 0
	}
	// ZFS compressratio in parseable mode is e.g. "1.50x"
	s = strings.TrimSuffix(s, "x")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
