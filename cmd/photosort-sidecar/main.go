// Command photosort-sidecar serves the sorting engine to the Electron app.
//
// Protocol: one JSON request per line on stdin, JSON events (one per line) on
// stdout. Every event carries the "id" of its request. Requests:
//
//	{"id":"1","op":"plan","sources":[...],"destination":"...","byLocation":true,"landmarks":true,"dedupe":true,"similar":true}
//	{"id":"2","op":"copy","planId":"1"}
//	{"id":"2","op":"copy","planId":"1","remote":{...},"dryRun":false}
//	{"id":"18","op":"remoteTest","remote":{"protocol":"daemon","host":"nas","port":873,"user":"me","module":"photos","path":"","password":"..."}}
//	{"id":"3","op":"thumbnail","path":"...","size":256}
//	{"id":"4","op":"tag","folder":"...","files":[...],"tagScenes":true,"tagMoment":true,"tagPeople":true,"tagMeta":true,"aiDir":"..."}
//	{"id":"5","op":"cancel","target":"2"}
//	{"id":"6","op":"weekDays","root":"...","from":"2026-05-04","to":"2026-06-14"}
//	{"id":"7","op":"weekPhotos","root":"...","from":"2026-05-11","to":"2026-05-17"}
//	{"id":"8","op":"scrapbookLayout","files":[...],"page":"a4-landscape","longSide":65,"gutter":5}
//	{"id":"9","op":"scrapbook","files":[...],"page":"a4-landscape","longSide":65,"gutter":5,"outDir":"...","fallbackDir":"...","name":"Semaine.pdf"}
//	{"id":"10","op":"yearMapYears","root":"..."}
//	{"id":"11","op":"yearMapScan","root":"...","year":2026}
//	{"id":"12","op":"yearMapLayout","places":[...],"page":"a4-landscape"}
//	{"id":"13","op":"yearMapPdf","places":[...],"page":"a4-landscape","name":"Carte 2026.pdf","title":"2026","subtitle":"...","outDir":"...","fallbackDir":"..."}
//	{"id":"14","op":"travelTrips","root":"...","year":2026}
//	{"id":"15","op":"travelScan","root":"...","from":"2026-07-01","to":"2026-07-14"}
//	{"id":"16","op":"travelLayout","steps":[...],"page":"a4-portrait","lang":"fr"}
//	{"id":"17","op":"travelPdf","steps":[...],"page":"a4-portrait","lang":"fr","name":"Voyage.pdf","title":"...","subtitle":"...","outDir":"...","fallbackDir":"..."}
//
// Events:
//
//   - plan: status (stage exif|hash|similar with done/total), draft (batches of
//     entries whose folder is known and of duplicates found so far), then plan.
//
//   - copy: progress per copied entry, then done.
//
//   - thumbnail: thumbnail (JPEG data URL). Previews seen while planning are
//     cached, so most thumbnails are served without decoding the photo again.
//
//   - tag: status (stage tags with done/total), tagged per photo (path, tags),
//     then done. Photos that already have a <photo>.xmp are left out; files,
//     if given, replace the folder walk.
//
//   - weekDays: days (photo count per YYYY-MM-DD) and rootOk (the folder looks
//     like a sorted library). weekPhotos: photos (path, date), oldest first;
//     root is the library folder, the day range is inclusive.
//
//   - scrapbookLayout: layout (pageWidth and pageHeight in mm, sheets of
//     placements in mm, failed photos that cannot be read). scrapbook: progress
//     per photo, then done (path, pages, photos, failed, fallback). Sizes are
//     in millimetres; a photo is never cropped, with at least 5 mm between photos.
//
//   - yearMapLayout and yearMapPdf take an optional provider (osm, positron,
//     topo, satellite; anything else is the offline vector map). Online maps
//     download only map tiles (cached on disk); if that fails the offline map
//     is used and the layout carries backdropError. The layout then has a
//     backdrop (JPEG data URL) and the credit to show on the map.
//
//   - yearMapYears: years (year folders of the library, newest first) and rootOk.
//     yearMapScan: status while GPS positions are read, then places (name, lat,
//     lon, count, shown photos), total and located photo counts. yearMapLayout
//     takes those places back and answers with the map drawn for a page: pageWidth
//     and pageHeight in mm, land and lakes (SVG paths in mm), pins (x, y, name,
//     count, shown, card box in mm or null). yearMapPdf writes that page: progress
//     per photo, then done (path, places, failed, fallback).
//
//   - travelTrips: status while GPS positions are read (year 0 reads the whole
//     library), then trips (from, to, photos, distanceKm, steps), most recent
//     first: runs of days spent more than 100 km from the usual place. travelScan
//     answers with trip, the route of the photos between from and to. A step is
//     (name, lat, lon, count, first, last, photo, key). travelLayout takes steps
//     back and answers with layout: pageWidth, pageHeight, land, lakes and route
//     (SVG paths in mm), stops (x, y, n, name, dates, photo, card box or null)
//     and distanceKm. travelPdf writes that page: progress per photo, then done
//     (path, steps, failed, fallback). lang (fr, en, da) sets the date format.
//
//   - remote: a plan (and its copy) with a "remote" object sends the sorted tree
//     to a NAS with rsync instead of copying locally; the plan destination is
//     then a scratch folder, "destination" is ignored. protocol is daemon
//     (module + password) or ssh (key authentication); checksum verifies the
//     files already on the NAS. copy streams progress per photo, remoteProgress
//     (percent, bytes, rate, eta, done, total), then done (sent, sentBytes,
//     existing, mismatched, mismatchedSample, warnings, dryRun). With dryRun
//     true nothing is sent and done only reports what would be. remoteTest
//     lists the destination: remoteTest (ok, entries, sample, pathMissing, tool,
//     warnings). Remote errors carry code (auth, module, denied, refused, host,
//     unreachable, hostKey, interrupted, diskFull, permission, path,
//     remoteRsync, rsyncMissing, protocol, config, unknown), detail and
//     resumable; running copy again resumes an interrupted upload.
//
// Any request may end with error (cancelled is true after a cancel); warning
// events are informational.
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"sort-iphone-pictures/internal/remote"
	"sort-iphone-pictures/internal/scrapbook"
	"sort-iphone-pictures/internal/tagger"
	"sort-iphone-pictures/internal/thumbs"
	"sort-iphone-pictures/sorter"
)

