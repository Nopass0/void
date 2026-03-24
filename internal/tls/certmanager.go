// Package tls provides TLS certificate management for VoidDB.
//
// Supported modes (configured via config.yaml tls section):
//
//  1. Disabled  – plain HTTP (default).
//  2. File      – bring-your-own cert/key PEM files.
//  3. ACME      – automatic Let's Encrypt via HTTP-01 challenge.
//  4. Wildcard  – manual DNS-01 wildcard certificate supplied as PEM files
//     (set mode: "file", wildcard: true as documentation hint).
package tls

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// Mode describes which certificate source is active.
type Mode string

const (
	// ModeOff disables TLS – HTTP only.
	ModeOff Mode = "off"
	// ModeFile loads a cert/key from PEM files on disk.
	// Also used for wildcard certs that are provisioned externally via DNS-01.
	ModeFile Mode = "file"
	// ModeACME obtains and renews certificates via Let's Encrypt HTTP-01.
	ModeACME Mode = "acme"
)

// Config contains all TLS-related settings mapped from config.yaml.
type Config struct {
	// Mode selects the certificate source: off | file | acme.
	Mode Mode `yaml:"mode"`

	// --- File mode -----------------------------------------------------------

	// CertFile is the path to a PEM-encoded certificate (or full chain) file.
	CertFile string `yaml:"cert_file"`
	// KeyFile is the path to the PEM-encoded private key file.
	KeyFile string `yaml:"key_file"`

	// --- Domain & ACME -------------------------------------------------------

	// Domain is the primary public domain (e.g. "void.example.com").
	// For wildcard coverage, supply a cert signed for *.example.com in
	// CertFile/KeyFile and set Mode to "file".
	Domain string `yaml:"domain"`
	// ExtraDomains are additional SANs requested from Let's Encrypt.
	ExtraDomains []string `yaml:"extra_domains"`
	// AcmeEmail is the contact address registered with Let's Encrypt.
	AcmeEmail string `yaml:"acme_email"`
	// AcmeCacheDir stores account keys and renewed certificates between
	// restarts. Defaults to "./data/acme-cache".
	AcmeCacheDir string `yaml:"acme_cache_dir"`

	// --- HTTP→HTTPS redirect -------------------------------------------------

	// RedirectHTTP starts a plain-HTTP listener on HTTPSrcPort that issues
	// 301 redirects to HTTPS and handles ACME HTTP-01 challenges.
	RedirectHTTP bool `yaml:"redirect_http"`
	// HTTPSrcPort is the plain-HTTP port for redirects/challenges (default 80).
	HTTPSrcPort int `yaml:"http_src_port"`
	// HTTPSPort is the HTTPS listen port (default 443).
	HTTPSPort int `yaml:"https_port"`
}

// Manager wraps certificate acquisition and server configuration.
type Manager struct {
	cfg      Config
	autocert *autocert.Manager
}

// New creates a Manager from cfg.
// Call TLSConfig() to get the *tls.Config for your https.Server.
func New(cfg Config) (*Manager, error) {
	// Apply defaults.
	if cfg.AcmeCacheDir == "" {
		cfg.AcmeCacheDir = "./data/acme-cache"
	}
	if cfg.HTTPSrcPort == 0 {
		cfg.HTTPSrcPort = 80
	}
	if cfg.HTTPSPort == 0 {
		cfg.HTTPSPort = 443
	}

	m := &Manager{cfg: cfg}

	if cfg.Mode == ModeACME {
		if cfg.Domain == "" {
			return nil, fmt.Errorf("tls: acme mode requires domain to be set")
		}
		if cfg.AcmeEmail == "" {
			return nil, fmt.Errorf("tls: acme mode requires acme_email to be set")
		}
		if err := os.MkdirAll(cfg.AcmeCacheDir, 0700); err != nil {
			return nil, fmt.Errorf("tls: create acme cache dir: %w", err)
		}

		domains := append([]string{cfg.Domain}, cfg.ExtraDomains...)
		m.autocert = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(cfg.AcmeCacheDir),
			HostPolicy: autocert.HostWhitelist(domains...),
			Email:      cfg.AcmeEmail,
			Client: &acme.Client{
				DirectoryURL: acme.LetsEncryptURL,
			},
		}
	}

	return m, nil
}

// TLSConfig returns the *tls.Config to attach to an http.Server.
// Returns nil when Mode is ModeOff (plain HTTP).
func (m *Manager) TLSConfig() (*tls.Config, error) {
	switch m.cfg.Mode {
	case ModeOff, "":
		return nil, nil

	case ModeFile:
		return m.loadFileConfig()

	case ModeACME:
		tc := m.autocert.TLSConfig()
		tc.MinVersion = tls.VersionTLS12
		tc.CurvePreferences = preferredCurves()
		return tc, nil

	default:
		return nil, fmt.Errorf("tls: unknown mode %q", m.cfg.Mode)
	}
}

// loadFileConfig builds a *tls.Config from PEM files on disk.
func (m *Manager) loadFileConfig() (*tls.Config, error) {
	if m.cfg.CertFile == "" || m.cfg.KeyFile == "" {
		return nil, fmt.Errorf("tls: file mode requires cert_file and key_file")
	}
	cert, err := tls.LoadX509KeyPair(m.cfg.CertFile, m.cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("tls: load key pair (%s, %s): %w",
			m.cfg.CertFile, m.cfg.KeyFile, err)
	}
	return &tls.Config{
		Certificates:     []tls.Certificate{cert},
		MinVersion:       tls.VersionTLS12,
		CurvePreferences: preferredCurves(),
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}, nil
}

// ACMEChallengeHandler returns the HTTP handler for ACME HTTP-01 challenges.
// Mount it at "/.well-known/acme-challenge/" in your plain-HTTP router, or
// start a dedicated redirect server via StartRedirectServer.
// Returns nil when not in ACME mode.
func (m *Manager) ACMEChallengeHandler() http.Handler {
	if m.autocert == nil {
		return nil
	}
	return m.autocert.HTTPHandler(nil)
}

// StartRedirectServer starts a goroutine that listens on HTTPSrcPort and:
//   - handles ACME HTTP-01 challenges (ACME mode only), then
//   - issues 301 redirects to https://Domain<path>.
//
// No-op when RedirectHTTP is false.
func (m *Manager) StartRedirectServer() {
	if !m.cfg.RedirectHTTP {
		return
	}

	target := strings.TrimRight(m.cfg.Domain, "/")
	redirectFn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dest := "https://" + target + r.RequestURI
		http.Redirect(w, r, dest, http.StatusMovedPermanently)
	})

	var handler http.Handler
	if m.autocert != nil {
		handler = m.autocert.HTTPHandler(redirectFn)
	} else {
		handler = redirectFn
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", m.cfg.HTTPSrcPort),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		_ = srv.ListenAndServe() // errors logged by caller if needed
	}()
}

// Enabled reports whether TLS is active.
func (m *Manager) Enabled() bool {
	return m.cfg.Mode != ModeOff && m.cfg.Mode != ""
}

// ListenAddr returns the HTTPS listen address (e.g. ":443").
func (m *Manager) ListenAddr() string {
	return fmt.Sprintf(":%d", m.cfg.HTTPSPort)
}

// preferredCurves returns the ordered list of preferred TLS elliptic curves.
func preferredCurves() []tls.CurveID {
	return []tls.CurveID{tls.X25519, tls.CurveP256}
}
