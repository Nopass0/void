package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/voiddb/void/internal/config"
	"github.com/voiddb/void/internal/engine"
	"github.com/voiddb/void/internal/engine/types"
)

// Settings is the runtime-manageable backup configuration exposed over the API.
type Settings struct {
	Dir          string `json:"dir"`
	Retain       int    `json:"retain"`
	ScheduleCron string `json:"schedule_cron"`
	NextRun      string `json:"next_run,omitempty"`
}

// FileInfo is metadata for a stored backup archive.
type FileInfo struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
}

// Service manages scheduled and on-demand backup archives stored on disk.
type Service struct {
	mu            sync.RWMutex
	store         *engine.Store
	cfg           *config.Config
	configPath    string
	serverVersion string
	cron          *cron.Cron
	entryID       cron.EntryID
	running       bool
}

// NewService creates a backup management service bound to the live server config.
func NewService(store *engine.Store, cfg *config.Config, configPath, serverVersion string) (*Service, error) {
	svc := &Service{
		store:         store,
		cfg:           cfg,
		configPath:    configPath,
		serverVersion: serverVersion,
		cron:          cron.New(),
	}

	if err := os.MkdirAll(cfg.Backup.Dir, 0755); err != nil {
		return nil, err
	}

	if err := svc.applyScheduleLocked(); err != nil {
		return nil, err
	}

	svc.cron.Start()
	return svc, nil
}

// Close stops the scheduler.
func (s *Service) Close() {
	if s == nil || s.cron == nil {
		return
	}
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// GetSettings returns the current backup settings with the computed next run time.
func (s *Service) GetSettings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	settings := Settings{
		Dir:          s.cfg.Backup.Dir,
		Retain:       s.cfg.Backup.Retain,
		ScheduleCron: s.cfg.Backup.ScheduleCron,
	}
	if s.entryID != 0 {
		if next := s.cron.Entry(s.entryID).Next; !next.IsZero() {
			settings.NextRun = next.UTC().Format(time.RFC3339)
		}
	}
	return settings
}

// UpdateSettings validates, persists, and activates new backup settings.
func (s *Service) UpdateSettings(settings Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := strings.TrimSpace(settings.Dir)
	if dir == "" {
		dir = s.cfg.Backup.Dir
	}
	schedule := strings.TrimSpace(settings.ScheduleCron)
	if schedule != "" {
		if _, err := cron.ParseStandard(schedule); err != nil {
			return Settings{}, fmt.Errorf("invalid cron schedule: %w", err)
		}
	}
	if settings.Retain < 0 {
		return Settings{}, fmt.Errorf("retain must be >= 0")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Settings{}, err
	}

	prev := s.cfg.Backup
	s.cfg.Backup.Dir = dir
	s.cfg.Backup.Retain = settings.Retain
	s.cfg.Backup.ScheduleCron = schedule

	if err := s.cfg.Save(s.configPath); err != nil {
		s.cfg.Backup = prev
		return Settings{}, err
	}

	if err := s.applyScheduleLocked(); err != nil {
		s.cfg.Backup = prev
		_ = s.cfg.Save(s.configPath)
		_ = s.applyScheduleLocked()
		return Settings{}, err
	}

	if err := s.pruneLocked(); err != nil {
		zap.L().Warn("backup prune after settings update failed", zap.Error(err))
	}

	return s.snapshotLocked(), nil
}

// ListFiles returns all stored .void archives sorted newest-first.
func (s *Service) ListFiles() ([]FileInfo, error) {
	s.mu.RLock()
	dir := s.cfg.Backup.Dir
	s.mu.RUnlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".void") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:       entry.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt > files[j].ModifiedAt
	})
	return files, nil
}

