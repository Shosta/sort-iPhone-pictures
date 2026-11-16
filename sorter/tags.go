package sorter

import (
	"path/filepath"

	"sort-iphone-pictures/internal/tagger"
)

// TagHook returns a CopyWithHook afterCopy function that analyses each source
// photo and writes its keywords to "<copied file>.xmp". Tagging is best effort:
// a failure never affects the copy.
func TagHook(tg *tagger.Tagger, destination string) func(Entry) {
	return func(entry Entry) {
		m := entry.Metadata
		tags, _ := tg.Tags(tagger.Photo{Path: entry.Source, TakenAt: m.TakenAt, Lat: m.Lat, Lon: m.Lon,
			HasGPS: m.HasGPS, UTCOffset: m.UTCOffset, HasOffset: m.HasOffset})
		if len(tags) > 0 {
			_ = tagger.WriteXMP(filepath.Join(destination, entry.Target)+".xmp", tags)
		}
	}
}
