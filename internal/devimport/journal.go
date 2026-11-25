package devimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// Journal remembers which device files were delivered, so "New" is an instant
// lookup (relative path + size + mtime, no hashing). One file per device.
type Journal struct {
	path string
	lock sync.Mutex
	Seen map[string]bool `json:"seen"`
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9._=-]`)

func key(f File) string { return fmt.Sprintf("%s|%d|%d", f.Rel, f.Size, f.ModTime.Unix()) }

// DefaultJournalDir is ~/.config/photosort/imports (platform config dir).
func DefaultJournalDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "photosort", "imports"), nil
}

// OpenJournal loads the journal of a device (empty when there is none yet).
func OpenJournal(dir, device string) (*Journal, error) {
	j := &Journal{path: filepath.Join(dir, unsafeID.ReplaceAllString(device, "_")+".json"), Seen: map[string]bool{}}
	data, err := os.ReadFile(j.path)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, j); err != nil {
		return nil, fmt.Errorf("journal %s: %w", j.path, err)
	}
	if j.Seen == nil {
		j.Seen = map[string]bool{}
	}
	return j, nil
}

// Has reports whether f was already delivered.
func (j *Journal) Has(f File) bool {
	j.lock.Lock()
	defer j.lock.Unlock()
	return j.Seen[key(f)]
}

// Add records delivered files and saves the journal atomically.
func (j *Journal) Add(files []File) error {
	j.lock.Lock()
	defer j.lock.Unlock()
	for _, f := range files {
		j.Seen[key(f)] = true
	}
	return j.saveLocked()
}

// Forget empties the journal and deletes its file.
func (j *Journal) Forget() error {
	j.lock.Lock()
	defer j.lock.Unlock()
	j.Seen = map[string]bool{}
	if err := os.Remove(j.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (j *Journal) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(j.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, j.path)
}
