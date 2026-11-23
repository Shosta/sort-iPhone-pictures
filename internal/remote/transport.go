// Package remote sends a sorted photo tree to a remote machine (a NAS).
//
// The sorting engine computes destination paths that file-sync tools cannot
// produce, so the photos are first "staged": a local tree of links (no data is
// copied) laid out exactly as the remote library should look. A Transport then
// uploads that tree and never overwrites what already exists remotely.
// Integrity and resumption are left to the transport (rsync today).
package remote

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Protocol string

const (
	// Daemon talks to an rsync daemon (module + password), e.g. the QNAP
	// "rsync server".
	Daemon Protocol = "daemon"
	// SSH runs rsync over SSH (key or agent authentication).
	SSH Protocol = "ssh"
)

// Config describes a remote library. Password is never persisted or logged.
type Config struct {
	Protocol Protocol `json:"protocol"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	User     string   `json:"user"`
	// Module is the rsync daemon module (Daemon only).
	Module string `json:"module"`
	// Path is a folder inside the module (Daemon) or the remote folder (SSH).
	Path     string `json:"path"`
	Password string `json:"password,omitempty"`
	// KeyFile is an optional SSH private key (SSH only).
	KeyFile string `json:"keyFile,omitempty"`
	// Checksum verifies, once the upload is done, that the files already
	// present remotely are byte-identical to the local ones.
	Checksum bool `json:"checksum"`
}

// Item is a photo to upload: Source is the local file, Target its path relative
// to the remote library root (slash or OS separated).
type Item struct {
	Source string
	Target string
	Size   int64
	// ModTime is the modification time of Source (rsync keeps it with -t).
	ModTime time.Time
	// Sidecar marks a tag file (.xmp) sent next to its photo; it is counted
	// apart from the photos.
	Sidecar bool
}

// Progress is a snapshot of a running upload.
type Progress struct {
	Percent float64 `json:"percent"`
	Bytes   int64   `json:"bytes"`
	Rate    string  `json:"rate,omitempty"`
	ETA     string  `json:"eta,omitempty"`
	Done    int     `json:"done"`
	Total   int     `json:"total"`
}

// Job is a staged tree ready to upload.
type Job struct {
	// Dir is the staged tree; its content lands in the remote library root.
	Dir   string
	Items []Item
	// DryRun lists what would be sent without sending anything.
	DryRun bool
	// OnSent is called when an item has been sent (or, for a dry run, would be).
	OnSent     func(Item)
	OnProgress func(Progress)
}

// Result summarises an upload.
type Result struct {
	DryRun bool `json:"dryRun"`
	// Sent are the targets uploaded (or that would be, for a dry run).
	Sent []string `json:"sent"`
	// SentBytes is the size of Sent.
	SentBytes int64 `json:"sentBytes"`
	// Existing is the number of items already present remotely and left untouched.
	Existing int `json:"existing"`
	// Tags is the number of tag files (.xmp) sent, TagsExisting the number left
	// alone because the NAS already had one (never overwritten).
	Tags         int `json:"tags"`
	TagsExisting int `json:"tagsExisting"`
	// Mismatched lists remote files that already existed with different content
	// (only filled when Config.Checksum is set); they are left untouched.
	Mismatched []string `json:"mismatched,omitempty"`
	// Warnings are machine-readable codes (e.g. "legacyRsync").
	Warnings []string `json:"warnings,omitempty"`
}

// TestResult is the outcome of a connection test.
type TestResult struct {
	// Entries is the number of entries listed at the destination.
	Entries int      `json:"entries"`
	Sample  []string `json:"sample"`
	// PathMissing is set when the folder does not exist yet (it will be created).
	PathMissing bool     `json:"pathMissing"`
	Tool        string   `json:"tool"`
	Warnings    []string `json:"warnings,omitempty"`
}

// Transport uploads staged trees. Other protocols can be added by
// implementing it.
type Transport interface {
	Name() string
	Test(ctx context.Context, cfg Config) (TestResult, error)
	Upload(ctx context.Context, cfg Config, job Job) (Result, error)
	// List returns the sub-folders of a folder of the remote server; with no
	// module (daemon) it lists the modules. Nothing is written.
	List(ctx context.Context, cfg Config, dir string) (Listing, error)
	// ListTree lists every file of the remote library (cfg.Path), recursively.
	ListTree(ctx context.Context, cfg Config) ([]RemoteFile, error)
	// FetchIndex reads the fingerprint index kept in the library (nil if none).
	FetchIndex(ctx context.Context, cfg Config) ([]byte, error)
	// StoreIndex writes the fingerprint index into the library: the only file
	// that is ever replaced remotely.
	StoreIndex(ctx context.Context, cfg Config, data []byte) error
}

// Validate checks the configuration and fills defaults.
func (c *Config) Validate() error { return c.validate(false) }

// ValidateBrowse is Validate for folder browsing: a daemon needs no module yet
// (the modules themselves are listed).
func (c *Config) ValidateBrowse() error { return c.validate(true) }

func (c *Config) validate(browse bool) error {
	c.Host = strings.TrimSpace(c.Host)
	c.User = strings.TrimSpace(c.User)
	c.Module = strings.Trim(strings.TrimSpace(c.Module), "/")
	c.Path = strings.TrimSpace(c.Path)
	if c.Protocol == "" {
		c.Protocol = Daemon
	}
	if c.Protocol != Daemon && c.Protocol != SSH {
		return configError("unknown protocol %q", c.Protocol)
	}
	if c.Host == "" || strings.ContainsAny(c.Host, " /\\@") || strings.HasPrefix(c.Host, "-") {
		return configError("invalid host")
	}
	if c.Port < 0 || c.Port > 65535 {
		return configError("invalid port")
	}
	if c.User != "" && (strings.ContainsAny(c.User, " /\\@:") || strings.HasPrefix(c.User, "-")) {
		return configError("invalid user")
	}
	switch c.Protocol {
	case Daemon:
		if c.Module == "" && browse {
			c.Path = ""
			break
		}
		if c.Module == "" || strings.ContainsAny(c.Module, " /\\@:") || strings.HasPrefix(c.Module, "-") {
			return configError("a module name is required")
		}
		c.Path = strings.Trim(c.Path, "/")
		if hasDotDot(c.Path) {
			return configError("invalid path")
		}
	case SSH:
		if (c.Path == "" && !browse) || strings.HasPrefix(c.Path, "-") {
			return configError("a remote folder is required")
		}
	}
	return nil
}

func hasDotDot(path string) bool {
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

// URL is a display form of the destination, without any secret.
func (c Config) URL() string {
	userinfo := ""
	if c.User != "" {
		userinfo = c.User + "@"
	}
	host := c.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if c.Port > 0 {
		host += ":" + strconv.Itoa(c.Port)
	}
	if c.Protocol == SSH {
		return "ssh://" + userinfo + host + "/" + strings.TrimPrefix(c.Path, "/")
	}
	return strings.TrimRight("rsync://"+userinfo+host+"/"+c.Module+"/"+c.Path, "/")
}

// ParseURL reads rsync://user@host[:port]/module[/path] (daemon) or
// ssh://user@host[:port]/remote/path (SSH). No password is accepted in a URL.
func ParseURL(raw string) (Config, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Config{}, configError("invalid remote URL")
	}
	cfg := Config{Host: u.Hostname(), User: u.User.Username()}
	if _, has := u.User.Password(); has {
		return Config{}, configError("do not put the password in the URL")
	}
	if port := u.Port(); port != "" {
		if cfg.Port, err = strconv.Atoi(port); err != nil {
			return Config{}, configError("invalid port")
		}
	}
	switch u.Scheme {
	case "rsync":
		cfg.Protocol = Daemon
		module, path, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
		cfg.Module, cfg.Path = module, path
	case "ssh":
		cfg.Protocol = SSH
		cfg.Path = u.Path
	default:
		return Config{}, configError("remote URL must start with rsync:// or ssh://")
	}
	return cfg, cfg.Validate()
}

func configError(format string, args ...any) error {
	return &Error{Kind: KindConfig, Detail: fmt.Sprintf(format, args...)}
}
