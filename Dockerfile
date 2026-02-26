# Build stage
FROM golang:1.26-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o /zfs-exporter .

# Runtime stage — minimal image, ZFS tools come from the host via chroot.
FROM debian:bookworm-slim

# We don't install ZFS tools here because the container uses chroot into the
# host rootfs (/host) to invoke the host's zfs/zpool binaries.
# This avoids version mismatches between container and host ZFS.
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /zfs-exporter /usr/local/bin/zfs-exporter

EXPOSE 9550

ENTRYPOINT ["zfs-exporter"]
CMD ["--path.rootfs=/host", "--path.procfs=/host/proc"]
