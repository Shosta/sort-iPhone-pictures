package tagger

import (
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Tags derived from the metadata alone, no model involved.
const (
	TagScreenshot = "Capture d'écran"
	TagSelfie     = "Selfie"
	TagPanorama   = "Panorama"
	TagLowLight   = "Faible luminosité"
	TagZoom       = "Zoom"
	TagMacro      = "Macro"
	TagWeekend    = "Week-end"
	TagNewYear    = "Jour de l'an"
	TagGroup      = "Groupe"
	TagSpring     = "Printemps"
	TagSummer     = "Été"
	TagAutumn     = "Automne"
	TagWinter     = "Hiver"
)

const (
	panoramaRatio  = 2.5 // long side over short side
	zoomFocal35    = 70  // mm, 35 mm equivalent
	macroDistance  = 0.15
	lowLightBV     = 0.0  // APEX brightness value at or below which the scene is dim
	lowLightISO    = 1600 // used when the camera gives no brightness value
	groupMinFaces  = 4
	firstValidYear = 1990
)

// Shot is the EXIF capture information the metadata tags are derived from.
type Shot struct {
	Lens, Comment   string
	ISO             int
	Brightness      float64 // APEX brightness value, meaningful if HasBrightness
	HasBrightness   bool
	Focal35         float64 // focal length in 35 mm equivalent, 0 if unknown
	SubjectDistance float64 // metres, 0 if unknown
	Width, Height   int     // pixels, 0 if unknown
}

// metaTags returns the tags the photo's metadata and file name give away.
func metaTags(photo Photo) []string {
	var tags []string
	shot := photo.Shot
	screenshot := isScreenshot(photo.Path, shot.Comment)
	if screenshot {
		tags = append(tags, TagScreenshot)
	}
	if !screenshot {
		if isFrontCamera(shot.Lens) {
			tags = append(tags, TagSelfie)
		}
		if w, h := dimensions(photo); w > 0 && h > 0 && float64(max(w, h)) >= panoramaRatio*float64(min(w, h)) {
			tags = append(tags, TagPanorama)
		}
		if shot.Focal35 >= zoomFocal35 {
			tags = append(tags, TagZoom)
		}
		if isMacro(shot) {
			tags = append(tags, TagMacro)
		}
		if isLowLight(shot) {
			tags = append(tags, TagLowLight)
		}
	}
	return append(tags, calendarTags(photo.TakenAt, photo.Lat, photo.HasGPS)...)
}

func isScreenshot(path, comment string) bool {
	if strings.Contains(strings.ToLower(comment), "screenshot") {
		return true
	}
	name := strings.ToLower(filepath.Base(path))
	for _, prefix := range []string{"screenshot", "screen shot", "capture d"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isFrontCamera(lens string) bool {
	return strings.Contains(strings.ToLower(lens), "front")
}

func isMacro(shot Shot) bool {
	if strings.Contains(strings.ToLower(shot.Lens), "macro") {
		return true
	}
	return shot.SubjectDistance > 0 && shot.SubjectDistance <= macroDistance
}

func isLowLight(shot Shot) bool {
	if shot.HasBrightness {
		return shot.Brightness <= lowLightBV
	}
	return shot.ISO >= lowLightISO
}

// dimensions returns the pixel size from the EXIF data or, failing that, the
// image header.
func dimensions(photo Photo) (int, int) {
	if photo.Shot.Width > 0 && photo.Shot.Height > 0 {
		return photo.Shot.Width, photo.Shot.Height
	}
	file, err := os.Open(photo.Path)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

// calendarTags returns the season, year, weekend and new year tags for the
// local capture time. Seasons are swapped in the southern hemisphere.
func calendarTags(takenAt time.Time, lat float64, hasGPS bool) []string {
	if takenAt.IsZero() || takenAt.Year() < firstValidYear {
		return nil
	}
	month := int(takenAt.Month())
	if hasGPS && lat < 0 {
		month = (month+5)%12 + 1
	}
	tags := []string{season(month), strconv.Itoa(takenAt.Year())}
	if wd := takenAt.Weekday(); wd == time.Saturday || wd == time.Sunday {
		tags = append(tags, TagWeekend)
	}
	if takenAt.Month() == time.January && takenAt.Day() == 1 {
		tags = append(tags, TagNewYear)
	}
	return tags
}

// season uses the meteorological seasons of the northern hemisphere.
func season(month int) string {
	switch {
	case month >= 3 && month <= 5:
		return TagSpring
	case month >= 6 && month <= 8:
		return TagSummer
	case month >= 9 && month <= 11:
		return TagAutumn
	}
	return TagWinter
}
