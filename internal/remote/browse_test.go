package remote

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseList(t *testing.T) {
	cases := []struct {
		line string
		name string
		dir  bool
		size int64
		ok   bool
	}{
		{"drwxr-xr-x          4,096 2024/05/01 10:00:00 2024", "2024", true, 4096, true},
		{"-rw-r--r--      1,234,567 2024/05/01 10:00:00 2024/05/IMG_1.HEIC", "2024/05/IMG_1.HEIC", false, 1234567, true},
		{"lrwxrwxrwx             12 2024/05/01 10:00:00 link -> target", "link", false, 12, true},
		{"drwxr-xr-x          4,096 2024/05/01 10:00:00 .", "", false, 0, false},
		{"photos\tPhoto library", "", false, 0, false},
	}
	for _, c := range cases {
		got, ok := parseList(c.line)
		if ok != c.ok || got.name != c.name || got.dir != c.dir || got.size != c.size {
			t.Errorf("parseList(%q) = %+v, %v", c.line, got, ok)
		}
	}
}

func TestListFoldersAndModules(t *testing.T) {
	t.Setenv("FAKE_LIST", "drwxr-xr-x 4,096 2024/05/01 10:00:00 .\n"+
		"drwxr-xr-x 4,096 2024/05/01 10:00:00 Zoo\n"+
		"drwxr-xr-x 4,096 2024/05/01 10:00:00 alpha\n"+
		"drwxr-xr-x 4,096 2024/05/01 10:00:00 @Recycle\n"+
		"drwxr-xr-x 4,096 2024/05/01 10:00:00 .@__thumb\n"+
		"-rw-r--r-- 10 2024/05/01 10:00:00 a.jpg\n")
	r := Rsync{Path: fakeRsync(t)}
	cfg := Config{Protocol: Daemon, Host: "nas", User: "me", Module: "photos", Password: "pw"}
	l, err := r.List(context.Background(), cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Dirs) != 2 || l.Dirs[0].Name != "alpha" || l.Dirs[1].Name != "Zoo" || l.Files != 1 {
		t.Fatalf("unexpected listing %+v", l)
	}
	if _, err := r.List(context.Background(), cfg, "../etc"); err == nil {
		t.Fatal("path traversal accepted")
	}

	t.Setenv("FAKE_LIST", "photos\tPhoto library\nvideo\t\n")
	cfg.Module = ""
	l, err = r.List(context.Background(), cfg, "")
	if err != nil || len(l.Dirs) != 2 || !l.Dirs[0].Module || l.Dirs[0].Comment != "Photo library" {
		t.Fatalf("modules: %+v %v", l, err)
	}
}

func TestDedupe(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(body), 0o600)
		return p
	}
	when := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	mk := func(p string) { os.Chtimes(p, when, when) }
	same, byPath, moved, fresh := write("a.jpg", "aaaa"), write("b.jpg", "bbbbb"), write("c.jpg", "cccccc"), write("d.jpg", "ddddddd")
	for _, p := range []string{same, byPath, moved, fresh} {
		mk(p)
	}
	sum, _ := HashFile(moved)
	lib := Library{
		Files: []RemoteFile{
			{Path: "2024/05/other/A.JPG", Size: 4, ModTime: when.Add(time.Second)},
			{Path: "2024/05/b.jpg", Size: 1, ModTime: when},
		},
		Index: Index{Photos: []Fingerprint{{SHA256: sum, Size: 6, Target: "old/place.jpg"}}},
	}
	items := []Item{
		{Source: same, Target: "2024/05/a.jpg"},
		{Source: byPath, Target: "2024/05/b.jpg"},
		{Source: moved, Target: "2024/05/c.jpg"},
		{Source: fresh, Target: "2024/05/d.jpg"},
	}
	r := Dedupe(items, lib)
	if len(r.ToSend) != 1 || r.ToSend[0].Target != "2024/05/d.jpg" {
		t.Fatalf("to send: %+v", r.ToSend)
	}
	if r.Count(ReasonSame) != 1 || r.Count(ReasonPath) != 1 || r.Count(ReasonIndex) != 1 {
		t.Fatalf("report: %+v", r.Skipped)
	}
}

func TestForbiddenOptionsRefused(t *testing.T) {
	for _, bad := range []string{"--delete", "--delete-after", "--del", "--remove-source-files", "--remove-sent-files", "--delete-excluded"} {
		if err := checkArgs([]string{"-a", bad, "x", "y"}); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestNoDestructiveOptionsGenerated(t *testing.T) {
	r := Rsync{Path: fakeRsync(t)}
	logFile := os.Getenv("FAKE_LOG")
	cfg := Config{Protocol: Daemon, Host: "nas", User: "me", Module: "photos", Path: "Lib", Password: "pw", Checksum: true}
	ctx := context.Background()
	r.Test(ctx, cfg)
	r.List(ctx, cfg, "x")
	r.ListTree(ctx, cfg)
	r.FetchIndex(ctx, cfg)
	r.StoreIndex(ctx, cfg, []byte("{}"))
	stage := t.TempDir()
	src := filepath.Join(t.TempDir(), "a.jpg")
	os.WriteFile(src, []byte("x"), 0o600)
	Send(ctx, r, cfg, stage, []Item{{Source: src, Target: "2024/a.jpg"}}, Options{})
	Send(ctx, r, cfg, stage, []Item{{Source: src, Target: "2024/a.jpg"}}, Options{DryRun: true})
	data, _ := os.ReadFile(logFile)
	if len(data) == 0 {
		t.Fatal("no command logged")
	}
	for _, word := range []string{"--delete", "--del ", "--remove-source-files", "--remove-sent-files"} {
		if strings.Contains(string(data)+" ", word) {
			t.Errorf("command contains %s:\n%s", word, data)
		}
	}
}
