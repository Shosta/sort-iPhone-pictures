package remote

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	cfg, err := ParseURL("rsync://remi@nas.local:874/photos/2024/Trips")
	if err != nil || cfg.Protocol != Daemon || cfg.Host != "nas.local" || cfg.Port != 874 || cfg.User != "remi" || cfg.Module != "photos" || cfg.Path != "2024/Trips" {
		t.Fatalf("daemon url: %+v %v", cfg, err)
	}
	cfg, err = ParseURL("ssh://remi@nas/share/Photos")
	if err != nil || cfg.Protocol != SSH || cfg.Path != "/share/Photos" {
		t.Fatalf("ssh url: %+v %v", cfg, err)
	}
	for _, bad := range []string{"rsync://u:secret@nas/m", "http://nas/m", "rsync://nas", "rsync://nas/m/../x", "rsync://-oProxy@nas/m", "ssh://nas"} {
		if _, err := ParseURL(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if got := (Config{Protocol: Daemon, Host: "nas", Port: 873, User: "u", Module: "m", Path: "p", Password: "secret"}).URL(); got != "rsync://u@nas:873/m/p" || strings.Contains(got, "secret") {
		t.Errorf("url = %q", got)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]Kind{
		"@ERROR: auth failed on module photos\nrsync error: error starting client-server protocol (code 5) at main.c(1850)": KindAuth,
		"@ERROR: Unknown module 'photo'":                                         KindModule,
		"@ERROR: access denied to photos from 10.0.0.2":                          KindDenied,
		"rsync: failed to connect to nas: Connection refused (111)":              KindRefused,
		"rsync: getaddrinfo: nas: Name or service not known":                     KindHost,
		"ssh: Could not resolve hostname nas: nodename nor servname provided":    KindHost,
		"ssh: connect to host nas port 22: Operation timed out":                  KindUnreachable,
		"user@nas: Permission denied (publickey,password).":                      KindAuth,
		"Host key verification failed.":                                          KindHostKey,
		"rsync: [receiver] write failed: No space left on device (28)":           KindDiskFull,
		"rsync: connection unexpectedly closed (0 bytes received so far)":        KindInterrupted,
		"bash: rsync: command not found":                                         KindRemoteTool,
		"rsync: [Receiver] mkdir \"/x/y\" failed: No such file or directory (2)": KindPath,
		"something odd": KindUnknown,
	}
	for text, want := range cases {
		if got := Classify(1, text); got.Kind != want {
			t.Errorf("%q: got %s, want %s", text, got.Kind, want)
		}
	}
	if got := Classify(1, "auth failed on module x with pw hunter22", "hunter22"); strings.Contains(got.Detail, "hunter22") {
		t.Errorf("password leaked: %q", got.Detail)
	}
}

func TestStageLinksWithoutCopyingAndUnstageKeepsPhotos(t *testing.T) {
	dir := t.TempDir()
	photo := filepath.Join(dir, "IMG_1.jpg")
	if err := os.WriteFile(photo, []byte("pixels"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "stage")
	items, err := Stage(root, []Item{{Source: photo, Target: "2024/2024-05/IMG_1.jpg"}})
	if err != nil || len(items) != 1 || items[0].Size != 6 {
		t.Fatalf("stage: %+v %v", items, err)
	}
	linked := filepath.Join(root, "2024", "2024-05", "IMG_1.jpg")
	if data, err := os.ReadFile(linked); err != nil || string(data) != "pixels" {
		t.Fatalf("link unreadable: %v", err)
	}
	if err := Unstage(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stage not removed")
	}
	if data, _ := os.ReadFile(photo); string(data) != "pixels" {
		t.Fatal("photo was damaged")
	}
	library := t.TempDir()
	os.WriteFile(filepath.Join(library, "mine.jpg"), []byte("x"), 0o644)
	if _, err := Stage(library, []Item{{Source: photo, Target: "a.jpg"}}); err == nil {
		t.Fatal("a non-empty folder was taken for a staging folder")
	}
	if err := Unstage(library); err == nil {
		t.Fatal("Unstage removed a library")
	}
	if _, err := Stage(root, []Item{{Source: photo, Target: "../escape.jpg"}}); err == nil {
		t.Fatal("unsafe target accepted")
	}
	Unstage(root)
	if _, err := Stage(root, []Item{{Source: photo, Target: "a.jpg"}, {Source: photo, Target: "a.jpg"}}); err == nil {
		t.Fatal("duplicate target accepted")
	}
}

// fakeRsync writes a stand-in rsync that logs its arguments and behaves as
// told by environment variables.
func fakeRsync(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "rsync  version ${FAKE_VERSION:-3.2.7}  protocol version 31"; exit 0; fi
echo "$@" >> "$FAKE_LOG"
for a in "$@"; do
  case "$a" in
    --password-file=*) f="${a#--password-file=}"; cat "$f" > "$FAKE_PW"; ls -l "$f" | cut -c1-10 > "$FAKE_PERMS";;
  esac
done
if [ -n "$FAKE_STDERR" ] && [ -z "$FAKE_AFTER_SENT" ]; then echo "$FAKE_STDERR" >&2; exit "${FAKE_EXIT:-1}"; fi
case " $* " in
  *" --list-only "*) printf '%s\n' "$FAKE_LIST"; exit 0;;
  *" --checksum "*) printf '%s\n' "$FAKE_MISMATCH"; exit 0;;
