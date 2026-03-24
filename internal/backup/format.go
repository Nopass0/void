// Package backup implements VoidDB's native .void archive format.
//
// .void file layout (a gzip-compressed tar archive):
//
//	manifest.json          – archive metadata
//	<db>/
//	  <collection>.ndjson  – newline-delimited JSON documents
//	  _blobs/<key>         – raw blob objects
//
// The format is intentionally simple: every tool that can read tar+gzip can
// inspect a .void file without the VoidDB binary.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// VoidVersion is the format version written to every manifest.
const VoidVersion = "1.0"

// Manifest is the JSON object stored as manifest.json inside every archive.
type Manifest struct {
	// VoidVersion is the archive format version.
	VoidVersion string `json:"void_version"`
	// ServerVersion is the VoidDB server version that created the archive.
	ServerVersion string `json:"server_version"`
	// CreatedAt is the UTC creation timestamp in RFC3339 format.
	CreatedAt string `json:"created_at"`
	// Databases lists which databases were included.
	Databases []string `json:"databases"`
	// Format is always "void-backup-v1".
	Format string `json:"format"`
	// Checksum is a SHA-256 hex digest of all NDJSON content (future use).
	Checksum string `json:"checksum,omitempty"`
}

// CollectionHeader is written as the first JSON object in each .ndjson file.
// Subsequent lines are the raw documents.
type CollectionHeader struct {
	Collection string `json:"__collection__"`
	Database   string `json:"__database__"`
	Count      int64  `json:"__count__"`
	ExportedAt string `json:"__exported_at__"`
}

// Writer writes a .void archive to an io.Writer (file or network stream).
type Writer struct {
	gz  *gzip.Writer
	tw  *tar.Writer
	mf  Manifest
}

// NewWriter creates a .void archive Writer that writes to w.
// serverVersion is included in the manifest for diagnostics.
func NewWriter(w io.Writer, serverVersion string) *Writer {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	return &Writer{
		gz: gz,
		tw: tw,
		mf: Manifest{
			VoidVersion:   VoidVersion,
			ServerVersion: serverVersion,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
			Format:        "void-backup-v1",
		},
	}
}

// WriteCollection writes all documents for a collection in NDJSON format.
// docs is a slice of raw JSON-marshallable document maps.
func (w *Writer) WriteCollection(db, col string, docs []map[string]interface{}) error {
	// Build NDJSON content.
	var content []byte

	// Header line.
	hdr := CollectionHeader{
		Collection: col,
		Database:   db,
		Count:      int64(len(docs)),
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}
	hdrJSON, err := json.Marshal(hdr)
	if err != nil {
		return fmt.Errorf("backup: marshal header: %w", err)
	}
	content = append(content, hdrJSON...)
	content = append(content, '\n')

	// Document lines.
	for _, doc := range docs {
		line, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("backup: marshal doc: %w", err)
		}
		content = append(content, line...)
		content = append(content, '\n')
	}

	path := filepath.Join(db, col+".ndjson")
	return w.writeEntry(path, content)
}

// WriteBlob writes a raw blob object into the archive.
func (w *Writer) WriteBlob(db, key string, data []byte) error {
	path := filepath.Join(db, "_blobs", key)
	return w.writeEntry(path, data)
}

// AddDatabase records a database name in the manifest.
func (w *Writer) AddDatabase(db string) {
	w.mf.Databases = append(w.mf.Databases, db)
}

// Close writes the manifest and closes the archive.
func (w *Writer) Close() error {
	// Write manifest.
	mfJSON, err := json.MarshalIndent(w.mf, "", "  ")
	if err != nil {
		return fmt.Errorf("backup: marshal manifest: %w", err)
	}
	if err := w.writeEntry("manifest.json", mfJSON); err != nil {
		return err
	}
	if err := w.tw.Close(); err != nil {
		return err
	}
	return w.gz.Close()
}

// writeEntry adds a file entry to the tar archive.
func (w *Writer) writeEntry(name string, data []byte) error {
	hdr := &tar.Header{
		Name:    filepath.ToSlash(name),
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: time.Now().UTC(),
	}
	if err := w.tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("backup: write tar header %s: %w", name, err)
	}
	if _, err := w.tw.Write(data); err != nil {
		return fmt.Errorf("backup: write tar data %s: %w", name, err)
	}
	return nil
}

// Reader reads a .void archive.
type Reader struct {
	gz       *gzip.Reader
	tr       *tar.Reader
	Manifest *Manifest
}

// NewReader opens a .void archive from r and parses the manifest.
// The manifest is not available until after Open() returns successfully.
func NewReader(r io.Reader) (*Reader, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("backup: not a valid .void file (gzip): %w", err)
	}
	return &Reader{gz: gz, tr: tar.NewReader(gz)}, nil
}

// ReadEntry reads the next entry from the archive.
// Returns (name, data, err). name == "" and err == io.EOF when done.
func (r *Reader) ReadEntry() (name string, data []byte, err error) {
	hdr, err := r.tr.Next()
	if err != nil {
		return "", nil, err // io.EOF is the normal end signal
	}
	data, err = io.ReadAll(r.tr)
	if err != nil {
		return "", nil, fmt.Errorf("backup: read entry %s: %w", hdr.Name, err)
	}
	// Parse manifest when we encounter it.
	if hdr.Name == "manifest.json" && r.Manifest == nil {
		r.Manifest = &Manifest{}
		if jsonErr := json.Unmarshal(data, r.Manifest); jsonErr != nil {
			return hdr.Name, data, nil // non-fatal, return raw
		}
	}
	return hdr.Name, data, nil
}

// Close closes the gzip reader.
func (r *Reader) Close() error { return r.gz.Close() }

// CreateBackupFile creates a .void archive at path and returns a Writer.
func CreateBackupFile(path, serverVersion string) (*Writer, *os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return NewWriter(f, serverVersion), f, nil
}

// OpenBackupFile opens an existing .void archive for reading.
func OpenBackupFile(path string) (*Reader, *os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	r, err := NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return r, f, nil
}
