// Package config provides configuration management for VoidDB.
// It supports YAML config files, environment variables and sensible defaults.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure for VoidDB.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Engine   EngineConfig   `yaml:"engine"`
	Auth     AuthConfig     `yaml:"auth"`
	Blob     BlobConfig     `yaml:"blob"`
	Log      LogConfig      `yaml:"log"`
	Admin    AdminConfig    `yaml:"admin"`
	TLS      TLSConfig      `yaml:"tls"`
	Backup   BackupConfig   `yaml:"backup"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Host to bind the API server on.
	Host string `yaml:"host"`
	// Port for the main API server.
	Port int `yaml:"port"`
	// BlobPort is the port for S3-compatible blob API.
	BlobPort int `yaml:"blob_port"`
	// ReadTimeout for HTTP requests.
	ReadTimeout time.Duration `yaml:"read_timeout"`
	// WriteTimeout for HTTP responses.
	WriteTimeout time.Duration `yaml:"write_timeout"`
	// MaxBodySize limits request body in bytes (default 64 MB).
	MaxBodySize int64 `yaml:"max_body_size"`
	// CORS allowed origins.
	CORSOrigins []string `yaml:"cors_origins"`
}

// EngineConfig holds storage engine settings.
type EngineConfig struct {
	// DataDir is the root directory where all data is persisted.
	DataDir string `yaml:"data_dir"`
	// MemTableSize is the max memtable size in bytes before flushing (default 64 MB).
	MemTableSize int64 `yaml:"memtable_size"`
	// BlockCacheSize is the LRU block cache size in bytes (default 256 MB).
	BlockCacheSize int64 `yaml:"block_cache_size"`
	// BloomFalsePositiveRate controls bloom filter accuracy (default 0.01 = 1%).
	BloomFalsePositiveRate float64 `yaml:"bloom_false_positive_rate"`
	// CompactionWorkers is the number of background compaction goroutines.
	CompactionWorkers int `yaml:"compaction_workers"`
	// SyncWAL forces fsync on every WAL write (safer but slower).
	SyncWAL bool `yaml:"sync_wal"`
	// WALDir overrides WAL directory (defaults to DataDir/wal).
	WALDir string `yaml:"wal_dir"`
	// MaxLevels for LSM compaction (default 7).
	MaxLevels int `yaml:"max_levels"`
	// LevelSizeMultiplier controls level size growth (default 10).
	LevelSizeMultiplier int `yaml:"level_size_multiplier"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	// JWTSecret is the HMAC-SHA256 signing key for JWT tokens.
	JWTSecret string `yaml:"jwt_secret"`
	// TokenExpiry is how long access tokens are valid (default 24h).
	TokenExpiry time.Duration `yaml:"token_expiry"`
	// RefreshExpiry is how long refresh tokens are valid (default 7d).
	RefreshExpiry time.Duration `yaml:"refresh_expiry"`
	// AdminPassword is the initial root admin password.
	AdminPassword string `yaml:"admin_password"`
	// RateLimitRPS is max requests per second per IP.
	RateLimitRPS int `yaml:"rate_limit_rps"`
}

