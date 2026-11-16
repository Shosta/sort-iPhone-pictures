package geo

import "testing"

func TestNearest(t *testing.T) {
	index := Default()
	if index.Len() < 100000 {
		t.Fatalf("embedded database too small: %d", index.Len())
	}
	for _, test := range []struct {
		lat, lon float64
		want     string
	}{
		{49.4432, 1.0993, "Rouen"},
		{48.8566, 2.3522, "Paris"},
		{43.6047, 1.4442, "Toulouse"},
		{-33.8688, 151.2093, "Sydney"},
	} {
		if got, ok := index.Nearest(test.lat, test.lon); !ok || got != test.want {
			t.Errorf("Nearest(%v, %v) = %q, %v; want %q", test.lat, test.lon, got, ok, test.want)
		}
	}
	if got, ok := index.Nearest(0, -30); ok {
		t.Errorf("mid-ocean point resolved to %q", got)
	}
	if _, ok := index.Nearest(91, 0); ok {
		t.Error("invalid latitude resolved")
	}
}

func TestPlacesNear(t *testing.T) {
	places := DefaultPlaces()
	if places.Len() == 0 {
		t.Fatal("empty landmark database")
	}
	if name, ok := places.Near(48.8610, 2.3358); !ok || name != "Musée du Louvre" {
		t.Fatalf("Louvre = %q, %v", name, ok)
	}
	if name, ok := places.Near(48.9245, 2.3601); !ok || name != "Stade de France" {
		t.Fatalf("Stade de France = %q, %v", name, ok)
	}
	if name, ok := places.Near(47.616, 1.517); !ok || name != "Château de Chambord" {
		t.Fatalf("Chambord = %q, %v", name, ok)
	}
	if name, ok := places.Near(45.0, -30.0); ok {
		t.Fatalf("open sea = %q", name)
	}
}

func TestDistrictsMapToCity(t *testing.T) {
	names := map[string]bool{"Paris": true, "Marseille": true}
	for name, want := range map[string]string{
		"Paris 15 Vaugirard": "Paris",
		"Marseille 08":       "Marseille",
		"Zona 179":           "Zona 179",
		"Rouen":              "Rouen",
	} {
		if got := cityOfDistrict(name, names); got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
}
