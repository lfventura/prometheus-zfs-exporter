# ZFS Exporter for Prometheus

A Prometheus exporter for ZFS metrics, designed to run as a Docker container on TrueNAS (or any Linux ZFS host).

## Metrics Exported

### Per-Dataset (`zfs list`)

| Metric | Description |
|---|---|
| `zfs_dataset_used_bytes` | Total space consumed (dataset + descendants) |
| `zfs_dataset_available_bytes` | Space available to the dataset |
| `zfs_dataset_referenced_bytes` | Data uniquely referenced by this dataset |
| `zfs_dataset_logical_used_bytes` | Logical used before compression/dedup |
| `zfs_dataset_logical_referenced_bytes` | Logical referenced before compression/dedup |
| `zfs_dataset_written_bytes` | Bytes written since last snapshot |
| `zfs_dataset_snapshot_count` | Number of snapshots |
| `zfs_dataset_compression_ratio` | Compression ratio (e.g. 1.5x) |
| `zfs_dataset_quota_bytes` | Quota (0 = none) |
| `zfs_dataset_reservation_bytes` | Reservation (0 = none) |
| `zfs_dataset_record_size_bytes` | Record size |
| `zfs_dataset_used_by_children_bytes` | Space used by child datasets |
| `zfs_dataset_used_by_dataset_bytes` | Space used by the dataset itself |
| `zfs_dataset_used_by_snapshots_bytes` | Space used by snapshots |
| `zfs_dataset_used_by_refreservation_bytes` | Space used by refreservation |

Labels: `name`, `mountpoint`, `type`, `pool`

### Per-Pool (`zpool list` / `zpool status`)

| Metric | Description |
|---|---|
| `zfs_pool_size_bytes` | Total pool size |
| `zfs_pool_allocated_bytes` | Allocated space |
| `zfs_pool_free_bytes` | Free space |
| `zfs_pool_fragmentation_percent` | Fragmentation % |
| `zfs_pool_capacity_percent` | Capacity used % |
| `zfs_pool_deduplication_ratio` | Dedup ratio |
| `zfs_pool_healthy` | 1 = ONLINE, 0 = degraded/faulted |
| `zfs_pool_read_errors_total` | Read errors |
| `zfs_pool_write_errors_total` | Write errors |
| `zfs_pool_checksum_errors_total` | Checksum errors |

Labels: `pool` (+ `health` for `zfs_pool_healthy`)

### ARC Cache (`/proc/spl/kstat/zfs/arcstats`)

| Metric | Description |
|---|---|
| `zfs_arc_size_bytes` | Current ARC size |
| `zfs_arc_max_size_bytes` | Max target ARC size |
| `zfs_arc_min_size_bytes` | Min target ARC size |
| `zfs_arc_hits_total` | Total ARC hits |
| `zfs_arc_misses_total` | Total ARC misses |
| `zfs_arc_hit_ratio` | Hit ratio (0.0–1.0) |
| `zfs_arc_l2_size_bytes` | L2ARC size |
| `zfs_arc_l2_hits_total` | L2ARC hits |
| `zfs_arc_l2_misses_total` | L2ARC misses |

## Running on TrueNAS

### Docker Compose (Custom App)

```yaml
services:
  zfs-exporter:
    build: .
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

The container uses `chroot /host` to run the host's `zfs` and `zpool` binaries, avoiding version mismatches between container and host ZFS.

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

## License

MIT
