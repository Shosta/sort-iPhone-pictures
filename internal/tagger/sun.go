package tagger

import (
	"math"
	"time"
)

// Moment tags.
const (
	TagSunrise = "Lever de soleil"
	TagSunset  = "Coucher de soleil"
	TagNight   = "Nuit"
)

// sunElevation returns the solar elevation in degrees at the given instant and
// place (NOAA approximation), plus the local solar hour (0-24).
func sunElevation(utc time.Time, lat, lon float64) (elevation, solarHour float64) {
	utc = utc.UTC()
	dayOfYear := float64(utc.YearDay())
	hours := float64(utc.Hour()) + float64(utc.Minute())/60 + float64(utc.Second())/3600
	gamma := 2 * math.Pi / 365 * (dayOfYear - 1 + (hours-12)/24)
	equationOfTime := 229.18 * (0.000075 + 0.001868*math.Cos(gamma) - 0.032077*math.Sin(gamma) -
		0.014615*math.Cos(2*gamma) - 0.040849*math.Sin(2*gamma))
	declination := 0.006918 - 0.399912*math.Cos(gamma) + 0.070257*math.Sin(gamma) -
		0.006758*math.Cos(2*gamma) + 0.000907*math.Sin(2*gamma) -
		0.002697*math.Cos(3*gamma) + 0.00148*math.Sin(3*gamma)
	trueSolarMinutes := math.Mod(hours*60+equationOfTime+4*lon+1440*2, 1440)
	hourAngle := trueSolarMinutes/4 - 180
	latRad, haRad := lat*math.Pi/180, hourAngle*math.Pi/180
	sinElevation := math.Sin(latRad)*math.Sin(declination) + math.Cos(latRad)*math.Cos(declination)*math.Cos(haRad)
	return math.Asin(math.Max(-1, math.Min(1, sinElevation))) * 180 / math.Pi, trueSolarMinutes / 60
}

// momentTag classifies a photo as sunrise, sunset or night. takenAt holds the
// local wall-clock time; offsetSeconds is its UTC offset when known, otherwise
// it is estimated from the longitude.
func momentTag(takenAt time.Time, lat, lon float64, offsetSeconds int, hasOffset bool) string {
	if takenAt.IsZero() {
		return ""
	}
	if !hasOffset {
		offsetSeconds = int(math.Round(lon/15)) * 3600
	}
	wall := takenAt
	instant := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), 0, time.UTC).
		Add(-time.Duration(offsetSeconds) * time.Second)
	elevation, solarHour := sunElevation(instant, lat, lon)
	switch {
	case elevation < -8:
		return TagNight
	case elevation >= -4 && elevation <= 6:
		if solarHour < 12 {
			return TagSunrise
		}
		return TagSunset
	}
	return ""
}

// isDaylight reports whether the sun is clearly up (used for sky tags).
func isDaylight(takenAt time.Time, lat, lon float64, offsetSeconds int, hasOffset, hasGPS bool) bool {
	if takenAt.IsZero() {
		return true
	}
	if !hasGPS {
		hour := takenAt.Hour()
		return hour >= 8 && hour < 19
	}
	if !hasOffset {
		offsetSeconds = int(math.Round(lon/15)) * 3600
	}
	instant := time.Date(takenAt.Year(), takenAt.Month(), takenAt.Day(), takenAt.Hour(), takenAt.Minute(), takenAt.Second(), 0, time.UTC).
		Add(-time.Duration(offsetSeconds) * time.Second)
	elevation, _ := sunElevation(instant, lat, lon)
	return elevation > 8
}
