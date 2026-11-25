package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"sort-iphone-pictures/internal/devimport"
	"sort-iphone-pictures/sorter"
)

const (
	importModeAll    = "all"
	importModeNew    = "new"
	importModePeriod = "period"
	dateLayout       = "2006-01-02"
	reconnectPoll    = 2 * time.Second
)

// gate lets the UI pause an import: workers call wait between files.
type gate struct {
	lock   sync.Mutex
	paused bool
	resume chan struct{}
}

func newGate() *gate { return &gate{resume: make(chan struct{})} }

func (g *gate) set(paused bool) bool {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.paused == paused {
		return false
	}
	g.paused = paused
	if !paused {
		close(g.resume)
		g.resume = make(chan struct{})
	}
	return true
}

func (g *gate) wait(ctx context.Context) error {
	for {
		g.lock.Lock()
		paused, resume := g.paused, g.resume
		g.lock.Unlock()
		if !paused {
			return ctx.Err()
		}
		select {
		case <-resume:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *server) importRoots() []string {
	if s.devRoots != nil {
		return s.devRoots
	}
	return devimport.DefaultRoots()
}

func (s *server) openJournal(id string) (*devimport.Journal, error) {
	dir := s.journalDir
	if dir == "" {
		var err error
		if dir, err = devimport.DefaultJournalDir(); err != nil {
			return nil, err
		}
	}
	return devimport.OpenJournal(dir, id)
}

// importSource resolves the device a request talks about: its DCIM folder and
// the id of its journal (a manually chosen folder gets an id of its own).
func importSource(req request) (dcim, id string, err error) {
	dcim = req.DevicePath
	if dcim == "" {
		return "", "", &devimport.Error{Code: devimport.CodeNoDevice, Detail: "no source"}
	}
	if info, err := os.Stat(dcim); err != nil || !info.IsDir() {
		return "", "", &devimport.Error{Code: devimport.CodeNoDevice, Detail: dcim, Err: err}
	}
	id = req.DeviceID
	if id == "" {
		id = devimport.IDForPath(dcim)
	}
	return dcim, id, nil
}

// importRange parses the period of a "period" import: from 00:00 of the first
// day to 23:59:59 of the last one.
func importRange(req request) (from, to time.Time, err error) {
	if req.Mode != importModePeriod {
		return
	}
	if from, err = time.ParseInLocation(dateLayout, req.From, time.Local); err != nil {
		return from, to, fmt.Errorf("invalid start date: %w", err)
	}
	if to, err = time.ParseInLocation(dateLayout, req.To, time.Local); err != nil {
		return from, to, fmt.Errorf("invalid end date: %w", err)
	}
	if to.Before(from) {
		return from, to, errors.New("end date is before start date")
	}
	to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, time.Local)
	return from, to, nil
}

func (s *server) importDetect(req request) error {
	devices := devimport.Detect(s.importRoots())
	if devices == nil {
		devices = []devimport.Device{}
	}
	s.send(event{"id": req.ID, "type": "importDevices", "devices": devices})
	return nil
}

func (s *server) importScan(ctx context.Context, req request) error {
	dcim, id, err := importSource(req)
	if err != nil {
		return err
	}
	from, to, err := importRange(req)
	if err != nil {
		return err
	}
	var journal *devimport.Journal
	if req.Mode == importModeNew {
		if journal, err = s.openJournal(id); err != nil {
			return err
		}
	}
	scan, err := devimport.ScanDCIM(ctx, dcim)
	if err != nil {
		return err
	}
	files := scan.Filter(journal, from, to)
	var bytes int64
	e := event{"id": req.ID, "type": "importScan", "total": len(scan.Files), "videos": scan.Videos, "count": len(files)}
	for _, f := range files {
		bytes += f.Size
	}
	e["bytes"] = bytes
	if len(files) > 0 {
		e["from"], e["to"] = files[0].ModTime.Format(dateLayout), files[len(files)-1].ModTime.Format(dateLayout)
	}
	s.send(e)
	return nil
}

func (s *server) importForget(req request) error {
	_, id, err := importSource(req)
	if err != nil {
		return err
	}
	journal, err := s.openJournal(id)
	if err != nil {
		return err
	}
	if err := journal.Forget(); err != nil {
		return err
	}
	s.send(event{"id": req.ID, "type": "done"})
	return nil
}

func (s *server) importControl(req request) error {
	s.lock.Lock()
	g := s.gates[req.Target]
	s.lock.Unlock()
	if g == nil {
		return nil
	}
	paused := req.Action == "pause"
	if g.set(paused) {
		kind := "importResumed"
		if paused {
			kind = "importPaused"
		}
		s.send(event{"id": req.Target, "type": kind, "reason": "manual"})
	}
	return nil
}

// staged is a batch copied into the transit folder, waiting to be sorted.
type staged struct {
	n     int
	root  string
	files []File
}

type File = devimport.File

// importRun copies the device photos in batches (about 200 MB) and sorts each
// into the destination while the next one is copied. A file counts as
// delivered, and enters the journal, only once its sorted copy is verified.
func (s *server) importRun(ctx context.Context, req request) error {
	if !s.importing.CompareAndSwap(false, true) {
		return errors.New("an import is already running")
	}
	defer s.importing.Store(false)
	if req.Destination == "" {
		return errors.New("no destination")
	}
	dcim, id, err := importSource(req)
	if err != nil {
		return err
	}
	from, to, err := importRange(req)
	if err != nil {
		return err
	}
	journal, err := s.openJournal(id)
	if err != nil {
		return err
	}
	scan, err := devimport.ScanDCIM(ctx, dcim)
	if err != nil {
		return err
	}
	filter := journal
	if req.Mode != importModeNew {
		filter = nil
	}
	files := scan.Filter(filter, from, to)
	total := len(files)
	if total == 0 {
		s.send(event{"id": req.ID, "type": "importDone", "imported": 0, "duplicates": 0, "videos": scan.Videos, "filtered": 0, "skipped": 0, "total": 0})
		return nil
	}
	batches := devimport.Batches(files, devimport.BatchBytes)
	var biggest int64
	for _, b := range batches {
		var size int64
		for _, f := range b {
			size += f.Size
		}
		biggest = max(biggest, size)
	}
	transit, err := devimport.PrepareTransit(2 * biggest)
	if err != nil {
		return err
	}
	defer devimport.CleanTransit()

	g := newGate()
	s.lock.Lock()
	s.gates[req.ID] = g
	s.lock.Unlock()
	defer func() {
		s.lock.Lock()
		delete(s.gates, req.ID)
		s.lock.Unlock()
	}()

	started := time.Now()
	var copied, delivered atomic.Int64
	progress := func(phase string, batch int, stage string) {
		e := event{"id": req.ID, "type": "importProgress", "phase": phase, "batch": batch + 1, "batches": len(batches),
			"copied": copied.Load(), "done": delivered.Load(), "total": total, "elapsedMs": time.Since(started).Milliseconds()}
		if stage != "" {
			e["stage"] = stage
		}
		s.send(e)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	queue := make(chan staged)
	producerErr := make(chan error, 1)
	go func() {
		defer close(queue)
		base := dcim
		for n, batch := range batches {
			var size int64
			for _, f := range batch {
				size += f.Size
			}
			if !devimport.EnoughSpace(transit, size) {
				producerErr <- &devimport.Error{Code: devimport.CodeLowDisk, Detail: transit}
				return
			}
			root := filepath.Join(transit, fmt.Sprintf("batch-%d", n))
			for _, f := range batch {
				for {
					if err := g.wait(runCtx); err != nil {
						producerErr <- err
						return
					}
					err := devimport.CopyFile(runCtx, filepath.Join(base, f.Rel), filepath.Join(root, f.Rel), f)
					if err == nil {
						break
					}
					if !devimport.Gone(base, err) {
						if runCtx.Err() != nil {
							producerErr <- runCtx.Err()
						} else {
							producerErr <- &devimport.Error{Code: devimport.CodeCopy, Detail: f.Rel, Err: err}
						}
						return
					}
					s.send(event{"id": req.ID, "type": "importPaused", "reason": "disconnected"})
					if base, err = s.waitForDevice(runCtx, id, base); err != nil {
						producerErr <- err
						return
					}
					s.send(event{"id": req.ID, "type": "importResumed", "reason": "reconnected"})
				}
				copied.Add(1)
				progress("copy", n, "")
			}
			select {
			case queue <- staged{n, root, batch}:
			case <-runCtx.Done():
				producerErr <- runCtx.Err()
				return
			}
		}
	}()

	var imported, duplicates, filtered, skipped int
	consume := func() error {
		for st := range queue {
			if err := g.wait(runCtx); err != nil {
				return err
			}
			canon, err := filepath.EvalSymlinks(st.root)
			if err != nil {
				return err
			}
			engine := sorter.Engine{
				ByLocation: req.ByLocation, Landmarks: req.Landmarks, SkipDuplicates: req.Dedupe, GroupSimilar: req.Similar,
				DateFallback: req.DateFallback, ScreenshotFolder: req.Screenshots, GroupBlurry: req.Blurry,
				TakenFrom: from, TakenTo: to,
				Progress: func(stage string, _, _ int) { progress("sort", st.n, stage) },
			}
			progress("sort", st.n, "exif")
			plan, err := engine.Plan(runCtx, []string{canon}, req.Destination)
			if err != nil {
				return err
			}
			report, err := sorter.Copy(runCtx, plan)
			if err != nil {
				return err
			}
			if report.Copied != len(plan.Entries) {
				return fmt.Errorf("batch %d: %d of %d photos sorted", st.n+1, report.Copied, len(plan.Entries))
			}
			byRel := make(map[string]File, len(st.files))
			for _, f := range st.files {
				byRel[f.Rel] = f
			}
			var done []File
			mark := func(path string) {
				if rel, err := filepath.Rel(canon, path); err == nil {
					if f, ok := byRel[rel]; ok {
						done = append(done, f)
					}
				}
			}
			for _, entry := range plan.Entries {
				info, err := os.Stat(filepath.Join(plan.Destination, entry.Target))
				src, srcErr := os.Stat(entry.Source)
				if err != nil || srcErr != nil || info.Size() != src.Size() {
					return fmt.Errorf("batch %d: %s was not delivered", st.n+1, entry.Target)
				}
				mark(entry.Source)
			}
			for _, dup := range plan.Duplicates {
				mark(dup.Path)
			}
			if err := journal.Add(done); err != nil {
				return err
			}
			os.RemoveAll(st.root)
			imported += len(plan.Entries)
			duplicates += len(plan.Duplicates)
			filtered += plan.Filtered
			skipped += len(plan.Skipped)
			delivered.Add(int64(len(st.files)))
			progress("sort", st.n, "")
		}
		return nil
	}
	err = consume()
	cancel()
	for range queue {
	}
	select {
	case perr := <-producerErr:
		if err == nil || errors.Is(err, context.Canceled) {
			err = perr
		}
	default:
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	s.send(event{"id": req.ID, "type": "importDone", "imported": imported, "duplicates": duplicates, "videos": scan.Videos,
		"filtered": filtered, "skipped": skipped, "total": total, "elapsedMs": time.Since(started).Milliseconds()})
	return nil
}

// waitForDevice blocks until the device is back (same id, possibly mounted
// elsewhere) and returns its DCIM folder.
func (s *server) waitForDevice(ctx context.Context, id, old string) (string, error) {
	ticker := time.NewTicker(reconnectPoll)
	defer ticker.Stop()
	for {
		for _, d := range devimport.Detect(s.importRoots()) {
			if d.ID == id {
				return d.Path, nil
			}
		}
		if info, err := os.Stat(old); err == nil && info.IsDir() {
			return old, nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}