esac
printf '  1,000  50%%  1.00MB/s  0:00:01 (xfr#1, to-chk=1/2)\r\n'
printf '%s\n' "$FAKE_SENT"
if [ -n "$FAKE_STDERR" ]; then echo "$FAKE_STDERR" >&2; exit "${FAKE_EXIT:-1}"; fi
exit 0
`
	path := filepath.Join(t.TempDir(), "rsync")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_LOG", filepath.Join(t.TempDir(), "log"))
	t.Setenv("FAKE_PW", filepath.Join(t.TempDir(), "pw"))
	t.Setenv("FAKE_PERMS", filepath.Join(t.TempDir(), "perms"))
	return path
}

func testItems(t *testing.T) []Item {
	t.Helper()
	dir := t.TempDir()
	var items []Item
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("12345"), 0o644); err != nil {
			t.Fatal(err)
		}
		items = append(items, Item{Source: path, Target: "2024/" + name})
	}
	return items
}

func daemonConfig() Config {
	return Config{Protocol: Daemon, Host: "nas", Port: 873, User: "remi", Module: "photos", Path: "Library", Password: "s3cret-pw"}
}

func TestUploadDaemonNeverExposesPassword(t *testing.T) {
	fake := fakeRsync(t)
	t.Setenv("FAKE_SENT", "2024/\n2024/a.jpg\n2024/c.jpg")
	var sent []string
	var progress []Progress
	result, err := Send(context.Background(), Rsync{Path: fake}, daemonConfig(), filepath.Join(t.TempDir(), "stage"), testItems(t), Options{
		OnSent:     func(item Item) { sent = append(sent, item.Target) },
		OnProgress: func(p Progress) { progress = append(progress, p) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sent, []string{"2024/a.jpg", "2024/c.jpg"}) || result.Existing != 1 || result.SentBytes != 10 {
		t.Fatalf("result = %+v sent=%v", result, sent)
	}
	if len(progress) == 0 || progress[0].Percent != 50 || progress[0].Rate != "1.00MB/s" || progress[0].Bytes != 1000 {
		t.Fatalf("progress = %+v", progress)
	}
	log, _ := os.ReadFile(os.Getenv("FAKE_LOG"))
	args := string(log)
	for _, want := range []string{"-rLt", "--partial-dir=.rsync-partial", "--ignore-existing", "--info=progress2", "--mkpath", "rsync://remi@nas:873/photos/Library/", "--password-file="} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in %q", want, args)
		}
	}
	if strings.Contains(args, "s3cret-pw") || strings.Contains(args, "--dry-run") || strings.Contains(args, "--checksum") {
		t.Errorf("unexpected arguments: %q", args)
	}
	if pw, _ := os.ReadFile(os.Getenv("FAKE_PW")); strings.TrimSpace(string(pw)) != "s3cret-pw" {
		t.Errorf("password file content = %q", pw)
	}
	if perms, _ := os.ReadFile(os.Getenv("FAKE_PERMS")); strings.TrimSpace(string(perms)) != "-rw-------" {
		t.Errorf("password file perms = %q", perms)
	}
	var leftovers []string
	for _, line := range strings.Fields(args) {
		if file, ok := strings.CutPrefix(line, "--password-file="); ok {
			if _, err := os.Stat(file); err == nil {
				leftovers = append(leftovers, file)
			}
		}
	}
	if len(leftovers) > 0 {
		t.Errorf("password file not removed: %v", leftovers)
	}
}

func TestUploadErrorsAreClassifiedWithoutPassword(t *testing.T) {
	fake := fakeRsync(t)
	t.Setenv("FAKE_STDERR", "@ERROR: auth failed on module photos (s3cret-pw)")
	_, err := Send(context.Background(), Rsync{Path: fake}, daemonConfig(), filepath.Join(t.TempDir(), "stage"), testItems(t), Options{})
	kind, detail, ok := KindOf(err)
	if !ok || kind != KindAuth || strings.Contains(detail, "s3cret-pw") {
		t.Fatalf("err = %v kind=%s detail=%q", err, kind, detail)
	}
}

func TestUploadDryRunSSHAndLegacyTool(t *testing.T) {
	fake := fakeRsync(t)
	t.Setenv("FAKE_VERSION", "2.6.9\n openrsync")
	t.Setenv("FAKE_SENT", "2024/b.jpg")
	cfg := Config{Protocol: SSH, Host: "nas", Port: 2222, User: "remi", Path: "/share/Photos", KeyFile: "/home/me/my key"}
	result, err := Send(context.Background(), Rsync{Path: fake}, cfg, filepath.Join(t.TempDir(), "stage"), testItems(t), Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || len(result.Sent) != 1 || result.Existing != 2 || !slices.Contains(result.Warnings, "legacyRsync") {
		t.Fatalf("result = %+v", result)
	}
	log, _ := os.ReadFile(os.Getenv("FAKE_LOG"))
	args := string(log)
	for _, want := range []string{"--dry-run", "remi@nas:/share/Photos/", "-p 2222", `-i "/home/me/my key"`, "BatchMode=yes"} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in %q", want, args)
		}
	}
	if strings.Contains(args, "--info=progress2") || strings.Contains(args, "--mkpath") {
		t.Errorf("legacy rsync got modern options: %q", args)
	}
}

func TestUploadChecksumVerifiesWithoutOverwriting(t *testing.T) {
	fake := fakeRsync(t)
	t.Setenv("FAKE_SENT", "")
	t.Setenv("FAKE_MISMATCH", "2024/b.jpg")
	cfg := daemonConfig()
	cfg.Checksum = true
	result, err := Send(context.Background(), Rsync{Path: fake}, cfg, filepath.Join(t.TempDir(), "stage"), testItems(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Mismatched, []string{"2024/b.jpg"}) {
		t.Fatalf("mismatched = %v", result.Mismatched)
	}
	log, _ := os.ReadFile(os.Getenv("FAKE_LOG"))
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "--ignore-existing") || strings.Contains(lines[1], "--ignore-existing") || !strings.Contains(lines[1], "-rLn --checksum") {
		t.Fatalf("calls = %q", lines)
	}
}

func TestStageIsRemovedAfterFailure(t *testing.T) {
	fake := fakeRsync(t)
	t.Setenv("FAKE_STDERR", "connection unexpectedly closed")
	stage := filepath.Join(t.TempDir(), "stage")
	_, err := Send(context.Background(), Rsync{Path: fake}, daemonConfig(), stage, testItems(t), Options{})
	if kind, _, _ := KindOf(err); kind != KindInterrupted || !kind.Resumable() {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stage left behind")
	}
}

func TestTestListsAndDetectsMissingFolder(t *testing.T) {
	fake := fakeRsync(t)
	t.Setenv("FAKE_LIST", "drwxr-xr-x          4,096 2024/05/01 10:00:00 .\ndrwxr-xr-x          4,096 2024/05/01 10:00:00 2023\n-rw-r--r--          12 2024/05/01 10:00:00 My Photo.jpg")
	res, err := Rsync{Path: fake}.Test(context.Background(), daemonConfig())
	if err != nil || res.Entries != 2 || res.Sample[1] != "My Photo.jpg" || res.PathMissing {
		t.Fatalf("res = %+v err=%v", res, err)
	}
	t.Setenv("FAKE_LIST", "")
	t.Setenv("FAKE_STDERR", "")
	if _, err := (Rsync{Path: fake}).Test(context.Background(), Config{Protocol: Daemon, Host: "nas", Module: "../x"}); err == nil {
		t.Fatal("invalid config accepted")
	}
}

func TestMissingRsync(t *testing.T) {
	_, err := Rsync{Path: filepath.Join(t.TempDir(), "nope")}.Test(context.Background(), daemonConfig())
	if kind, _, _ := KindOf(err); kind != KindNotFound {
		t.Fatalf("err = %v", err)
	}
}

// With a real rsync, a local folder stands in for the NAS: the staged links are
// followed, accented folders keep their names and nothing existing is rewritten.
func TestRealRsyncFollowsLinksAndKeepsExisting(t *testing.T) {
	bin, err := exec.LookPath("rsync")
	if err != nil {
		t.Skip("rsync not installed")
	}
	tool, err := Probe(context.Background(), bin)
	if err != nil {
		t.Skip("rsync not usable")
	}
	src := t.TempDir()
	photo := filepath.Join(src, "IMG.jpg")
	os.WriteFile(photo, []byte("new"), 0o644)
	stage := filepath.Join(t.TempDir(), "stage")
	items, err := Stage(stage, []Item{{Source: photo, Target: "2024/Montréal/IMG.jpg"}, {Source: photo, Target: "2024/Montréal/OLD.jpg"}})
	if err != nil {
		t.Fatal(err)
	}
	remoteDir := t.TempDir()
	os.MkdirAll(filepath.Join(remoteDir, "2024", "Montréal"), 0o755)
	os.WriteFile(filepath.Join(remoteDir, "2024", "Montréal", "OLD.jpg"), []byte("already there"), 0o644)
	args := tool.uploadArgs(Config{}, Job{Dir: stage, Items: items}, nil, remoteDir+"/")
	args = slices.DeleteFunc(args, func(a string) bool { return a == "--mkpath" })
	var sent []string
	err = Rsync{}.run(context.Background(), bin, args, "", func(line string) { sent = append(sent, line) })
	if err != nil {
		t.Fatalf("rsync: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(remoteDir, "2024", "Montréal", "IMG.jpg")); string(data) != "new" {
		t.Errorf("file not uploaded as a regular file: %q", data)
	}
	if info, _ := os.Lstat(filepath.Join(remoteDir, "2024", "Montréal", "IMG.jpg")); info == nil || info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("symlink copied instead of the photo")
	}
	if data, _ := os.ReadFile(filepath.Join(remoteDir, "2024", "Montréal", "OLD.jpg")); string(data) != "already there" {
		t.Errorf("existing file rewritten: %q", data)
	}
	if !slices.Contains(sent, "2024/Montréal/IMG.jpg") || slices.Contains(sent, "2024/Montréal/OLD.jpg") {
		t.Errorf("reported names = %q", sent)
	}
}
