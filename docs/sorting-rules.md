# Sorting rules

Example destination layout:

```text
2026/2026-10/2026-10-20 - iPhone - 1.png
2026/2026-10/2026-10-20 - iPhone - 2.png
```

- Recursive scan, with a stable processing order by source path.
- The date comes exclusively from the EXIF capture date (`OriginalDate`); the file modification date is never substituted (except by the optional date fallback of the Electron app, which uses the filename date, then the file date).
- The camera model comes from EXIF, otherwise `Unknown camera`. Unsafe characters in the model are replaced; the model is limited to 40 characters.
- Numbering starts at 1 per day and model, avoiding names already present in the destination and names reserved in the current plan.
- `--by-location` (CLI): if the photo has GPS data, a city subfolder is added inside the month (`2026/2026-06/Rouen/...`). Without GPS, or with no city within 50 km, the photo stays at the root of the month. The city is found offline in an embedded GeoNames database (`internal/geo`, local names, CC BY 4.0 licence, https://www.geonames.org); no coordinate leaves the machine. Nearby districts and suburbs are grouped under the most populated city (6 km radius around the nearest locality). Disabled by default.
- `--landmarks`: if the photo is within ~120 m of a monument, or ~250 m of a museum, castle, palace, stadium, hall or theatre (Europe, Canada, United States; GeoNames database `internal/geo/places.tsv.gz`), the folder carries the place name (`.../Musee du Louvre/...`); otherwise it is the city (best effort).
  - Regenerate the places: `go run ./internal/geo/gen poi allCountries.txt internal/geo/places.tsv.gz`.
  - Regenerate the city database: `go run ./internal/geo/gen cities1000.txt internal/geo/cities.tsv.gz`.
- The extension is kept, normalised to lowercase; no image conversion.
- Overlapping sources are de-duplicated by canonical path. Content duplicates in distinct files are not detected by the CLI sort (the Electron app can skip them).
- Symbolic links met during the scan are ignored. A source folder explicitly given through a link is resolved before the scan.
- Source and destination must be disjoint, including through a symbolic link.
- Images without a date, unreadable, or with invalid EXIF are skipped with a reason. Files with unrecognised extensions are not examined.
- Photos without a valid capture date (or without EXIF) are copied to the `Photos_Undated` folder, renamed `Photo undated - N.ext` (incremental N).
- Copy only: originals are never renamed, moved or deleted. Binary content and EXIF are kept, but filesystem dates and permissions are not. Copies are created with private permissions.
- No existing file is ever overwritten, even if it appears after planning. The copy stops on error; earlier copies remain. A partially copied file is removed on error or managed cancellation. A hard process kill may leave an incomplete file.
- Running again copies the photos again with new numbers: this is not yet a synchronisation or a de-duplication.

## Supported formats

The [imagemeta](https://github.com/evanoberholster/imagemeta) decoder is a Go dependency; the CLI needs neither ExifTool, a graphics engine nor a C compiler. Examined extensions: JPEG, PNG, TIFF, HEIC/HEIF, AVIF, DNG, CR2/CR3, NEF, ARW, RW2 and CRW. Whether they can be used depends on supported EXIF being present. PNG without an EXIF block, videos, Live Photos and XMP/AAE companion files are not handled. The real formats of your cameras remain to be tested; the built-in tests use a small TIFF with EXIF generated in memory.

## Engine API

`sorter.Engine.Plan(ctx, sources, destination)` prepares a plan without writing. `sorter.Copy(ctx, plan)` runs the copies and returns their number, even if an error occurs afterwards. Individual read errors are listed in `plan.Skipped`. Both operations accept a cancellation context. `internal/desktop` and the CLI reuse the same package. `sorter.CopyWithProgress` adds one notification per successful copy for UIs. `Engine.ReadMetadata` lets tests substitute the EXIF reader. The plan is produced and consumed within the same run; the CLI does not reload JSON plans. Sources must not change between analysis and copy.
