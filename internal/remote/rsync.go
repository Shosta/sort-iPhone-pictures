package remote

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Tool describes the rsync executable in use.
type Tool struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	// Info2 means --info=progress2 (overall progress) is available.
	Info2 bool `json:"info2"`
	// MkPath means --mkpath (create missing parent folders) is available.
	MkPath bool `json:"mkpath"`
	// OpenRsync is the limited BSD implementation shipped with macOS.
	OpenRsync bool `json:"openrsync"`
}

var versionPattern = regexp.MustCompile(`version (\d+)\.(\d+)(?:\.(\d+))?`)

// Probe runs "path --version" and fills the capabilities.
func Probe(ctx context.Context, binary string) (Tool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput()
	text := string(out)
	if err != nil && !strings.Contains(text, "rsync") {
		return Tool{}, err
	}
	tool := Tool{Path: binary, OpenRsync: strings.Contains(strings.ToLower(text), "openrsync")}
	if match := versionPattern.FindStringSubmatch(text); match != nil {
		tool.Version = strings.TrimSuffix(strings.Join(match[1:], "."), ".")
		major, _ := strconv.Atoi(match[1])
		minor, _ := strconv.Atoi(match[2])
		patch, _ := strconv.Atoi(match[3])
		if !tool.OpenRsync {
			tool.Info2 = major > 3 || (major == 3 && minor >= 1)
			tool.MkPath = major > 3 || (major == 3 && (minor > 2 || (minor == 2 && patch >= 3)))
		}
	}
	return tool, nil
}

// candidates lists rsync executables by preference. macOS ships openrsync as
// /usr/bin/rsync, which lacks modern options, so Homebrew builds come first.
func candidates() []string {
	var list []string
	if runtime.GOOS == "darwin" {
		list = append(list, "/opt/homebrew/bin/rsync", "/usr/local/bin/rsync")
	}
	if found, err := exec.LookPath("rsync"); err == nil {
		list = append(list, found)
	}
	if runtime.GOOS == "darwin" {
		list = append(list, "/usr/bin/rsync")
	}
	return list
}

// FindRsync picks the best rsync available.
func FindRsync(ctx context.Context) (Tool, error) {
	var fallback *Tool
	for _, candidate := range candidates() {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		tool, err := Probe(ctx, candidate)
		if err != nil {
			continue
		}
		if tool.Info2 {
			return tool, nil
		}
		if fallback == nil {
			fallback = &tool
		}
	}
	if fallback != nil {
		return *fallback, nil
	}
	return Tool{}, &Error{Kind: KindNotFound}
}

// Rsync is the rsync transport.
type Rsync struct {
	// Path forces an executable (tests); empty means FindRsync.
	Path string
}

func (r Rsync) Name() string { return "rsync" }

func (r Rsync) tool(ctx context.Context) (Tool, error) {
	if r.Path != "" {
		tool, err := Probe(ctx, r.Path)
		if err != nil {
			return Tool{}, &Error{Kind: KindNotFound, Detail: err.Error()}
		}
		return tool, nil
	}
	return FindRsync(ctx)
}

func (t Tool) warnings() []string {
	if t.Info2 {
		return nil
	}
	return []string{"legacyRsync"}
}

// connection returns the options shared by every command and a cleanup for
// the temporary password file.
func (t Tool) connection(cfg Config) (opts []string, cleanup func(), err error) {
	cleanup = func() {}
	opts = []string{"--timeout=60"}
	switch cfg.Protocol {
	case Daemon:
		opts = append(opts, "--no-motd")
		if !t.OpenRsync {
			opts = append(opts, "--contimeout=15")
		}
		if cfg.Password != "" {
			file, err := os.CreateTemp("", "photosort-rsync-*")
			if err != nil {
				return nil, cleanup, err
			}
			cleanup = func() { os.Remove(file.Name()) }
			if err := file.Chmod(0o600); err != nil {
				file.Close()
				cleanup()
				return nil, func() {}, err
			}
			_, werr := file.WriteString(cfg.Password + "\n")
			if cerr := errors.Join(werr, file.Close()); cerr != nil {
				cleanup()
				return nil, func() {}, cerr
			}
			opts = append(opts, "--password-file="+file.Name())
		}
	case SSH:
		ssh := "ssh -o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new"
		if cfg.Port > 0 {
			ssh += " -p " + strconv.Itoa(cfg.Port)
		}
		if cfg.KeyFile != "" {
			ssh += " -i " + strconv.Quote(cfg.KeyFile)
		}
		opts = append(opts, "-e", ssh)
	default:
		return nil, cleanup, configError("unknown protocol %q", cfg.Protocol)
	}
	return opts, cleanup, nil
}

