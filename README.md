# ZFS Exporter for Prometheus

A Prometheus exporter for ZFS metrics, designed to run as a Docker container on TrueNAS (or any Linux ZFS host).

It collects per-dataset, per-pool, and ARC cache metrics from ZFS, exposing them in Prometheus format on an HTTP endpoint.

## Metrics Exported

### Per-Dataset (`zfs list`)

Collected via `zfs list -Hp` with tab-separated output.

| Metric | Type | Description |
|---|---|---|
| `zfs_dataset_used_bytes` | Gauge | Total space consumed (dataset + snapshots + children + refreservation) |
| `zfs_dataset_available_bytes` | Gauge | Space available to the dataset |
| `zfs_dataset_referenced_bytes` | Gauge | Data uniquely referenced by this dataset |
| `zfs_dataset_logical_used_bytes` | Gauge | Logical used before compression/dedup |
| `zfs_dataset_logical_referenced_bytes` | Gauge | Logical referenced before compression/dedup |
| `zfs_dataset_written_bytes` | Gauge | Bytes written since last snapshot |
| `zfs_dataset_snapshot_count` | Gauge | Number of snapshots |
| `zfs_dataset_compression_ratio` | Gauge | Compression ratio (e.g. 1.5x) |
| `zfs_dataset_quota_bytes` | Gauge | Quota (0 = none) |
| `zfs_dataset_reservation_bytes` | Gauge | Reservation (0 = none) |
| `zfs_dataset_record_size_bytes` | Gauge | Record size |
| `zfs_dataset_used_by_children_bytes` | Gauge | Space used by child datasets |
| `zfs_dataset_used_by_dataset_bytes` | Gauge | Space used by the dataset itself (USEDDS — excludes snapshots) |
| `zfs_dataset_used_by_snapshots_bytes` | Gauge | Space used by snapshots (USEDSNAP — would be freed if all snapshots were deleted) |
| `zfs_dataset_used_by_refreservation_bytes` | Gauge | Space used by refreservation |

Labels: `name`, `mountpoint`, `type`, `pool`

> **Note**: The sum of `used_by_dataset` + `used_by_snapshots` + `used_by_children` + `used_by_refreservation` equals `used`.

### Per-Pool (`zpool list` / `zpool status`)

Collected via `zpool list -Hp` and `zpool status -p`.

| Metric | Type | Description |
|---|---|---|
| `zfs_pool_size_bytes` | Gauge | Total pool size |
| `zfs_pool_allocated_bytes` | Gauge | Allocated space |
| `zfs_pool_free_bytes` | Gauge | Free space |
| `zfs_pool_fragmentation_percent` | Gauge | Fragmentation % |
| `zfs_pool_capacity_percent` | Gauge | Capacity used % |
| `zfs_pool_deduplication_ratio` | Gauge | Dedup ratio |
| `zfs_pool_healthy` | Gauge | 1 = ONLINE, 0 = degraded/faulted |
| `zfs_pool_read_errors_total` | Gauge | Read errors (from `zpool status`) |
| `zfs_pool_write_errors_total` | Gauge | Write errors (from `zpool status`) |
| `zfs_pool_checksum_errors_total` | Gauge | Checksum errors (from `zpool status`) |

Labels: `pool` (+ `health` for `zfs_pool_healthy`)

### Pool Logical I/O (`/proc/spl/kstat/zfs/<pool>/iostats`)

Cumulative counters of **all I/O requests** that pass through the ZFS ARC subsystem.
This is **logical I/O** — what applications requested from the pool, including both ARC cache hits and actual disk reads.

| Metric | Type | Description |
|---|---|---|
| `zfs_pool_logical_read_ops_total` | Counter | Total logical read operations |
| `zfs_pool_logical_write_ops_total` | Counter | Total logical write operations |
| `zfs_pool_logical_read_bytes_total` | Counter | Total logical bytes read |
| `zfs_pool_logical_write_bytes_total` | Counter | Total logical bytes written |

