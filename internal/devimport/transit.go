package devimport

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

// TransitMargin is free space kept beyond the batches themselves.
const TransitMargin = 64 << 20

// transitCandidates are the folders where batches are staged, in order.
func transitCandidates() []string {
	name := "photosort"
	if u, err := user.Current(); err == nil {
		name += "-" + u.Username
	}
	list := []string{filepath.Join(os.TempDir(), name)}
	if dir, err := os.UserCacheDir(); err == nil {
		list = append(list, filepath.Join(dir, "photosort", "import"))
	}
	return list
}

// PrepareTransit returns an empty private transit folder with at least need
// bytes free: the temp folder, else the user cache. Anything left over from an
// earlier run is removed silently.
func PrepareTransit(need int64) (string, error) {
	return prepareTransit(transitCandidates(), need)
}

func prepareTransit(candidates []string, need int64) (string, error) {
	var lastErr error
	for _, dir := range candidates {
		if err := securePrivateDir(dir); err != nil {
			lastErr = err
			continue
		}
		if err := emptyDir(dir); err != nil {
			lastErr = err
			continue
		}
		if !EnoughSpace(dir, need) {
			lastErr = &Error{Code: CodeLowDisk, Detail: dir}
			continue
		}
		return dir, nil
	}
	if lastErr == nil {
		lastErr = &Error{Code: CodeLowDisk}
	}
	return "", lastErr
}

// CleanTransit empties the transit folders (best effort).
func CleanTransit() {
	for _, dir := range transitCandidates() {
		if securePrivateDir(dir) == nil {
			emptyDir(dir)
		}
	}
}

// EnoughSpace reports whether dir can still take need bytes plus a margin.
func EnoughSpace(dir string, need int64) bool {
	free, ok := freeSpace(dir)
	return !ok || int64(free) >= need+TransitMargin
}

// securePrivateDir creates dir with mode 0700, or checks that an existing one
// is a real directory (no symlink) owned by the current user.
func securePrivateDir(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a plain directory", dir)
	}
	if !ownedByCurrentUser(info) {
		return fmt.Errorf("%s belongs to another user", dir)
	}
	return os.Chmod(dir, 0o700)
}

func emptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
