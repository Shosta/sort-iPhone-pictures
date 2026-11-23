package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// IndexName is the fingerprint index kept at the root of the remote library.
const IndexName = ".photosort-index.json"

// DirEntry is a sub-folder (or module) of a remote folder.
type DirEntry struct {
	Name string `json:"name"`
	// Module marks a daemon module (top level); Comment is its description.
	Module  bool   `json:"module,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// Listing is the content of a remote folder.
type Listing struct {
	// Path is the folder listed (relative to the module for a daemon).
	Path string     `json:"path"`
	Dirs []DirEntry `json:"dirs"`
	// Files is the number of files (not shown) in the folder.
	Files int `json:"files"`
	// Missing is set when the folder does not exist.
	Missing  bool     `json:"missing,omitempty"`
	Tool     string   `json:"tool,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// RemoteFile is a file of the remote library.
type RemoteFile struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

type listLine struct {
	name    string
	dir     bool
	size    int64
	modTime time.Time
}

var listLinePattern = regexp.MustCompile(`^([-dlcbps])[rwxsStT-]{9}\s+([\d,.]+)\s+(\d{4}/\d\d/\d\d)\s+(\d\d:\d\d:\d\d)\s+(.+)$`)

// parseList reads one --list-only line. ok is false for blank lines and ".".
func parseList(line string) (listLine, bool) {
	line = strings.TrimRight(line, "\r\n")
	match := listLinePattern.FindStringSubmatch(strings.TrimSpace(line))
	if match == nil {
		return listLine{}, false
	}
	name := match[5]
	if match[1] == "l" {
		name, _, _ = strings.Cut(name, " -> ")
	}
	if name == "." || name == "" {
		return listLine{}, false
	}
	size, _ := strconv.ParseInt(strings.NewReplacer(",", "", ".", "").Replace(match[2]), 10, 64)
	when, _ := time.ParseInLocation("2006/01/02 15:04:05", match[3]+" "+match[4], time.Local)
	return listLine{name: name, dir: match[1] == "d", size: size, modTime: when}, true
}

// session prepares a tool and its shared options for one operation.
func (r Rsync) session(ctx context.Context, cfg Config) (Tool, []string, func(), error) {
	tool, err := r.tool(ctx)
	if err != nil {
		return Tool{}, nil, nil, err
	}
	conn, cleanup, err := tool.connection(cfg)
	if err != nil {
		return Tool{}, nil, nil, err
	}
	return tool, conn, cleanup, nil
}

func cleanDir(dir string) (string, error) {
	dir = strings.Trim(strings.ReplaceAll(strings.TrimSpace(dir), "\\", "/"), "/")
	if hasDotDot(dir) || strings.HasPrefix(dir, "-") {
		return "", configError("invalid path")
	}
	return dir, nil
}

// hiddenFolder reports folders a NAS creates for itself (recycle bin, thumbnails).
func hiddenFolder(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") || strings.EqualFold(name, "lost+found")
}

// List implements Transport.
func (r Rsync) List(ctx context.Context, cfg Config, dir string) (Listing, error) {
	dir, err := cleanDir(dir)
	if err != nil {
		return Listing{}, err
	}
	cfg.Path = dir
	if cfg.Protocol == SSH && dir != "" {
		cfg.Path = "/" + dir
	}
	if err := cfg.ValidateBrowse(); err != nil {
		return Listing{}, err
	}
	tool, conn, cleanup, err := r.session(ctx, cfg)
	if err != nil {
		return Listing{}, err
	}
	defer cleanup()
	listing := Listing{Path: dir, Dirs: []DirEntry{}, Tool: tool.Path + " " + tool.Version, Warnings: tool.warnings()}
	modules := cfg.Protocol == Daemon && cfg.Module == ""
	args := append([]string{"--list-only", "-d", "-8"}, conn...)
	err = r.run(ctx, tool.Path, append(args, destination(cfg)), cfg.Password, func(line string) {
		if modules {
			// Module lines are "name<TAB>comment".
			name, comment, _ := strings.Cut(line, "\t")
			name = strings.TrimSpace(name)
			if name != "" && !strings.ContainsAny(name, " /") {
				listing.Dirs = append(listing.Dirs, DirEntry{Name: name, Module: true, Comment: strings.TrimSpace(comment)})
			}
			return
		}
		entry, ok := parseList(line)
		switch {
		case !ok:
		case entry.dir && !hiddenFolder(entry.name):
			listing.Dirs = append(listing.Dirs, DirEntry{Name: entry.name})
		case !entry.dir:
			listing.Files++
		}
	})
	if kind, _, ok := KindOf(err); ok && kind == KindPath {
		listing.Missing = true
		return listing, nil
	}
	if err != nil {
		return listing, err
	}
	sort.Slice(listing.Dirs, func(i, j int) bool {
		return strings.ToLower(listing.Dirs[i].Name) < strings.ToLower(listing.Dirs[j].Name)
	})
	return listing, nil
}

// ListTree implements Transport. A library folder that does not exist yet is
// simply empty.
func (r Rsync) ListTree(ctx context.Context, cfg Config) ([]RemoteFile, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	tool, conn, cleanup, err := r.session(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	files := []RemoteFile{}
	args := append([]string{"--list-only", "-r", "-8"}, conn...)
	err = r.run(ctx, tool.Path, append(args, destination(cfg)), cfg.Password, func(line string) {
		if entry, ok := parseList(line); ok && !entry.dir && !hiddenName(entry.name) {
			files = append(files, RemoteFile{Path: normalize(entry.name), Size: entry.size, ModTime: entry.modTime})
		}
	})
	if kind, _, ok := KindOf(err); ok && kind == KindPath {
		return []RemoteFile{}, nil
	}
	return files, err
}

// hiddenName is true for files under a NAS-internal or partial-transfer folder.
func hiddenName(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, "@") || part == ".rsync-partial" {
			return true
		}
	}
	return false
}

// FetchIndex implements Transport.
func (r Rsync) FetchIndex(ctx context.Context, cfg Config) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	tool, conn, cleanup, err := r.session(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	dir, err := os.MkdirTemp("", "photosort-index-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	args := append([]string{"-t", "-8"}, conn...)
	err = r.run(ctx, tool.Path, append(args, destination(cfg)+IndexName, localArg(dir)), cfg.Password, func(string) {})
	if kind, _, ok := KindOf(err); ok && kind == KindPath {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, IndexName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// StoreIndex implements Transport.
func (r Rsync) StoreIndex(ctx context.Context, cfg Config, data []byte) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	tool, conn, cleanup, err := r.session(ctx, cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	dir, err := os.MkdirTemp("", "photosort-index-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	local := filepath.Join(dir, IndexName)
	if err := os.WriteFile(local, data, 0o600); err != nil {
		return err
	}
	args := []string{"-t", "-8"}
	if tool.MkPath {
		args = append(args, "--mkpath")
	}
	args = append(args, conn...)
	return r.run(ctx, tool.Path, append(args, local, destination(cfg)+IndexName), cfg.Password, func(string) {})
}