Labels: `pool`

> **Note**: Use `derivative(unit: 1s, nonNegative: true)` in Flux or `rate()` in PromQL to convert to ops/s or bytes/s.

### Pool Physical Disk I/O (`/proc/diskstats`)

Cumulative counters of **actual physical disk I/O**, aggregated per pool.
Disks are mapped to pools by parsing `zpool status` and resolved via `/dev/disk/by-id/` symlinks.
Sectors from `/proc/diskstats` are converted to bytes (× 512).

| Metric | Type | Description |
|---|---|---|
| `zfs_pool_physical_read_ops_total` | Counter | Total physical read operations completed on pool disks |
| `zfs_pool_physical_write_ops_total` | Counter | Total physical write operations completed on pool disks |
| `zfs_pool_physical_read_bytes_total` | Counter | Total physical bytes read from pool disks |
| `zfs_pool_physical_write_bytes_total` | Counter | Total physical bytes written to pool disks |

Labels: `pool`

> **Logical vs Physical I/O**: Logical I/O may be higher than physical for reads (ARC cache hits satisfy reads without touching disk) or lower for writes (ZFS write amplification from checksums, metadata, and redundancy).

### ARC Cache (`/proc/spl/kstat/zfs/arcstats`)

ZFS Adaptive Replacement Cache statistics from the kernel.

| Metric | Type | Description |
|---|---|---|
| `zfs_arc_size_bytes` | Gauge | Current ARC size |
| `zfs_arc_max_size_bytes` | Gauge | Max target ARC size |
| `zfs_arc_min_size_bytes` | Gauge | Min target ARC size |
| `zfs_arc_hits_total` | Counter | Total ARC hits |
| `zfs_arc_misses_total` | Counter | Total ARC misses |
| `zfs_arc_hit_ratio` | Gauge | Hit ratio (0.0–1.0) |
| `zfs_arc_l2_size_bytes` | Gauge | L2ARC size |
| `zfs_arc_l2_hits_total` | Counter | L2ARC hits |
| `zfs_arc_l2_misses_total` | Counter | L2ARC misses |

## Running on TrueNAS

### Docker Compose (Custom App)

```yaml
services:
  zfs-exporter:
    image: ghcr.io/lfventura/prometheus-zfs-exporter:latest
    container_name: zfs_exporter
    pid: host
    privileged: true
    ports:
      - "9550:9550"
    restart: unless-stopped
    command:
      - "--path.rootfs=/host"
      - "--path.procfs=/host/proc"
      - "--web.listen-address=:9550"
    volumes:
      - /:/host:ro,rslave
      - /dev:/host/dev:ro
```

The container uses `chroot /host` to run the host's `zfs` and `zpool` binaries, avoiding version mismatches between container and host ZFS. The host's `/proc` is mapped to read ARC stats, pool iostats, and disk I/O counters.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--web.listen-address` | `:9550` | Address to listen on |
| `--web.telemetry-path` | `/metrics` | Metrics endpoint path |
| `--path.rootfs` | `/` | Host root filesystem (use `/host` in containers) |
| `--path.procfs` | `/proc` | procfs mount point (use `/host/proc` in containers) |
| `--log.level` | `info` | Log level: debug, info, warn, error |
| `--version` | | Print version and exit |

### Prometheus Configuration

```yaml
scrape_configs:
  - job_name: "zfs"
    static_configs:
      - targets: ["truenas-host:9550"]
```

## Building

```bash
# Local build
go build -o zfs-exporter .

# Docker
docker compose build
docker compose up -d
```

## Architecture

```
main.go                    HTTP server + CLI flags
collector/
  options.go               Shared configuration (ProcPath, RootfsPath)
  dataset.go               Per-dataset metrics from zfs list
  pool.go                  Per-pool metrics from zpool + /proc/diskstats
  arc.go                   ARC cache metrics from /proc/spl/kstat/zfs/arcstats
```

## License

MIT
