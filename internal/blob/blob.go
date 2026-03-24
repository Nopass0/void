// Package blob implements the VoidDB object / blob storage layer.
// Objects are stored as regular files under a configurable root directory with
// the layout:  <StorageDir>/<bucket>/<key>
// Metadata (size, content-type, ETag, custom headers) is kept alongside the
// object in a companion <key>.meta JSON file.
package blob

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ObjectMeta describes an object stored in the blob layer.
type ObjectMeta struct {
	// Bucket is the containing bucket name.
	Bucket string `json:"bucket"`
	// Key is the object key within the bucket.
	Key string `json:"key"`
	// Size is the object byte length.
	Size int64 `json:"size"`
	// ContentType is the MIME type of the object.
	ContentType string `json:"content_type"`
	// ETag is the MD5 hex digest of the object content.
	ETag string `json:"etag"`
	// LastModified is the UTC upload timestamp.
	LastModified time.Time `json:"last_modified"`
	// Metadata stores arbitrary user-defined key/value headers.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Store is the blob storage engine.
// It is safe for concurrent use by multiple goroutines.
type Store struct {
	mu         sync.RWMutex
	storageDir string
	maxObjSize int64
}

// NewStore creates a Store rooted at storageDir.
// maxObjSize limits individual object size (0 = no limit).
func NewStore(storageDir string, maxObjSize int64) (*Store, error) {
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("blob: mkdir storage: %w", err)
	}
	return &Store{storageDir: storageDir, maxObjSize: maxObjSize}, nil
}

// CreateBucket creates the bucket directory if it does not already exist.
func (s *Store) CreateBucket(bucket string) error {
	if err := validateName(bucket); err != nil {
		return err
	}
	return os.MkdirAll(s.bucketDir(bucket), 0755)
}

// DeleteBucket removes a bucket and all its objects.
// Returns an error if the bucket is not empty and force is false.
func (s *Store) DeleteBucket(bucket string, force bool) error {
	dir := s.bucketDir(bucket)
	if !force {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("blob: read bucket: %w", err)
		}
		// Count non-meta files.
		count := 0
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".meta") {
				count++
			}
		}
		if count > 0 {
			return fmt.Errorf("blob: bucket %q is not empty", bucket)
		}
	}
	return os.RemoveAll(dir)
}

// ListBuckets returns all existing bucket names.
func (s *Store) ListBuckets() ([]string, error) {
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		return nil, fmt.Errorf("blob: list buckets: %w", err)
	}
	var buckets []string
	for _, e := range entries {
		if e.IsDir() {
			buckets = append(buckets, e.Name())
		}
	}
	return buckets, nil
}

