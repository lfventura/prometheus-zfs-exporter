package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/lfventura/zfs-exporter/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	version = "dev"
)

func main() {
	listenAddr := flag.String("web.listen-address", ":9550", "Address to listen on for metrics.")
	metricsPath := flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	procPath := flag.String("path.procfs", "/proc", "procfs mount point (use /host/proc when running inside a container).")
	rootfsPath := flag.String("path.rootfs", "/", "Root filesystem mount point (use /host when running inside a container). Used to locate ZFS/zpool binaries via chroot.")
	showVersion := flag.Bool("version", false, "Print version and exit.")
	logLevel := flag.String("log.level", "info", "Log level: debug, info, warn, error.")

	flag.Parse()

	if *showVersion {
		fmt.Printf("zfs-exporter version %s\n", version)
		os.Exit(0)
	}

	// Configure structured logger.
	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	logger.Info("starting zfs-exporter",
		"version", version,
		"listen", *listenAddr,
		"path.procfs", *procPath,
		"path.rootfs", *rootfsPath,
	)

	// Build collector options from flags.
	opts := collector.Options{
		ProcPath:   *procPath,
		RootfsPath: *rootfsPath,
	}

	// Register collectors.
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		collector.NewDatasetCollector(logger, opts),
		collector.NewPoolCollector(logger, opts),
		collector.NewARCCollector(logger, opts),
	)

	http.Handle(*metricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<html><head><title>ZFS Exporter</title></head>
<body><h1>ZFS Exporter</h1>
<p><a href="%s">Metrics</a></p>
</body></html>`, *metricsPath)
	})

	logger.Info("listening", "address", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, nil); err != nil {
		logger.Error("http server error", "error", err)
		os.Exit(1)
	}
}
