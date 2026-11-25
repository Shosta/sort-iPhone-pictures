package devimport

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultRoots are the places where Linux mounts phones and cards. Other
// systems return none: the user picks the source manually.
func DefaultRoots() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	roots := []string{fmt.Sprintf("/run/user/%d/gvfs", os.Getuid())}
	if u, err := user.Current(); err == nil {
		roots = append(roots, filepath.Join("/media", u.Username), filepath.Join("/run/media", u.Username))
	}
	return append(roots, "/media")
}

// Detect finds mounted volumes with a DCIM folder directly below them (or one
// level deeper for GVFS mounts that expose several storages).
func Detect(roots []string) []Device {
	var devices []Device
	seen := map[string]bool{}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			dcim := findDCIM(filepath.Join(root, entry.Name()))
			if dcim == "" || seen[dcim] {
				continue
			}
			seen[dcim] = true
			devices = append(devices, describe(entry.Name(), dcim))
		}
	}
	return devices
}

func findDCIM(mount string) string {
	if dcim := filepath.Join(mount, "DCIM"); isDir(dcim) {
		return dcim
	}
	entries, err := os.ReadDir(mount)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if dcim := filepath.Join(mount, entry.Name(), "DCIM"); isDir(dcim) {
			return dcim
		}
	}
	return ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// describe derives a stable id and a friendly name from the mount name, e.g.
// "afc:host=00008030-001A" gives id "afc-00008030-001A" and name "iPhone".
func describe(mountName, dcim string) Device {
	id, name := mountName, mountName
	if strings.HasPrefix(mountName, "afc:host=") {
		host, _, _ := strings.Cut(strings.TrimPrefix(mountName, "afc:host="), ",")
		id, name = "afc-"+host, "iPhone"
	}
	return Device{ID: id, Name: name, Path: dcim}
}