// PutObject writes an object to the store.
// The caller must close r after PutObject returns.
func (s *Store) PutObject(bucket, key, contentType string, r io.Reader, metadata map[string]string) (*ObjectMeta, error) {
	if err := validateName(bucket); err != nil {
		return nil, err
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}
	if err := s.CreateBucket(bucket); err != nil {
		return nil, err
	}

	objPath := s.objectPath(bucket, key)

	// Ensure parent directories for nested keys exist.
	if err := os.MkdirAll(filepath.Dir(objPath), 0755); err != nil {
		return nil, fmt.Errorf("blob: mkdir key dir: %w", err)
	}

	// Write to a temporary file first, then rename for atomicity.
	tmp := objPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return nil, fmt.Errorf("blob: create temp file: %w", err)
	}

	hasher := md5.New()
	var writer io.Writer = f
	if s.maxObjSize > 0 {
		writer = io.MultiWriter(f, hasher)
		r = io.LimitReader(r, s.maxObjSize+1)
	} else {
		writer = io.MultiWriter(f, hasher)
	}

	written, err := io.Copy(writer, r)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("blob: write object: %w", err)
	}
	if s.maxObjSize > 0 && written > s.maxObjSize {
		os.Remove(tmp)
		return nil, fmt.Errorf("blob: object exceeds max size (%d bytes)", s.maxObjSize)
	}

	if err := os.Rename(tmp, objPath); err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("blob: rename: %w", err)
	}

	etag := hex.EncodeToString(hasher.Sum(nil))
	meta := &ObjectMeta{
		Bucket:       bucket,
		Key:          key,
		Size:         written,
		ContentType:  contentType,
		ETag:         etag,
		LastModified: time.Now().UTC(),
		Metadata:     metadata,
	}
	if err := s.writeMeta(bucket, key, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

// GetObject returns a reader for the object data and its metadata.
// The caller is responsible for closing the returned ReadCloser.
func (s *Store) GetObject(bucket, key string) (io.ReadCloser, *ObjectMeta, error) {
	meta, err := s.HeadObject(bucket, key)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(s.objectPath(bucket, key))
	if err != nil {
		return nil, nil, fmt.Errorf("blob: open object: %w", err)
	}
	return f, meta, nil
}

// HeadObject returns only the metadata for an object without reading its body.
func (s *Store) HeadObject(bucket, key string) (*ObjectMeta, error) {
	meta, err := s.readMeta(bucket, key)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// DeleteObject removes an object and its metadata file.
func (s *Store) DeleteObject(bucket, key string) error {
	objPath := s.objectPath(bucket, key)
	metaPath := s.metaPath(bucket, key)
	if err := os.Remove(objPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("blob: delete object: %w", err)
	}
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("blob: delete meta: %w", err)
	}
	return nil
}

// ListObjects lists objects in a bucket with an optional prefix filter.
func (s *Store) ListObjects(bucket, prefix string) ([]*ObjectMeta, error) {
	dir := s.bucketDir(bucket)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blob: bucket %q not found", bucket)
		}
		return nil, fmt.Errorf("blob: list objects: %w", err)
	}

	var result []*ObjectMeta
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".meta") || strings.HasSuffix(path, ".tmp") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		meta, err := s.readMeta(bucket, key)
		if err != nil {
			return nil
		}
		result = append(result, meta)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("blob: list objects: %w", err)
	}
	return result, nil
}

// CopyObject copies an object from srcBucket/srcKey to dstBucket/dstKey.
func (s *Store) CopyObject(srcBucket, srcKey, dstBucket, dstKey string) (*ObjectMeta, error) {
	rc, srcMeta, err := s.GetObject(srcBucket, srcKey)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return s.PutObject(dstBucket, dstKey, srcMeta.ContentType, rc, srcMeta.Metadata)
}

// --- internal helpers --------------------------------------------------------

func (s *Store) bucketDir(bucket string) string {
	return filepath.Join(s.storageDir, bucket)
}

func (s *Store) objectPath(bucket, key string) string {
	// Replace slashes in key with OS separator for nested "directory" keys.
	return filepath.Join(s.storageDir, bucket, filepath.FromSlash(key))
}

func (s *Store) metaPath(bucket, key string) string {
	return s.objectPath(bucket, key) + ".meta"
}

func (s *Store) writeMeta(bucket, key string, meta *ObjectMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("blob: marshal meta: %w", err)
	}
	return os.WriteFile(s.metaPath(bucket, key), data, 0644)
}

func (s *Store) readMeta(bucket, key string) (*ObjectMeta, error) {
	data, err := os.ReadFile(s.metaPath(bucket, key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blob: object %s/%s not found", bucket, key)
		}
		return nil, fmt.Errorf("blob: read meta: %w", err)
	}
	var meta ObjectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("blob: unmarshal meta: %w", err)
	}
	return &meta, nil
}

// validateName checks that a bucket name is safe.
func validateName(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\..") {
		return fmt.Errorf("blob: invalid bucket name %q", name)
	}
	return nil
}

// validateKey checks that an object key is safe.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("blob: object key must not be empty")
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("blob: object key must not contain '..'")
	}
	return nil
}