type request struct {
	ID           string         `json:"id"`
	Op           string         `json:"op"`
	Sources      []string       `json:"sources"`
	Destination  string         `json:"destination"`
	ByLocation   bool           `json:"byLocation"`
	Landmarks    bool           `json:"landmarks"`
	Dedupe       bool           `json:"dedupe"`
	Similar      bool           `json:"similar"`
	DateFallback bool           `json:"dateFallback"`
	Screenshots  bool           `json:"screenshots"`
	Blurry       bool           `json:"blurry"`
	TagScenes    bool           `json:"tagScenes"`
	TagMoment    bool           `json:"tagMoment"`
	TagPeople    bool           `json:"tagPeople"`
	TagMeta      bool           `json:"tagMeta"`
	AIDir        string         `json:"aiDir"`
	Folder       string         `json:"folder"`
	Files        []string       `json:"files"`
	PlanID       string         `json:"planId"`
	Path         string         `json:"path"`
	Size         int            `json:"size"`
	Target       string         `json:"target"`
	Root         string         `json:"root"`
	From         string         `json:"from"`
	To           string         `json:"to"`
	Page         string         `json:"page"`
	LongSide     float64        `json:"longSide"`
	Gutter       float64        `json:"gutter"`
	Bleed        float64        `json:"bleed"`
	Frame        string         `json:"frame"`
	Layout       string         `json:"layout"`
	Background   string         `json:"background"`
	OutDir       string         `json:"outDir"`
	FallbackDir  string         `json:"fallbackDir"`
	Name         string         `json:"name"`
	Year         int            `json:"year"`
	Provider     string         `json:"provider"`
	Title        string         `json:"title"`
	Subtitle     string         `json:"subtitle"`
	Places       []yearPlace    `json:"places"`
	Steps        []travelStep   `json:"steps"`
	Lang         string         `json:"lang"`
	Remote       *remote.Config `json:"remote"`
	DryRun       bool           `json:"dryRun"`
}

type event map[string]any

// Batching intervals keep the renderer smooth on large libraries.
const (
	draftInterval  = 80 * time.Millisecond
	statusInterval = 50 * time.Millisecond
	cacheBudget    = 96 << 20
	// busyThumbs bounds thumbnail decodes while a plan or a copy runs, so the
	// engine keeps most cores (and the disk) for itself.
	busyThumbs = 2
)

type server struct {
	out     io.Writer
	outLock sync.Mutex
	lock    sync.Mutex
	plans   map[string]sorter.Plan
	cancels map[string]context.CancelFunc
	thumbs  chan struct{}
	// busy counts running plans and copies; busySlots limits thumbnails meanwhile.
	busy      atomic.Int32
	busySlots chan struct{}
	cache     *thumbCache
	measurer  scrapbook.Measurer
	transport remote.Transport
	wg        sync.WaitGroup
}