// OpenFile opens a stored backup archive by name for download.
func (s *Service) OpenFile(name string) (*os.File, os.FileInfo, error) {
	s.mu.RLock()
	dir := s.cfg.Backup.Dir
	s.mu.RUnlock()

	path, err := validateBackupPath(dir, name)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

// DeleteFile removes a stored backup archive.
func (s *Service) DeleteFile(name string) error {
	s.mu.RLock()
	dir := s.cfg.Backup.Dir
	s.mu.RUnlock()

	path, err := validateBackupPath(dir, name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// CreateBackup writes a fresh archive to the configured backup directory.
func (s *Service) CreateBackup(databases []string) (FileInfo, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return FileInfo{}, fmt.Errorf("backup is already running")
	}
	s.running = true
	dir := s.cfg.Backup.Dir
	retain := s.cfg.Backup.Retain
	version := s.serverVersion
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return FileInfo{}, err
	}

	filename := fmt.Sprintf("voiddb_backup_%s.void", time.Now().UTC().Format("20060102_150405_000"))
	path := filepath.Join(dir, filename)
	writer, file, err := CreateBackupFile(path, version)
	if err != nil {
		return FileInfo{}, err
	}

	if err := s.writeBackup(writer, databases); err != nil {
		file.Close()
		_ = os.Remove(path)
		return FileInfo{}, err
	}
	if err := writer.Close(); err != nil {
		file.Close()
		_ = os.Remove(path)
		return FileInfo{}, err
	}
	if err := file.Close(); err != nil {
		return FileInfo{}, err
	}

	s.mu.Lock()
	if retain > 0 {
		if err := s.pruneLocked(); err != nil {
			zap.L().Warn("backup prune failed", zap.Error(err))
		}
	}
	s.mu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		Name:       filename,
		Size:       info.Size(),
		ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) writeBackup(writer *Writer, databases []string) error {
	dbNames := s.store.ListDatabases()
	if len(databases) > 0 {
		keep := make(map[string]struct{}, len(databases))
		for _, name := range databases {
			keep[name] = struct{}{}
		}
		filtered := make([]string, 0, len(databases))
		for _, name := range dbNames {
			if _, ok := keep[name]; ok {
				filtered = append(filtered, name)
			}
		}
		dbNames = filtered
	}

	for _, dbName := range dbNames {
		writer.AddDatabase(dbName)
		db := s.store.DB(dbName)
		for _, colName := range s.store.ListCollections(dbName) {
			col := db.Collection(colName)
			docs, err := col.Find(nil)
			if err != nil {
				continue
			}
			raw := make([]map[string]interface{}, 0, len(docs))
			for _, doc := range docs {
				m := make(map[string]interface{}, len(doc.Fields)+1)
				m["_id"] = doc.ID
				for k, v := range doc.Fields {
					m[k] = valueToJSONInterface(v)
				}
				raw = append(raw, m)
			}
			if err := writer.WriteCollection(dbName, colName, raw); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Service) snapshotLocked() Settings {
	settings := Settings{
		Dir:          s.cfg.Backup.Dir,
		Retain:       s.cfg.Backup.Retain,
		ScheduleCron: s.cfg.Backup.ScheduleCron,
	}
	if s.entryID != 0 {
		if next := s.cron.Entry(s.entryID).Next; !next.IsZero() {
			settings.NextRun = next.UTC().Format(time.RFC3339)
		}
	}
	return settings
}

func (s *Service) applyScheduleLocked() error {
	if s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.entryID = 0
	}

	spec := strings.TrimSpace(s.cfg.Backup.ScheduleCron)
	if spec == "" {
		return nil
	}

	if _, err := cron.ParseStandard(spec); err != nil {
		return err
	}

	entryID, err := s.cron.AddFunc(spec, func() {
		if _, err := s.CreateBackup(nil); err != nil {
			zap.L().Error("scheduled backup failed", zap.Error(err))
			return
		}
		zap.L().Info("scheduled backup created")
	})
	if err != nil {
		return err
	}
	s.entryID = entryID
	return nil
}

func (s *Service) pruneLocked() error {
	if s.cfg.Backup.Retain <= 0 {
		return nil
	}

	files, err := s.listFilesLocked()
	if err != nil {
		return err
	}
	if len(files) <= s.cfg.Backup.Retain {
		return nil
	}

	for _, file := range files[s.cfg.Backup.Retain:] {
		if err := os.Remove(filepath.Join(s.cfg.Backup.Dir, file.Name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *Service) listFilesLocked() ([]FileInfo, error) {
	entries, err := os.ReadDir(s.cfg.Backup.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".void") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:       entry.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt > files[j].ModifiedAt
	})
	return files, nil
}

func validateBackupPath(dir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || strings.Contains(name, "..") || !strings.HasSuffix(name, ".void") {
		return "", fmt.Errorf("invalid backup file name")
	}
	return filepath.Join(dir, name), nil
}

func valueToJSONInterface(v types.Value) interface{} {
	switch v.Type() {
	case types.TypeNull:
		return nil
	case types.TypeString:
		return v.StringVal()
	case types.TypeNumber:
		return v.NumberVal()
	case types.TypeBoolean:
		return v.BoolVal()
	case types.TypeArray:
		arr := v.ArrayVal()
		out := make([]interface{}, len(arr))
		for i, item := range arr {
			out[i] = valueToJSONInterface(item)
		}
		return out
	case types.TypeObject:
		obj := v.ObjectVal()
		out := make(map[string]interface{}, len(obj))
		for k, val := range obj {
			out[k] = valueToJSONInterface(val)
		}
		return out
	case types.TypeBlob:
		bucket, key := v.BlobRef()
		return map[string]string{"_blob_bucket": bucket, "_blob_key": key}
	}
	return nil
}
