package sorter

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"sort-iphone-pictures/internal/geo"
	"sort-iphone-pictures/internal/tagger"
)

// UndatedFolder receives photos without a usable capture date.
const UndatedFolder = "Photos_Undated"

type Entry struct {
	Source   string   `json:"source"`
	Target   string   `json:"target"`
	Metadata Metadata `json:"metadata"`
}

type Issue struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Plan struct {
	Destination string  `json:"destination"`
	Entries     []Entry `json:"entries"`
	Skipped     []Issue `json:"skipped"`
	// Duplicates lists true duplicates left out of the plan (SkipDuplicates).
	Duplicates []Duplicate `json:"duplicates"`
	// Filtered counts photos left out because their date is outside
	// Engine.TakenFrom/TakenTo.
	Filtered int `json:"filtered"`
}

type Report struct {
	Copied int `json:"copied"`
}

// Locator maps GPS coordinates to a place name.
type Locator func(lat, lon float64) (string, bool)

type Engine struct {
	ReadMetadata MetadataReader
	// ByLocation adds a place folder inside each month when GPS data resolves.
	ByLocation bool
	// Landmarks names the folder after a nearby landmark (museum, castle,
	// stadium...) instead of the city. Only used with ByLocation.
	Landmarks bool
	// LocateLandmark overrides the embedded offline landmark database.
	LocateLandmark Locator
	// SkipDuplicates leaves out photos that are true duplicates (same bytes or
	// same decoded pixels) of an earlier source or of a file in the destination.
	SkipDuplicates bool
	// GroupSimilar puts near-identical shots (bursts, retakes) taken close in
	// time into a shared "Almost Duplicates" folder inside the month folder.
	GroupSimilar bool
	// Hash overrides the perceptual image hash (tests).
	Hash func(path string) (uint64, bool)
	// Progress, if set, reports the current planning stage ("exif", then
	// "hash" when duplicates or near-duplicates are looked for, then
	// "similar") and how many photos are done out of total.
	Progress func(stage string, done, total int)
	// Draft, if set, receives each dated photo as soon as its folder is known,
	// before duplicates and near-duplicates are resolved. Its Target is the
	// folder joined with the original file name.
	Draft func(Entry)
	// OnDuplicate, if set, receives each true duplicate as soon as it is
	// found, in source order.
	OnDuplicate func(Duplicate)
	// Preview, if set, receives the upright preview (at most PreviewSize
	// pixels, often the 160×120 Exif thumbnail) of photos looked at while
	// planning, so callers can cache it. It is called from several
	// goroutines at once.
	Preview func(path string, img image.Image)
	// Locate overrides the embedded offline city database.
	Locate Locator
	// DateFallback dates photos lacking an EXIF capture date from the date in
	// their file name, then from their modification time, instead of leaving
	// them in UndatedFolder.
	DateFallback bool
	// TakenFrom and TakenTo (inclusive, zero = unbounded) keep only photos
	// whose capture date (after DateFallback) lies in the range. With a range,
	// photos without a usable date are left out.
	TakenFrom, TakenTo time.Time
	// ScreenshotFolder files screenshots under ScreenshotsFolder instead of
	// the regular year/month tree.
	ScreenshotFolder bool
	// GroupBlurry files clearly blurry photos under BlurryFolder (inside the
	// year/month tree) so they can be reviewed; nothing is ever deleted.
	GroupBlurry bool
	// BlurThreshold is the sharpness below which a photo is blurry; zero
	// means DefaultBlurThreshold.
	BlurThreshold float64
	// Sharpness overrides the sharpness measure (tests).
	Sharpness func(path string) (float64, bool)
	// Now overrides the clock used to reject future fallback dates (tests).
	Now func() time.Time
}

// ScreenshotsFolder receives screenshots when Engine.ScreenshotFolder is set.
const ScreenshotsFolder = "Screenshots"