func main() {
	srv := newServer(os.Stdout)
	if err := srv.serve(context.Background(), os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func newServer(out io.Writer) *server {
	return &server{
		out:       out,
		plans:     map[string]sorter.Plan{},
		cancels:   map[string]context.CancelFunc{},
		thumbs:    make(chan struct{}, runtime.NumCPU()),
		busySlots: make(chan struct{}, busyThumbs),
		cache:     newThumbCache(cacheBudget),
		transport: remote.Rsync{},
	}
}

func (s *server) serve(ctx context.Context, in io.Reader) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			s.send(event{"id": "", "type": "error", "message": "invalid request: " + err.Error()})
			continue
		}
		s.handle(ctx, req)
	}
	stop()
	s.wg.Wait()
	return scanner.Err()
}

func (s *server) send(e event) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	s.outLock.Lock()
	defer s.outLock.Unlock()
	s.out.Write(append(data, '\n'))
}

func (s *server) handle(parent context.Context, req request) {
	if req.Op == "cancel" {
		s.lock.Lock()
		cancel := s.cancels[req.Target]
		s.lock.Unlock()
		if cancel != nil {
			cancel()
		}
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.lock.Lock()
	s.cancels[req.ID] = cancel
	s.lock.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			cancel()
			s.lock.Lock()
			delete(s.cancels, req.ID)
			s.lock.Unlock()
		}()
		if err := s.run(ctx, req); err != nil {
			e := event{"id": req.ID, "type": "error", "message": err.Error(), "cancelled": errors.Is(err, context.Canceled)}
			if kind, detail, ok := remote.KindOf(err); ok {
				e["code"], e["detail"], e["resumable"] = string(kind), detail, kind.Resumable()
			}
			s.send(e)
		}
	}()
}

func (s *server) run(ctx context.Context, req request) error {
	switch req.Op {
	case "plan":
		return s.plan(ctx, req)
	case "copy":
		s.lock.Lock()
		plan, ok := s.plans[req.PlanID]
		s.lock.Unlock()
		if !ok {
			return errors.New("unknown plan")
		}
		if req.Remote != nil {
			return s.remoteCopy(ctx, req, plan)
		}
		total := len(plan.Entries)
		s.busy.Add(1)
		defer s.busy.Add(-1)
		report, err := sorter.CopyWithProgress(ctx, plan, func(entry sorter.Entry) {
			s.send(event{"id": req.ID, "type": "progress", "entry": entry, "total": total})
		})
		if err != nil {
			return err
		}
		s.send(event{"id": req.ID, "type": "done", "copied": report.Copied, "total": total})
	case "remoteTest":
		return s.remoteTest(ctx, req)
	case "thumbnail":
		return s.thumbnail(ctx, req)
	case "tag":
		return s.tag(ctx, req)
	case "weekDays":
		return s.weekDays(ctx, req)
	case "weekPhotos":
		return s.weekPhotos(ctx, req)
	case "scrapbookLayout":
		return s.scrapbookLayout(ctx, req)
	case "scrapbook":
		return s.scrapbook(ctx, req)
	case "yearMapYears":
		return s.yearMapYears(req)
	case "yearMapScan":
		return s.yearMapScan(ctx, req)
	case "yearMapLayout":
		return s.yearMapLayout(ctx, req)
	case "yearMapPdf":
		return s.yearMapPdf(ctx, req)
	case "travelTrips":
		return s.travelTrips(ctx, req)
	case "travelScan":
		return s.travelScan(ctx, req)
	case "travelLayout":
		return s.travelLayout(ctx, req)
	case "travelPdf":
		return s.travelPdf(ctx, req)
	default:
		return fmt.Errorf("unknown op %q", req.Op)
	}
	return nil
}

// statusThrottle drops status events sent less than statusInterval apart,
// except stage changes and the last one of each stage.
type statusThrottle struct {
	lock  sync.Mutex
	stage string
	last  time.Time
}

func (t *statusThrottle) allow(stage string, done, total int) bool {
	t.lock.Lock()
	defer t.lock.Unlock()
	now := time.Now()
	if stage == t.stage && done < total && now.Sub(t.last) < statusInterval {
		return false
	}
	t.stage, t.last = stage, now
	return true
}

