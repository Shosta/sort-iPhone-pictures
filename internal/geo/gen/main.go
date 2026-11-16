// Command gen builds internal/geo/cities.tsv.gz from a GeoNames cities1000.txt dump.
//
//	go run ./internal/geo/gen cities1000.txt internal/geo/cities.tsv.gz
//
// With the "poi" mode it builds internal/geo/places.tsv.gz from a GeoNames
// allCountries.txt dump, keeping landmarks in Europe, Canada and the US:
//
//	go run ./internal/geo/gen poi allCountries.txt internal/geo/places.tsv.gz
package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) == 4 && os.Args[1] == "poi" {
		if err := runPlaces(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gen <cities1000.txt> <cities.tsv.gz>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(inPath, outPath string) error {
	in, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zip, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(zip)
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	count := 0
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		// PPLX are city sections (districts), which would shadow the city itself.
		if len(fields) < 15 || fields[7] == "PPLX" {
			continue
		}
		lat, errLat := strconv.ParseFloat(fields[4], 64)
		lon, errLon := strconv.ParseFloat(fields[5], 64)
		if errLat != nil || errLon != nil || fields[1] == "" {
			continue
		}
		population, _ := strconv.Atoi(fields[14])
		fmt.Fprintf(writer, "%s\t%.4f\t%.4f\t%d\n", fields[1], lat, lon, population)
		count++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if err := zip.Close(); err != nil {
		return err
	}
	fmt.Println("cities:", count)
	return nil
}

// placeRank orders landmark types: the larger, the more likely a photo
// taken nearby is "of" that place (a museum beats its courtyard).
var placeRank = map[string]int{
	"MUS": 3, "STDM": 3, "ARNA": 3, "THTR": 3, "OPRA": 3, "ZOO": 3, "AMTH": 3,
	"CSTL": 2, "PAL": 2, "PYR": 2, "PYRS": 2, "FT": 2,
	"MNMT": 1, "HSTS": 1, "RUIN": 1,
}

var placeCountries = func() map[string]bool {
	set := make(map[string]bool)
	for _, code := range strings.Fields("AD AL AT AX BA BE BG BY CH CY CZ DE DK EE ES FI FO FR GB GG GI GR HR HU IE IM IS IT JE LI LT LU LV MC MD ME MK MT NL NO PL PT RO RS RU SE SI SJ SK SM UA VA XK CA US") {
		set[code] = true
	}
	return set
}()

func runPlaces(inPath, outPath string) error {
	in, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zip, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(zip)
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	count := 0
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 9 || !placeCountries[fields[8]] || fields[1] == "" {
			continue
		}
		rank := placeRank[fields[7]]
		lat, errLat := strconv.ParseFloat(fields[4], 64)
		lon, errLon := strconv.ParseFloat(fields[5], 64)
		if rank == 0 || errLat != nil || errLon != nil {
			continue
		}
		fmt.Fprintf(writer, "%s\t%.5f\t%.5f\t%d\n", fields[1], lat, lon, rank)
		count++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if err := zip.Close(); err != nil {
		return err
	}
	fmt.Println("places:", count)
	return nil
}