// BlobConfig holds blob/object storage settings.
type BlobConfig struct {
	// StorageDir is where blob files are written.
	StorageDir string `yaml:"storage_dir"`
	// MaxObjectSize limits a single object in bytes (default 5 GB).
	MaxObjectSize int64 `yaml:"max_object_size"`
	// EnableS3API enables the S3-compatible HTTP endpoints.
	EnableS3API bool `yaml:"enable_s3_api"`
	// S3Region reported in ListBuckets responses.
	S3Region string `yaml:"s3_region"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	// Level is the minimum log level: debug, info, warn, error.
	Level string `yaml:"level"`
	// Format is "json" or "console".
	Format string `yaml:"format"`
	// OutputPath is a file path or "stdout"/"stderr".
	OutputPath string `yaml:"output_path"`
}

// AdminConfig holds admin panel settings.
type AdminConfig struct {
	// Enabled controls whether to serve the admin SPA from the API server.
	Enabled bool `yaml:"enabled"`
	// StaticDir is the path to the built Next.js export.
	StaticDir string `yaml:"static_dir"`
}

// TLSConfig holds TLS / SSL / Let's Encrypt settings.
type TLSConfig struct {
	// Mode selects the certificate source: off | file | acme.
	Mode string `yaml:"mode"`
	// CertFile is the path to a PEM-encoded certificate (or chain) file.
	CertFile string `yaml:"cert_file"`
	// KeyFile is the path to the PEM-encoded private key file.
	KeyFile string `yaml:"key_file"`
	// Domain is the primary public hostname (required for acme mode).
	Domain string `yaml:"domain"`
	// ExtraDomains are additional SANs for ACME certificates.
	ExtraDomains []string `yaml:"extra_domains"`
	// AcmeEmail is the contact address registered with Let's Encrypt.
	AcmeEmail string `yaml:"acme_email"`
	// AcmeCacheDir stores ACME account keys and renewed certs (default: ./data/acme-cache).
	AcmeCacheDir string `yaml:"acme_cache_dir"`
	// RedirectHTTP starts a plain-HTTP listener that issues 301→HTTPS redirects.
	RedirectHTTP bool `yaml:"redirect_http"`
	// HTTPSrcPort is the plain-HTTP port for redirects/challenges (default 80).
	HTTPSrcPort int `yaml:"http_src_port"`
	// HTTPSPort is the HTTPS listen port (default 443).
	HTTPSPort int `yaml:"https_port"`
}

// BackupConfig holds backup scheduling and storage settings.
type BackupConfig struct {
	// Dir is where automatic backup archives are written.
	Dir string `yaml:"dir"`
	// Retain is how many backup files to keep (oldest deleted, 0 = keep all).
	Retain int `yaml:"retain"`
	// ScheduleCron is a cron expression for automatic backups (empty = disabled).
	// Example: "0 2 * * *" (daily at 02:00 UTC).
	ScheduleCron string `yaml:"schedule_cron"`
}

// DefaultConfig returns a Config populated with production-ready defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         7700,
			BlobPort:     7701,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			MaxBodySize:  64 * 1024 * 1024,
			CORSOrigins:  []string{"*"},
		},
		Engine: EngineConfig{
			DataDir:                "./data",
			MemTableSize:           64 * 1024 * 1024,
			BlockCacheSize:         256 * 1024 * 1024,
			BloomFalsePositiveRate: 0.01,
			CompactionWorkers:      2,
			SyncWAL:                false,
			MaxLevels:              7,
			LevelSizeMultiplier:    10,
		},
		Auth: AuthConfig{
			JWTSecret:     "change-me-in-production",
			TokenExpiry:   24 * time.Hour,
			RefreshExpiry: 7 * 24 * time.Hour,
			AdminPassword: "admin",
			RateLimitRPS:  1000,
		},
		Blob: BlobConfig{
			StorageDir:    "./blob",
			MaxObjectSize: 5 * 1024 * 1024 * 1024,
			EnableS3API:   true,
			S3Region:      "void-1",
		},
		Log: LogConfig{
			Level:      "info",
			Format:     "console",
			OutputPath: "./logs/voiddb.log",
		},
		Admin: AdminConfig{
			Enabled:   true,
			StaticDir: "./admin/out",
		},
		TLS: TLSConfig{
			Mode:        "off",
			HTTPSrcPort: 80,
			HTTPSPort:   443,
			AcmeCacheDir: "./data/acme-cache",
		},
		Backup: BackupConfig{
			Dir:    "./backups",
			Retain: 14,
		},
	}
}

// Load reads a YAML config file from path and merges it with defaults.
// Environment variables prefixed with VOID_ override file values.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("config: read file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: parse yaml: %w", err)
		}
	}

	// Override from environment variables.
	applyEnv(cfg)

	return cfg, nil
}

// Save writes the configuration back to the file
func (c *Config) Save(path string) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// applyEnv overrides config values from VOID_* environment variables.
func applyEnv(cfg *Config) {
	if v := os.Getenv("VOID_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("VOID_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("VOID_DATA_DIR"); v != "" {
		cfg.Engine.DataDir = v
	}
	if v := os.Getenv("VOID_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("VOID_ADMIN_PASSWORD"); v != "" {
		cfg.Auth.AdminPassword = v
	}
	if v := os.Getenv("VOID_BLOB_DIR"); v != "" {
		cfg.Blob.StorageDir = v
	}
	if v := os.Getenv("VOID_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("VOID_TLS_MODE"); v != "" {
		cfg.TLS.Mode = v
	}
	if v := os.Getenv("VOID_DOMAIN"); v != "" {
		cfg.TLS.Domain = v
	}
	if v := os.Getenv("VOID_ACME_EMAIL"); v != "" {
		cfg.TLS.AcmeEmail = v
	}
	if v := os.Getenv("VOID_TLS_CERT"); v != "" {
		cfg.TLS.CertFile = v
	}
	if v := os.Getenv("VOID_TLS_KEY"); v != "" {
		cfg.TLS.KeyFile = v
	}
}

// Validate checks that all required config fields are valid.
func (c *Config) Validate() error {
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("config: auth.jwt_secret must not be empty")
	}
	if c.Engine.DataDir == "" {
		return fmt.Errorf("config: engine.data_dir must not be empty")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("config: server.port %d is out of range", c.Server.Port)
	}
	return nil
}