func (s *server) plan(ctx context.Context, req request) error {
	s.busy.Add(1)
	defer s.busy.Add(-1)
	var (
		pendingLock sync.Mutex
		entries     []sorter.Entry
		duplicates  []sorter.Duplicate
		throttle    statusThrottle
	)
	flush := func() {
		pendingLock.Lock()
		batch, dups := entries, duplicates
		entries, duplicates = nil, nil
		pendingLock.Unlock()
		if len(batch)+len(dups) > 0 {
			s.send(event{"id": req.ID, "type": "draft", "entries": batch, "duplicates": dups})
		}
	}
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(draftInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				flush()
			case <-stop:
				return
			}
		}
	}()
	engine := sorter.Engine{
		ByLocation: req.ByLocation, Landmarks: req.Landmarks, SkipDuplicates: req.Dedupe, GroupSimilar: req.Similar,
		DateFallback: req.DateFallback, ScreenshotFolder: req.Screenshots, GroupBlurry: req.Blurry,
		Progress: func(stage string, done, total int) {
			if throttle.allow(stage, done, total) {
				s.send(event{"id": req.ID, "type": "status", "stage": stage, "done": done, "total": total})
			}
		},
		Draft: func(entry sorter.Entry) {
			pendingLock.Lock()
			entries = append(entries, entry)
			pendingLock.Unlock()
		},
		OnDuplicate: func(duplicate sorter.Duplicate) {
			pendingLock.Lock()
			duplicates = append(duplicates, duplicate)
			pendingLock.Unlock()
		},
		Preview: func(path string, img image.Image) {
			if data, err := thumbs.Encode(img); err == nil {
				bounds := img.Bounds()
				s.cache.put(path, data, max(bounds.Dx(), bounds.Dy()))
			}
		},
	}
	if req.Remote != nil {
		cleanup, err := prepareRemotePlan(&req)
		if err != nil {
			close(stop)
			<-stopped
			return err
		}
		defer cleanup()
	}
	plan, err := engine.Plan(ctx, req.Sources, req.Destination)
	close(stop)
	<-stopped
	if err != nil {
		return err
	}
	flush()
	s.lock.Lock()
	s.plans[req.ID] = plan
	s.lock.Unlock()
	s.send(event{"id": req.ID, "type": "plan", "plan": plan})
	return nil
}

func (s *server) thumbnail(ctx context.Context, req request) error {
	size := req.Size
	if size < 16 || size > 1024 {
		size = 256
	}
	data, ok := s.cache.get(req.Path, size)
	if !ok {
		select {
		case s.thumbs <- struct{}{}:
			defer func() { <-s.thumbs }()
		case <-ctx.Done():
			return ctx.Err()
		}
		if s.busy.Load() > 0 {
			select {
			case s.busySlots <- struct{}{}:
				defer func() { <-s.busySlots }()
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		// Another request may have produced it while this one waited.
		if data, ok = s.cache.get(req.Path, size); !ok {
			var err error
			if data, err = thumbs.JPEG(req.Path, size); err != nil {
				return err
			}
			s.cache.put(req.Path, data, size)
		}
	}
	s.send(event{"id": req.ID, "type": "thumbnail", "path": req.Path,
		"dataUrl": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)})
	return nil
}

func (s *server) tag(ctx context.Context, req request) error {
	tg, err := tagger.New(tagger.FindAssets(req.AIDir), tagger.Options{Scenes: req.TagScenes, Moment: req.TagMoment, People: req.TagPeople, Meta: req.TagMeta})
	if err != nil {
		return err
	}
	defer tg.Close()
	var paths []string
	if len(req.Files) > 0 {
		for _, path := range req.Files {
			if _, err := os.Lstat(path + ".xmp"); err != nil {
				paths = append(paths, path)
			}
		}
	} else if req.Folder == "" {
		return errors.New("no folder to tag")
	} else if paths, err = sorter.PhotosToTag(ctx, req.Folder); err != nil {
		return err
	}
	s.send(event{"id": req.ID, "type": "status", "stage": "tags", "done": 0, "total": len(paths)})
	tagged := 0
	err = sorter.TagPhotos(ctx, tg, paths, func(result sorter.TagResult) {
		e := event{"id": req.ID, "type": "tagged", "path": result.Path, "tags": result.Tags, "done": result.Done, "total": result.Total}
		if result.Err != nil {
			e["error"] = result.Err.Error()
		} else if len(result.Tags) > 0 {
			tagged++
		}
		s.send(e)
	})
	if err != nil {
		return err
	}
	s.send(event{"id": req.ID, "type": "done", "tagged": tagged, "total": len(paths)})
	return nil
}
