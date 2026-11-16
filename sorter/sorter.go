package sorter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"sort-iphone-pictures/internal/geo"
)

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
	// Locate overrides the embedded offline city database.
	Locate Locator
}

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
	var dedupe *deduper
	if engine.SkipDuplicates {
		dedupe = newDeduper(plan.Destination)
	}
	var pending []candidate
	reserved := make(map[string]bool)
	counters := make(map[string]int)
	for _, path := range ordered {
		if err := ctx.Err(); err != nil {
			return plan, err
		}
		metadata, err := reader(ctx, path)
		if err != nil {
			if ctx.Err() != nil {
				return plan, ctx.Err()
			}
			plan.Skipped = append(plan.Skipped, Issue{path, err.Error()})
			continue
		}
		if metadata.TakenAt.IsZero() || metadata.TakenAt.Year() < 1 || metadata.TakenAt.Year() > 9999 {
			plan.Skipped = append(plan.Skipped, Issue{path, "missing or invalid original capture date"})
			continue
		}
		if dedupe != nil {
			if of, found := dedupe.check(path); found {
				plan.Duplicates = append(plan.Duplicates, Duplicate{path, of, within(plan.Destination, of)})
				continue
			}
		}
		month := filepath.Join(metadata.TakenAt.Format("2006"), metadata.TakenAt.Format("2006-01"))
		if engine.ByLocation && metadata.HasGPS {
			place, found := "", false
			if engine.Landmarks && landmark != nil {
				place, found = landmark(metadata.Lat, metadata.Lon)
				if found && placeName(place) == "" {
					found = false
				}
			}
			if !found {
				place, found = locate(metadata.Lat, metadata.Lon)
			}
			if found {
				if folder := placeName(place); folder != "" {
					month = filepath.Join(month, folder)
				}
			}
		}
		pending = append(pending, candidate{path, metadata, month})
	}
	if engine.GroupSimilar {
		hasher := engine.Hash
		if hasher == nil {
			hasher = imageHash
		}
		groups, err := similarGroups(ctx, pending, hasher)
		if err != nil {
			return plan, err
		}
		for index, group := range groups {
			if group != "" {
				pending[index].month = filepath.Join(pending[index].month, group)
			}
		}
	}
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
	return plan, nil
}

func Copy(ctx context.Context, plan Plan) (Report, error) {
	return CopyWithProgress(ctx, plan, nil)
}

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
	for _, entry := range plan.Entries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if err := copyEntry(ctx, root, entry); err != nil {
			return report, fmt.Errorf("copy %q: %w", entry.Source, err)
		}
		report.Copied++
		if onCopied != nil {
			onCopied(entry)
		}
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

func supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".tif", ".tiff", ".heic", ".heif", ".avif", ".dng", ".cr2", ".cr3", ".nef", ".arw", ".rw2", ".crw":
		return true
	}
	return false
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