func (engine Engine) Plan(ctx context.Context, sources []string, destination string) (Plan, error) {
	plan := Plan{Entries: []Entry{}, Skipped: []Issue{}, Duplicates: []Duplicate{}}
	if len(sources) == 0 || strings.TrimSpace(destination) == "" {
		return plan, errors.New("at least one source and a destination are required")
	}
	var err error
	plan.Destination, err = canonicalPath(destination)
	if err != nil {
		return plan, fmt.Errorf("destination: %w", err)
	}
	if info, err := os.Stat(plan.Destination); err == nil && !info.IsDir() {
		return plan, errors.New("destination is not a directory")
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return plan, err
	}
	paths := make(map[string]struct{})
	for _, source := range sources {
		if strings.TrimSpace(source) == "" {
			return plan, errors.New("source must not be empty")
		}
		root, err := filepath.EvalSymlinks(source)
		if err != nil {
			return plan, fmt.Errorf("source %q: %w", source, err)
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return plan, err
		}
		info, err := os.Stat(root)
		if err != nil {
			return plan, err
		}
		if !info.IsDir() {
			return plan, fmt.Errorf("source %q is not a directory", source)
		}
		if within(root, plan.Destination) || within(plan.Destination, root) {
			return plan, errors.New("source and destination directories must not overlap")
		}
		err = filepath.WalkDir(root, func(path string, item fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				plan.Skipped = append(plan.Skipped, Issue{path, walkErr.Error()})
				return nil
			}
			if item.Type()&os.ModeSymlink != 0 {
				plan.Skipped = append(plan.Skipped, Issue{path, "symbolic link ignored"})
				return nil
			}
			if !item.IsDir() && item.Type().IsRegular() && supported(path) {
				paths[path] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return plan, err
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	reader := engine.ReadMetadata
	if reader == nil {
		reader = ReadMetadata
	}
	locate := engine.Locate
	if engine.ByLocation && locate == nil {
		locate = geo.Default().Nearest
	}
	landmark := engine.LocateLandmark
	if engine.ByLocation && engine.Landmarks && landmark == nil {
		landmark = geo.DefaultPlaces().Near
	}
	now := time.Now()
	if engine.Now != nil {
		now = engine.Now()
	}
	folderOf := func(path string, metadata Metadata) string {
		month := filepath.Join(metadata.TakenAt.Format("2006"), metadata.TakenAt.Format("2006-01"))
		if engine.ScreenshotFolder && tagger.IsScreenshot(path, metadata.Shot.Comment) {
			return filepath.Join(ScreenshotsFolder, month)
		}
		if !engine.ByLocation || !metadata.HasGPS {
			return month
		}
		place, found := "", false
		if engine.Landmarks && landmark != nil {
			place, found = landmark(metadata.Lat, metadata.Lon)
			found = found && placeName(place) != ""
		}
		if !found {
			place, found = locate(metadata.Lat, metadata.Lon)
		}
		if folder := placeName(place); found && folder != "" {
			return filepath.Join(month, folder)
		}
		return month
	}

	// Stage 1: EXIF and folder of every photo, on all cores. Each photo is
	// reported as a draft as soon as it is known.
	type photo struct {
		metadata Metadata
		err      error
		folder   string
	}
	photos := make([]photo, len(ordered))
	done := 0
	for i := range parallel(ctx, len(ordered), func(i int) {
		item := &photos[i]
		item.metadata, item.err = reader(ctx, ordered[i])
		readable := item.err == nil || errors.Is(item.err, errNoEXIF)
		if readable && engine.DateFallback && !validDate(item.metadata) {
			if date, source, ok := fallbackDate(ordered[i], filepath.Base(ordered[i]), now); ok {
				item.metadata.TakenAt, item.metadata.DateSource = date, source
				item.err = nil
			}
		}
		if item.err == nil && validDate(item.metadata) {
			item.folder = folderOf(ordered[i], item.metadata)
		}
	}) {
		done++
		if item := photos[i]; engine.Draft != nil {
			switch {
			case item.folder != "":
				engine.Draft(Entry{ordered[i], filepath.Join(item.folder, filepath.Base(ordered[i])), item.metadata})
			case item.err == nil || errors.Is(item.err, errNoEXIF):
				engine.Draft(Entry{ordered[i], filepath.Join(UndatedFolder, filepath.Base(ordered[i])), item.metadata})
			}
		}
		if engine.Progress != nil {
			engine.Progress("exif", done, len(ordered))
		}
	}
	if err := ctx.Err(); err != nil {
		return plan, err
	}
	var dated, undated []int
	for i, path := range ordered {
		switch item := photos[i]; {
		case item.err != nil && !errors.Is(item.err, errNoEXIF):
			plan.Skipped = append(plan.Skipped, Issue{path, item.err.Error()})
		case engine.outsideRange(item.metadata):
			plan.Filtered++
		case item.folder == "":
			undated = append(undated, i)
		default:
			dated = append(dated, i)
		}
	}

	// Stage 2: content fingerprints (each photo read and decoded once, on all
	// cores), consumed in source order so the first copy of a photo always
	// wins.
	pending := make([]candidate, 0, len(dated))
	if engine.SkipDuplicates || engine.GroupSimilar {
		var dedupe *deduper
		if engine.SkipDuplicates {
			dedupe = newDeduper(plan.Destination)
		}
		options := fingerprintOptions{
			bytes:      engine.SkipDuplicates,
			pixels:     engine.SkipDuplicates,
			perceptual: engine.GroupSimilar,
			hash:       engine.Hash,
			preview:    engine.Preview,
		}
		prints := make([]fingerprint, len(dated))
		ready := make([]bool, len(dated))
		// Only photos with a neighbour close in time can be near-duplicates:
		// the others need no perceptual hash, often the costliest part.
		var nearby []bool
		if engine.GroupSimilar {
			times, folders := make([]time.Time, len(dated)), make([]string, len(dated))
			for k, i := range dated {
				times[k], folders[k] = photos[i].metadata.TakenAt, photos[i].folder
			}
			nearby = closeInTime(times, folders)
		}
		// Costliest photos first, so a slow decode never runs alone at the end.
		paths := make([]string, len(dated))
		for k, i := range dated {
			paths[k] = ordered[i]
		}
		schedule := costliestFirst(paths)
		next, done := 0, 0
		for j := range parallel(ctx, len(dated), func(j int) {
			k := schedule[j]
			own := options
			own.perceptual = options.perceptual && nearby[k]
			prints[k] = fingerprintOf(paths[k], own)
		}) {
			k := schedule[j]
			ready[k] = true
			for ; next < len(dated) && ready[next]; next++ {
				path, item := ordered[dated[next]], photos[dated[next]]
				if dedupe != nil {
					if of, found := dedupe.check(path, prints[next]); found {
						duplicate := Duplicate{path, of, within(plan.Destination, of)}
						plan.Duplicates = append(plan.Duplicates, duplicate)
						if engine.OnDuplicate != nil {
							engine.OnDuplicate(duplicate)
						}
						continue
					}
				}
				pending = append(pending, candidate{path, item.metadata, item.folder, prints[next].imageSum, prints[next].imageOK})
			}
			done++
			if engine.Progress != nil {
				engine.Progress("hash", done, len(dated))
			}
		}
		if err := ctx.Err(); err != nil {
			return plan, err
		}
	} else {
		for _, i := range dated {
			pending = append(pending, candidate{path: ordered[i], metadata: photos[i].metadata, month: photos[i].folder})
		}
	}
	if engine.GroupBlurry {
		threshold, measure := engine.BlurThreshold, engine.Sharpness
		if threshold == 0 {
			threshold = DefaultBlurThreshold
		}
		if measure == nil {
			measure = sharpnessOf
		}
		blurry := make([]bool, len(pending))
		done := 0
		for range parallel(ctx, len(pending), func(i int) {
			item := pending[i]
			if tagger.IsScreenshot(item.path, item.metadata.Shot.Comment) {
				return
			}
			value, ok := measure(item.path)
			blurry[i] = ok && value < threshold
		}) {
			done++
			if engine.Progress != nil {
				engine.Progress("blur", done, len(pending))
			}
		}
		if err := ctx.Err(); err != nil {
			return plan, err
		}
		for i, isBlurry := range blurry {
			if isBlurry {
				pending[i].month = filepath.Join(BlurryFolder, pending[i].month)
			}
		}
	}
	if engine.GroupSimilar {
		if engine.Progress != nil {
			engine.Progress("similar", len(pending), len(pending))
		}
		groups, err := similarGroups(ctx, pending)
		if err != nil {
			return plan, err
		}
		for index, group := range groups {
			if group != "" {
				pending[index].month = filepath.Join(pending[index].month, group)
			}
		}
	}
	reserved := make(map[string]bool)
	counters := make(map[string]int)
	for _, item := range pending {
		path, metadata, month := item.path, item.metadata, item.month
		prefix := metadata.TakenAt.Format("2006-01-02") + " - " + cameraName(metadata.Camera)
		key := strings.ToLower(filepath.Join(month, prefix))
		var target string
		for {
			if err := ctx.Err(); err != nil {
				return plan, err
			}
			counters[key]++
			target = filepath.Join(month, fmt.Sprintf("%s - %d%s", prefix, counters[key], strings.ToLower(filepath.Ext(path))))
			if reserved[strings.ToLower(target)] {
				continue
			}
			_, err := os.Lstat(filepath.Join(plan.Destination, target))
			if errors.Is(err, fs.ErrNotExist) {
				break
			}
			if err != nil {
				return plan, fmt.Errorf("check target: %w", err)
			}
		}
		reserved[strings.ToLower(target)] = true
		plan.Entries = append(plan.Entries, Entry{path, target, metadata})
	}
	// Photos without a capture date are renamed "Photo undated - N" in one default folder
	// so none is forgotten after sorting; numbering is incremental.
	undatedCount := 0
	for _, i := range undated {
		if err := ctx.Err(); err != nil {
			return plan, err
		}
		path := ordered[i]
		extension := strings.ToLower(filepath.Ext(path))
		var target string
		for {
			undatedCount++
			target = filepath.Join(UndatedFolder, fmt.Sprintf("Photo undated - %d%s", undatedCount, extension))
			if reserved[strings.ToLower(target)] {
				continue
			}
			_, err := os.Lstat(filepath.Join(plan.Destination, target))
			if errors.Is(err, fs.ErrNotExist) {
				break
			}
			if err != nil {
				return plan, fmt.Errorf("check target: %w", err)
			}
		}
		reserved[strings.ToLower(target)] = true
		plan.Entries = append(plan.Entries, Entry{path, target, photos[i].metadata})
	}
	return plan, nil
}

func Copy(ctx context.Context, plan Plan) (Report, error) {
	return CopyWithProgress(ctx, plan, nil)
}

// copyWorkers bounds concurrent copies: enough to hide fsync latency without
// thrashing a spinning or network disk.
const copyWorkers = 4

// CopyWithProgress copies the plan with a few concurrent workers. onCopied is
// never called concurrently, but entries may complete out of plan order.
func CopyWithProgress(ctx context.Context, plan Plan, onCopied func(Entry)) (Report, error) {
	report := Report{}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if !filepath.IsAbs(plan.Destination) {
		return report, errors.New("plan destination must be absolute")
	}
	for _, entry := range plan.Entries {
		if !filepath.IsLocal(entry.Target) || entry.Target == "." {
			return report, fmt.Errorf("unsafe target %q", entry.Target)
		}
	}
	if len(plan.Entries) == 0 {
		return report, nil
	}
	if err := os.MkdirAll(plan.Destination, 0755); err != nil {
		return report, err
	}
	root, err := os.OpenRoot(plan.Destination)
	if err != nil {
		return report, err
	}
	defer root.Close()
	var (
		jobs    = make(chan Entry)
		failed  = make(chan struct{})
		wg      sync.WaitGroup
		lock    sync.Mutex
		failure error
	)
	// Copies overlap their reads, writes and fsyncs; onCopied stays serialized.
	for range min(copyWorkers, len(plan.Entries)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobs {
				err := copyEntry(ctx, root, entry)
				lock.Lock()
				if err == nil {
					report.Copied++
					if onCopied != nil {
						onCopied(entry)
					}
				} else if failure == nil {
					failure = fmt.Errorf("copy %q: %w", entry.Source, err)
					close(failed)
				}
				lock.Unlock()
			}
		}()
	}
	// After a failure no new copy starts, but those in flight finish and are kept.
feed:
	for _, entry := range plan.Entries {
		select {
		case <-failed:
			break feed
		case <-ctx.Done():
			break feed
		default:
		}
		select {
		case jobs <- entry:
		case <-failed:
			break feed
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if failure != nil {
		return report, failure
	}
	if report.Copied < len(plan.Entries) {
		return report, ctx.Err()
	}
	return report, nil
}

func copyEntry(ctx context.Context, root *os.Root, entry Entry) error {
	source, err := os.Open(entry.Source)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("source is not a regular file")
	}
	directory := ""
	for _, part := range strings.Split(filepath.Dir(entry.Target), string(filepath.Separator)) {
		directory = filepath.Join(directory, part)
		if err := root.Mkdir(directory, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	target, err := root.OpenFile(entry.Target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, contextReader{ctx, source})
	if copyErr == nil {
		copyErr = target.Sync()
	}
	closeErr := target.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return errors.Join(err, root.Remove(entry.Target))
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func (engine Engine) outsideRange(m Metadata) bool {
	if engine.TakenFrom.IsZero() && engine.TakenTo.IsZero() {
		return false
	}
	if !validDate(m) {
		return true
	}
	return (!engine.TakenFrom.IsZero() && m.TakenAt.Before(engine.TakenFrom)) ||
		(!engine.TakenTo.IsZero() && m.TakenAt.After(engine.TakenTo))
}

func validDate(m Metadata) bool {
	return !m.TakenAt.IsZero() && m.TakenAt.Year() >= 1 && m.TakenAt.Year() <= 9999
}

func supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".tif", ".tiff", ".heic", ".heif", ".avif", ".dng", ".cr2", ".cr3", ".nef", ".arw", ".rw2", ".crw":
		return true
	}
	return false
}

// IsPhoto reports whether path has the extension of a supported photo.
func IsPhoto(path string) bool {
	return supported(path)
}

func cameraName(camera string) string {
	if camera = sanitize(camera, 40); camera == "" {
		return "Unknown camera"
	}
	return camera
}

func placeName(place string) string {
	return sanitize(place, 60)
}

func sanitize(camera string, limit int) string {
	camera = strings.Map(func(char rune) rune {
		if unicode.IsControl(char) || strings.ContainsRune(`<>:"/\|?*`, char) {
			return '-'
		}
		return char
	}, camera)
	camera = strings.Trim(strings.Join(strings.Fields(camera), " "), " .-")
	characters := []rune(camera)
	if len(characters) > limit {
		camera = strings.Trim(string(characters[:limit]), " .-")
	}
	return camera
}

func within(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && (relative == "." || filepath.IsLocal(relative))
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(absolute)
	if parent == absolute {
		return "", err
	}
	resolved, err = canonicalPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(absolute)), nil
}
