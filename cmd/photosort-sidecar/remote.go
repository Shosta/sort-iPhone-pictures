package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"sort-iphone-pictures/internal/remote"
	"sort-iphone-pictures/sorter"
)

// remoteStage is the plan "destination" of a remote sort: a scratch folder
// that only ever holds links to the photos while they are being sent.
func remoteStage() (string, error) {
	dir, err := os.MkdirTemp("", "photosort-stage-")
	if err != nil {
		return "", err
	}
	return dir, nil
}

// prepareRemotePlan validates the remote configuration and returns the scratch
// destination the engine plans against (nothing exists remotely to compare with).
func prepareRemotePlan(req *request) (cleanup func(), err error) {
	if err := req.Remote.Validate(); err != nil {
		return nil, err
	}
	dir, err := remoteStage()
	if err != nil {
		return nil, err
	}
	req.Destination = dir
	// The engine only needs the folder while planning; sending recreates it.
	return func() { os.Remove(dir) }, nil
}

func (s *server) remoteTest(ctx context.Context, req request) error {
	if req.Remote == nil {
		return errors.New("no remote destination given")
	}
	cfg := *req.Remote
	res, err := s.transport.Test(ctx, cfg)
	if err != nil {
		return err
	}
	s.send(event{"id": req.ID, "type": "remoteTest", "ok": true, "url": cfg.URL(), "entries": res.Entries,
		"sample": res.Sample, "pathMissing": res.PathMissing, "tool": res.Tool, "warnings": res.Warnings})
	return nil
}

func (s *server) remoteCopy(ctx context.Context, req request, plan sorter.Plan) error {
	cfg := *req.Remote
	if err := cfg.Validate(); err != nil {
		return err
	}
	total := len(plan.Entries)
	items := make([]remote.Item, 0, total)
	entries := make(map[string]sorter.Entry, total)
	var tagItems []remote.Item
	for _, entry := range plan.Entries {
		items = append(items, remote.Item{Source: entry.Source, Target: entry.Target})
		entries[entry.Target] = entry
		// Tags live in an .xmp next to the photo; they travel with it and never
		// replace one already on the NAS (--ignore-existing).
		if _, err := os.Stat(entry.Source + ".xmp"); err == nil {
			tagItems = append(tagItems, remote.Item{Source: entry.Source + ".xmp", Target: entry.Target + ".xmp", Sidecar: true})
		}
	}
	cacheDir := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cacheDir = filepath.Join(dir, "photosort", "remote")
	}
	var (
		lib     remote.Library
		report  = remote.Report{ToSend: items}
		skipped = map[string]bool{}
	)
	if !req.RemoteNoDedupe {
		var err error
		lib, err = remote.LoadLibrary(ctx, s.transport, cfg, remote.LibraryOptions{CacheDir: cacheDir, MaxAge: 24 * time.Hour, Refresh: req.RemoteRefresh})
		if err != nil {
			return err
		}
		report = remote.Dedupe(items, lib)
		for _, sk := range report.Skipped {
			skipped[sk.Item.Target] = true
		}
	}
	toSend := append(append([]remote.Item{}, report.ToSend...), tagItems...)
	s.busy.Add(1)
	defer s.busy.Add(-1)
	var (
		lock       sync.Mutex
		reported   = map[string]bool{}
		lastStatus time.Time
	)
	var sentItems []remote.Item
	opts := remote.Options{DryRun: req.DryRun}
	if !req.DryRun {
		opts.OnSent = func(item remote.Item) {
			lock.Lock()
			reported[item.Target] = true
			sentItems = append(sentItems, item)
			lock.Unlock()
			s.send(event{"id": req.ID, "type": "progress", "entry": entries[item.Target], "total": total})
		}
		opts.OnProgress = func(p remote.Progress) {
			lock.Lock()
			due := time.Since(lastStatus) >= statusInterval*2 || p.Percent >= 100
			if due {
				lastStatus = time.Now()
			}
			lock.Unlock()
			if due {
				s.send(event{"id": req.ID, "type": "remoteProgress", "percent": p.Percent, "bytes": p.Bytes, "rate": p.Rate, "eta": p.ETA, "done": p.Done, "total": p.Total})
			}
		}
	}
	result, err := remote.Send(ctx, s.transport, cfg, plan.Destination, toSend, opts)
	if err != nil {
		return err
	}
	if !req.DryRun {
		// What was already on the NAS counts as sorted too.
		for _, entry := range plan.Entries {
			lock.Lock()
			done := reported[entry.Target] && !skipped[entry.Target]
			lock.Unlock()
			if !done {
				s.send(event{"id": req.ID, "type": "progress", "entry": entry, "total": total})
			}
		}
	}
	if !req.DryRun && !req.RemoteNoDedupe && len(sentItems) > 0 {
		var photos []remote.Item
		for _, item := range sentItems {
			if !item.Sidecar {
				photos = append(photos, item)
			}
		}
		// The index is a convenience: failing to save it must not fail the sort.
		if err := remote.Remember(ctx, s.transport, cfg, lib, cacheDir, photos); err != nil {
			result.Warnings = append(result.Warnings, "index: "+err.Error())
		}
	}
	mismatched := result.Mismatched
	if len(mismatched) > 20 {
		mismatched = mismatched[:20]
	}
	s.send(event{"id": req.ID, "type": "done", "copied": total, "total": total, "remote": true, "dryRun": result.DryRun,
		"sent": len(result.Sent), "sentBytes": result.SentBytes, "existing": result.Existing,
		"mismatched": len(result.Mismatched), "mismatchedSample": mismatched, "warnings": result.Warnings, "url": cfg.URL(),
		"alreadyPath": report.Count(remote.ReasonPath), "alreadySame": report.Count(remote.ReasonSame), "alreadyIndex": report.Count(remote.ReasonIndex),
		"alreadyTotal": len(report.Skipped), "toSend": len(report.ToSend), "tags": result.Tags, "tagsExisting": result.TagsExisting})
	return nil
}

func (s *server) remoteList(ctx context.Context, req request) error {
	if req.Remote == nil {
		return errors.New("no remote destination given")
	}
	cfg := *req.Remote
	listing, err := s.transport.List(ctx, cfg, req.RemoteDir)
	if err != nil {
		return err
	}
	s.send(event{"id": req.ID, "type": "remoteList", "ok": true, "path": listing.Path, "dirs": listing.Dirs,
		"files": listing.Files, "missing": listing.Missing, "tool": listing.Tool, "warnings": listing.Warnings})
	return nil
}
