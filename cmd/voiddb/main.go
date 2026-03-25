// Command voiddb is the VoidDB server binary.
// It reads a YAML configuration file (default: config.yaml), starts the
// storage engine, and serves the REST + S3-compatible HTTP APIs.
//
// Usage:
//
//	voiddb [-config path/to/config.yaml]
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/voiddb/void/internal/api"
	"github.com/voiddb/void/internal/auth"
	"github.com/voiddb/void/internal/backup"
	"github.com/voiddb/void/internal/blob"
	"github.com/voiddb/void/internal/config"
	"github.com/voiddb/void/internal/engine"
	"github.com/voiddb/void/internal/kvcache"
	"github.com/voiddb/void/internal/logs"
	voidtls "github.com/voiddb/void/internal/tls"
	"gopkg.in/natefinch/lumberjack.v2"
)

// version is set by ldflags at build time.
var version = "dev"

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "voiddb: load config: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "voiddb: invalid config: %v\n", err)
		os.Exit(1)
	}

	// --- Logger --------------------------------------------------------------
	var encoder zapcore.Encoder
	if cfg.Log.Format == "json" {
		encoder = zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	} else {
		encoder = zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	}

	rawLevel := zapcore.InfoLevel
	if cfg.Log.Level == "debug" {
		rawLevel = zapcore.DebugLevel
	}

	consoleCore := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), rawLevel)
	ringCore := logs.Hook()
	cores := []zapcore.Core{consoleCore, ringCore}

	if cfg.Log.OutputPath != "" && cfg.Log.OutputPath != "stdout" && cfg.Log.OutputPath != "stderr" {
		if err := os.MkdirAll(filepath.Dir(cfg.Log.OutputPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "voiddb: create log dir: %v\n", err)
			os.Exit(1)
		}
		lumberjackLogger := &lumberjack.Logger{
			Filename:   cfg.Log.OutputPath,
			MaxSize:    100,
			MaxBackups: 7,
			MaxAge:     7,
			Compress:   true,
		}
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(lumberjackLogger), rawLevel))
	}

	core := zapcore.NewTee(cores...)
	logger := zap.New(core, zap.AddCaller())

	defer logger.Sync() //nolint:errcheck
	zap.ReplaceGlobals(logger)

	// --- Storage Engine ------------------------------------------------------
	eng, err := engine.Open(engine.Options{
		DataDir:             cfg.Engine.DataDir,
		WALDir:              cfg.Engine.WALDir,
		MemTableSize:        cfg.Engine.MemTableSize,
		BlockCacheSize:      cfg.Engine.BlockCacheSize,
		BloomFPRate:         cfg.Engine.BloomFalsePositiveRate,
		CompactionWorkers:   cfg.Engine.CompactionWorkers,
		SyncWAL:             cfg.Engine.SyncWAL,
		MaxLevels:           cfg.Engine.MaxLevels,
		LevelSizeMultiplier: cfg.Engine.LevelSizeMultiplier,
	})
	if err != nil {
		logger.Fatal("open engine", zap.Error(err))
	}
	defer func() {
		if err := eng.Close(); err != nil {
			logger.Error("close engine", zap.Error(err))
		}
	}()

	store := engine.NewStore(eng)
	logger.Info("storage engine started",
		zap.String("data_dir", cfg.Engine.DataDir),
		zap.Int64("memtable_size_mb", cfg.Engine.MemTableSize/1024/1024),
	)

	// --- Auth ----------------------------------------------------------------
	authSvc := auth.NewService(
		cfg.Auth.JWTSecret,
		cfg.Auth.TokenExpiry,
		cfg.Auth.RefreshExpiry,
	)
	if err := authSvc.Bootstrap(cfg.Auth.AdminPassword); err != nil {
		logger.Fatal("bootstrap auth", zap.Error(err))
	}
	logger.Info("auth service started")

	// --- Blob Store ----------------------------------------------------------
	blobStore, err := blob.NewStore(cfg.Blob.StorageDir, cfg.Blob.MaxObjectSize)
	if err != nil {
		logger.Fatal("open blob store", zap.Error(err))
	}
	logger.Info("blob store started", zap.String("storage_dir", cfg.Blob.StorageDir))

	// --- TLS -----------------------------------------------------------------
	tlsMgr, err := voidtls.New(voidtls.Config{
		Mode:         voidtls.Mode(cfg.TLS.Mode),
		CertFile:     cfg.TLS.CertFile,
		KeyFile:      cfg.TLS.KeyFile,
		Domain:       cfg.TLS.Domain,
		ExtraDomains: cfg.TLS.ExtraDomains,
		AcmeEmail:    cfg.TLS.AcmeEmail,
		AcmeCacheDir: cfg.TLS.AcmeCacheDir,
		RedirectHTTP: cfg.TLS.RedirectHTTP,
		HTTPSrcPort:  cfg.TLS.HTTPSrcPort,
		HTTPSPort:    cfg.TLS.HTTPSPort,
	})
	if err != nil {
		logger.Fatal("init TLS manager", zap.Error(err))
	}

	tlsConfig, err := tlsMgr.TLSConfig()
	if err != nil {
		logger.Fatal("build TLS config", zap.Error(err))
	}

	// Init Cache
	memCache := kvcache.New()
	defer memCache.Close()

	backupSvc, err := backup.NewService(store, cfg, *configPath, version)
	if err != nil {
		logger.Fatal("start backup service", zap.Error(err))
	}
	defer backupSvc.Close()

	// --- HTTP Router ---------------------------------------------------------
	router := api.NewRouter(store, authSvc, blobStore, backupSvc, memCache, api.RouterOptions{
		CORSOrigins: cfg.Server.CORSOrigins,
		S3Region:    cfg.Blob.S3Region,
	})

	// Determine listen address.
	var listenAddr string
	if tlsMgr.Enabled() {
		listenAddr = tlsMgr.ListenAddr()
	} else {
		listenAddr = fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	}

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		TLSConfig:    tlsConfig,
	}

	// --- Start HTTP-01 challenge / redirect server ---------------------------
	tlsMgr.StartRedirectServer()

	// --- Start main server ---------------------------------------------------
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logger.Fatal("listen", zap.String("addr", listenAddr), zap.Error(err))
	}

	go func() {
		scheme := "http"
		if tlsMgr.Enabled() {
			scheme = "https"
		}
		logger.Info("VoidDB server listening",
			zap.String("addr", listenAddr),
			zap.String("scheme", scheme),
			zap.String("version", version),
		)
		var serveErr error
		if tlsMgr.Enabled() {
			serveErr = srv.ServeTLS(ln, "", "") // certs already in srv.TLSConfig
		} else {
			serveErr = srv.Serve(ln)
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(serveErr))
		}
	}()

	// --- Graceful shutdown ---------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
	}
	logger.Info("VoidDB stopped")
}