// destination is the rsync destination argument of a (validated) config.
func destination(cfg Config) string {
	if cfg.Protocol == SSH {
		host := cfg.Host
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		if cfg.User != "" {
			host = cfg.User + "@" + host
		}
		return strings.TrimRight(host+":"+cfg.Path, "/") + "/"
	}
	return strings.TrimRight(cfg.URL(), "/") + "/"
}

func localArg(dir string) string {
	if runtime.GOOS == "windows" {
		// cwRsync and similar builds expect Cygwin-style paths.
		slashed := filepath.ToSlash(dir)
		if len(slashed) >= 2 && slashed[1] == ':' {
			slashed = "/cygdrive/" + strings.ToLower(slashed[:1]) + slashed[2:]
		}
		dir = slashed
	}
	return strings.TrimRight(dir, `/\`) + "/"
}

func (t Tool) uploadArgs(cfg Config, job Job, conn []string, dest string) []string {
	// Times are kept; permissions and owners are not (a NAS user rarely
	// matches the local one, which only produces errors).
	args := []string{"-rLt", "--partial-dir=.rsync-partial", "--ignore-existing", "--out-format=%n", "-8", "--exclude=" + StageMarker}
	if !t.OpenRsync {
		args = append(args, "--chmod=D755,F644")
	}
	if t.MkPath {
		args = append(args, "--mkpath")
	}
	if job.DryRun {
		args = append(args, "--dry-run")
	} else if t.Info2 {
		args = append(args, "--info=progress2")
	} else {
		args = append(args, "--progress")
	}
	args = append(args, conn...)
	return append(args, localArg(job.Dir), dest)
}

func (t Tool) verifyArgs(job Job, conn []string, dest string) []string {
	args := []string{"-rLn", "--checksum", "--out-format=%n", "-8", "--exclude=" + StageMarker}
	args = append(args, conn...)
	return append(args, localArg(job.Dir), dest)
}

var progressPattern = regexp.MustCompile(`^\s*([\d,.]+)\s+(\d+)%\s+(\S+)\s+(\d+:\d+:\d+)`)

func normalize(name string) string {
	return strings.TrimPrefix(strings.TrimRight(filepath.ToSlash(strings.TrimSpace(name)), "/"), "./")
}

// Upload sends the staged tree; existing remote files are never rewritten.
func (r Rsync) Upload(ctx context.Context, cfg Config, job Job) (Result, error) {
	if err := cfg.Validate(); err != nil {
		return Result{}, err
	}
	tool, err := r.tool(ctx)
	if err != nil {
		return Result{}, err
	}
	conn, cleanup, err := tool.connection(cfg)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	dest := destination(cfg)
	byName := make(map[string]Item, len(job.Items))
	for _, item := range job.Items {
		byName[normalize(item.Target)] = item
	}
	result := Result{DryRun: job.DryRun, Sent: []string{}, Warnings: tool.warnings()}
	photos, tags := 0, 0
	for _, item := range job.Items {
		if item.Sidecar {
			tags++
		} else {
			photos++
		}
	}
	if !tool.MkPath {
		if err := r.checkParent(ctx, tool, cfg, conn); err != nil {
			return result, err
		}
	}
	seen := map[string]bool{}
	var bytesDone int64
	emit := func(p Progress) {
		if job.OnProgress != nil {
			p.Total = photos
			job.OnProgress(p)
		}
	}
	err = r.run(ctx, tool.Path, tool.uploadArgs(cfg, job, conn, dest), cfg.Password, func(line string) {
		if item, ok := byName[normalize(line)]; ok {
			if !seen[normalize(line)] {
				seen[normalize(line)] = true
				if item.Sidecar {
					result.Tags++
					return
				}
				result.Sent = append(result.Sent, item.Target)
				result.SentBytes += item.Size
				bytesDone += item.Size
				if job.OnSent != nil {
					job.OnSent(item)
				}
				if !tool.Info2 {
					emit(Progress{Done: len(result.Sent), Bytes: bytesDone, Percent: 100 * float64(len(result.Sent)) / float64(max(1, photos))})
				}
			}
			return
		}
		if match := progressPattern.FindStringSubmatch(line); match != nil && tool.Info2 && !job.DryRun {
			percent, _ := strconv.ParseFloat(match[2], 64)
			done, _ := strconv.ParseInt(strings.NewReplacer(",", "", ".", "").Replace(match[1]), 10, 64)
			emit(Progress{Percent: percent, Bytes: done, Rate: match[3], ETA: match[4], Done: len(result.Sent)})
		}
	})
	if err != nil {
		return result, err
	}
	result.Existing = photos - len(result.Sent)
	result.TagsExisting = tags - result.Tags
	if cfg.Checksum && !job.DryRun {
		mismatched, err := r.verify(ctx, tool, cfg, job, conn, dest, byName)
		if err != nil {
			return result, err
		}
		result.Mismatched = mismatched
	}
	return result, nil
}

func (r Rsync) verify(ctx context.Context, tool Tool, cfg Config, job Job, conn []string, dest string, byName map[string]Item) ([]string, error) {
	var mismatched []string
	err := r.run(ctx, tool.Path, tool.verifyArgs(job, conn, dest), cfg.Password, func(line string) {
		if item, ok := byName[normalize(line)]; ok {
			mismatched = append(mismatched, item.Target)
		}
	})
	return mismatched, err
}

// Test lists the destination (rsync --list-only): it checks the host, the
// credentials, the module and the folder without writing anything.
func (r Rsync) Test(ctx context.Context, cfg Config) (TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return TestResult{}, err
	}
	tool, err := r.tool(ctx)
	if err != nil {
		return TestResult{}, err
	}
	conn, cleanup, err := tool.connection(cfg)
	if err != nil {
		return TestResult{}, err
	}
	defer cleanup()
	dest := destination(cfg)
	result := TestResult{Sample: []string{}, Tool: tool.Path + " " + tool.Version, Warnings: tool.warnings()}
	list := func(target string) ([]string, error) {
		var names []string
		args := append([]string{"--list-only", "-8"}, conn...)
		err := r.run(ctx, tool.Path, append(args, target), cfg.Password, func(line string) {
			if name := listedName(line); name != "" {
				names = append(names, name)
			}
		})
		return names, err
	}
	names, err := list(dest)
	if kind, _, ok := KindOf(err); ok && kind == KindPath && cfg.Path != "" && cfg.Path != "/" {
		// The folder does not exist yet: fine if its parent does, it is created on upload.
		parent := cfg
		parent.Path = path.Dir(strings.TrimRight(cfg.Path, "/"))
		if parent.Path == "." {
			parent.Path = ""
		}
		if _, perr := list(destination(parent)); perr == nil {
			result.PathMissing = true
			return result, nil
		}
	}
	if err != nil {
		return result, err
	}
	result.Entries = len(names)
	result.Sample = names[:min(5, len(names))]
	return result, nil
}

var listPattern = regexp.MustCompile(`^[-dlcbps][rwxsStT-]{9}\s+\S+\s+\S+\s+\S+\s+(.+)$`)

// listedName extracts the name from a --list-only line (ignoring ".").
func listedName(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	name := line
	if match := listPattern.FindStringSubmatch(line); match != nil {
		name = match[1]
	} else if tab := strings.IndexByte(line, '\t'); tab > 0 {
		name = line[:tab]
	}
	name = strings.TrimSpace(name)
	if name == "." {
		return ""
	}
	return name
}

// run executes rsync, feeding every output line (split on \n and \r) to onLine.
func (r Rsync) run(ctx context.Context, binary string, args []string, password string, onLine func(string)) error {
	if err := checkArgs(args); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = nil
	cmd.WaitDelay = 5 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &tailBuffer{limit: 8 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return &Error{Kind: KindNotFound, Detail: Scrub(err.Error(), password)}
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	scanner.Split(splitLines)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			onLine(line)
		}
	}
	io.Copy(io.Discard, stdout)
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr == nil {
		return nil
	}
	code := -1
	var exit *exec.ExitError
	if errors.As(waitErr, &exit) {
		code = exit.ExitCode()
	}
	text := stderr.String()
	if text == "" {
		text = waitErr.Error()
	}
	return Classify(code, text, password)
}

// splitLines splits on "\n" and "\r" (rsync redraws progress with "\r").
func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// tailBuffer keeps the last bytes written to it.
type tailBuffer struct {
	limit int
	data  []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string { return string(b.data) }

// checkParent covers rsync without --mkpath (older versions, openrsync): only
// the last folder is created, so a destination missing several levels fails
// with a clear message instead of an obscure rsync error.
func (r Rsync) checkParent(ctx context.Context, tool Tool, cfg Config, conn []string) error {
	if strings.Trim(cfg.Path, "/") == "" {
		return nil
	}
	probe := func(c Config) error {
		args := append([]string{"--list-only", "-d", "-8"}, conn...)
		return r.run(ctx, tool.Path, append(args, destination(c)), cfg.Password, func(string) {})
	}
	err := probe(cfg)
	if kind, _, ok := KindOf(err); !ok || kind != KindPath {
		return nil
	}
	parent := cfg
	parent.Path = path.Dir(strings.TrimRight(cfg.Path, "/"))
	if parent.Path == "." || parent.Path == "/" {
		return nil
	}
	if kind, _, ok := KindOf(probe(parent)); ok && kind == KindPath {
		return &Error{Kind: KindMkpath}
	}
	return nil
}
