# Weekly scrapbooking sheet (PDF): specification

Date: 2026-11-16 · Branch: `feat/weekly-scrapbook-pdf` · Status: **validated and implemented** (see "Differences from the implementation")

## Goal

From the sorted root folder (`Root/2026/2026-05/…jpg`), choose a week
in a calendar, see that week's photos, select some, and get a
printable PDF: a grid of photos to cut out and stick into the weekly planner.
The PDF opens automatically at the end. No original is modified.

New 📅 button in the header, next to 🏷️ (shortcut `Cmd/Ctrl+P`), which opens
a dedicated panel, modelled on `TagsPanel`. The main screen gains only
one button: the rule of 3 actions maximum is respected.

## Decisions

Confirmed by the user (✔) or adopted by default (assumption):

| # | Topic | Decision |
| --- | --- | --- |
| 1 | Photo size ✔ | **Long side = 6.5 cm, original ratio kept**: portrait 6.5 cm high, landscape 6.5 cm wide (i.e. 4.9 × 6.5 cm or 6.5 × 4.9 cm for a 3:4 iPhone). Adjustable value (5 to 8 cm) |
| 2 | Space between photos ✔ | **At least 5 mm** between two photos, horizontally and vertically, to make cutting easier. Default value 5 mm, adjustable from 5 to 10 mm (never below 5 mm) |
| 3 | Start of week ✔ | Monday → Sunday |
| 4 | Initial selection ✔ | No photo ticked; "tick all / untick all" buttons |
| 5 | Page format ✔ | **A4 landscape** (297 × 210 mm) by default, 10 mm margins (printers' non-printable area); A4 portrait and Letter as options; reminder to print at 100% |
| 6 | Output folder ✔ | Chosen in the **preferences** (⚙️), **Downloads** by default; name `Semaine 2026-S20 (11-17 mai).pdf`, suffix ` (2)` if the file already exists |
| 7 | Near-duplicates ✔ | **Included**: the "… - Almost Duplicates" folders are walked like the others; only non-image files are ignored |
| 8 | Cropping (assumption) | None: each photo keeps its framing and shape, printed without distortion (EXIF orientation applied) |
| 9 | Photo date (assumption) | `YYYY-MM-DD` prefix of the file name (already written by the sort), otherwise EXIF date, otherwise modification date |
| 10 | Cutting aid (assumption) | Thin grey rule around each photo; the space between photos serves as the cutting zone |

Out of scope (YAGNI, possible later): date caption (useless with a
5 mm cutting space), individual size per photo (S/M/L), wider panoramas
(they follow the long-side rule in v1), smart cropping on
faces (YuNet already exists), filtering by tags, manual drag-and-drop
layout.

## Approaches studied

- **A. PDF in Go (recommended)**: layout engine and PDF generation in the
  Go core with `github.com/go-pdf/fpdf` (MIT, v0.9.0). Consistent with "the business
  logic stays in Go", parallelisable (goroutines), low memory, no
  runtime dependency (no Chromium).
- **B. Electron's `printToPDF` (HTML/CSS)**: easy layout, but the logic
  leaves Go, images go through the renderer, high memory, rendering
  dependent on Chromium.
- **C. PPTX export**: reproduces the old process, but does not automate
  printing and adds a heavy dependency.

→ **A**.

## Architecture

```text
internal/scrapbook/            (new, pure Go, no network I/O)
  week.go      Week(date, firstDay) -> [start, end] ; labels ("11-17 mai")
  scan.go      ScanWeek(root, week) -> []Photo ; DayCounts(root, month) -> map[day]n
               (recursive under Root/YYYY/YYYY-MM, place subfolders and
               "Almost Duplicates" included)
  layout.go    Layout(page, opts, []Size) -> [][]Placement   (pure, tested: row placement)
  render.go    Render(photos, opts, out, progress)    (fpdf + resizing)
cmd/photosort-sidecar/main.go  new JSON protocol operations
photosort-electron/src/renderer/components/ScrapbookPanel.tsx (+ Calendar.tsx)
```

### Sidecar operations (line-by-line JSON, like `tag`)

- `weekDays` `{root, month}` → `{days: {"2026-05-12": 7, …}}`: number of photos per
  day, to mark the calendar. Walk limited to `Root/YYYY/YYYY-MM`.
- `weekPhotos` `{root, from, to}` → streamed `photo` events `{path, date, w, h}`
  (the list shows from the first photo; `w`×`h` = dimensions after
  EXIF orientation, read from the header without a full decode), then `done`. Thumbnails
  reuse the existing `thumbnail` op (cache, rate limiting).
- `scrapbookLayout` `{page, longSide, gutter, sizes:[{w,h}…]}` → pages of positions
  in mm. `gutter` is brought up to a 5 mm minimum on the Go side. Single source of truth:
  the UI preview draws these positions, and so does the PDF.
- `scrapbook` `{files, options, outDir, name}` → `progress {done,total}` events
  then `done {path, pages}`. `outDir` comes from the preferences (Downloads by
  default), created if missing. Cancellable (`cancel`), like the other operations.

### Layout (`layout.go`)

Each photo is scaled so that its long side equals `longSide`
(6.5 cm), without cropping. To keep cutting simple (edge-to-edge cuts
first in rows, then vertical), placement is done **by rows**, with
a space of at least `gutter` (5 mm) between two neighbouring photos and between two
rows:

1. Photos are separated by orientation (portrait / landscape / square), the
   chronological order being kept within each group.
2. Rows are filled from left to right; the height of a row is
   that of its tallest photo. The remaining photos of each group are
   gathered into a last mixed row to limit paper waste.
3. Rows are stacked page after page; a row never splits across pages.

Capacity on A4 landscape (usable area 277 × 190 mm, 5 mm space) for 3:4 photos:
5 portraits of 48.75 mm per row over 2 rows, i.e. **10 per page**; or
4 landscapes of 65 mm per row over 3 rows of 48.75 mm, i.e. **12 per page**.
A page may mix rows of portraits and landscapes (for example 1 row of
portraits + 2 rows of landscapes = 172.5 mm high). The number of pages is
shown before generation.

### PDF generation

1. Worker pool: decoding (JPEG/HEIC/PNG already handled, EXIF orientation
   applied), reduction to **300 dpi** of the printed size, JPEG q90 re-encoding.
   Avoids 200 MB PDFs and keeps printing sharp.
2. Sequential assembly of the pages; thin grey cutting rule around each
   photo (it stays within the cutting space, the photo is never trimmed).
3. Atomic write (`.tmp` then rename) into the output folder, then
   `shell.openPath` on the Electron side when `done` is received.

### Interface

The panel unfolds on a single screen, with no blocking steps:

1. **Root folder**: prefilled with the destination folder from the preferences.
2. **Monthly calendar**: days with photos marked with a dot; a click on a
   day highlights the whole week; previous/next month arrows; "This week".
3. **Photos of the week**: existing `VirtualGrid` + `Thumb`, checkboxes,
   counter "9 of 38".
4. **Settings** (collapsed): long-side size (6.5 cm), page format, space
   between photos (5 mm minimum).
5. **Page preview** on the right with the photos in their true proportion, updated
   live (number of pages shown), then a **Create the PDF** button. A line
   reminds the output folder ("Saved in: Downloads"). Reminder:
   print at 100% ("actual size"), without "fit to page".

**Preferences (⚙️)**: new field "Weekly planner PDF folder" with a
folder picker (reuses `dialog:pickFolder`). Empty = Downloads,
resolved on the main side by `app.getPath('downloads')` (new IPC `app:downloadsDir`,
valid on macOS, Windows and Linux).

fr/en/da i18n; preferences persisted in `prefs.ts` (`scrapOutputDir`,
`scrapLongSide`, `scrapPage`, `scrapGutter`).

## Aesthetic direction

"**Stationery notebook**": builds on the existing themes (CSS variables) to
stay consistent, with a warm tone of paper and adhesive tape.

- The selected week appears as a translucent **washi tape strip**
  laid over the calendar row (rotation of ~1°, jagged edge in CSS).
- Ticked photos get a light **white polaroid-style frame** and a sequence
  number (stamp); unticking removes it smoothly.
- **One highlight**: on "Create the PDF", the ticked thumbnails fly to
  the page preview (reuses `Flights.tsx`), the sheet fills up, then slides
  off the screen when the PDF opens. The rest stays sober: `transform`/`opacity`
  transitions only, as in the rest of the application.
- Typography: the application's system stack; sequence numbers and week label
  in monospace (`ui-monospace`) to evoke a date stamp.
- No live blur or costly shadows (existing performance constraints).

## Error handling and edge cases

- Week with no photo: clear message and disabled button.
- Unreadable photo or undecodable HEIC: ignored with a warning in the summary,
  the other pages are generated.
- More photos than fit on a page: automatic pagination.
- Invalid root folder or one without `YYYY/YYYY-MM`: message guiding to the right folder.
- Output folder missing or not writable: fallback to Downloads with a
  warning; if writing still fails, explicit error and no partial file.
- Week spanning two months or two years: the scan covers both folders.
- Cancellation in progress: no partial file left behind.

## Tests

- Go: `week_test.go` (start of week, month/year overlap, 53rd week),
  `scan_test.go` (temporary tree, non-image files ignored, "Almost Duplicates"
  included, fallback to EXIF/mtime), `layout_test.go` (exact long side to 0.1 mm,
  portrait/landscape/square, mixed rows, **space ≥ 5 mm between all pairs of
  photos**, even with a requested `gutter` < 5 mm, no overflow of the margins,
  10 portraits / 12 landscapes per A4 landscape page, pagination), `render_test.go`
  (valid PDF: `%PDF` header, page count, reasonable size, cancellation,
  ` (2)` suffix on name collision).
- End-to-end sidecar test on a **copy** of a sample of
  `~/Pictures` (never the original).
- UI: `npm run typecheck`, `electron-vite build`, manual walkthrough (calendar,
  selection, preview, PDF opening) and printing of a sheet at 100% scale
  to check the dimensions (ruler).

## Breakdown into steps

1. Go core: `week`, `scan`, `layout` + tests.
2. PDF rendering + sidecar operations + e2e test.
3. Calendar and selection in the panel (without preview).
4. Live preview, settings, preferences, i18n.
5. "Stationery notebook" animations, README, packaging unchanged (no new
   native dependency: fpdf is pure Go, compatible with the CI workflow's CGO/Windows).

## Differences from the implementation

The final sidecar and UI contract differs slightly from the above:

- `weekDays` takes `{root, from, to}` (the visible calendar grid, including weeks
  of neighbouring months) and returns `{days, rootOk}`; a false `rootOk` shows
  "no year folder here".
- `weekPhotos` returns **a single** `photos` event `{photos:[{path, date}]}`
  sorted by date, and not a `photo` stream: the scan reads only file names
  (or EXIF/mtime as a last resort), it is instantaneous.
- `scrapbookLayout` takes `{files, page, longSide, gutter}`: the ratio of each
  photo is read on the Go side from the header (`image.DecodeConfig`, preview of 11 HEICs
  down from 18 s to 1 s) and not sent by the UI. It returns `layout` with the
  positions in mm and `failed` for unreadable photos, which are excluded.
- `scrapbook` additionally takes `fallbackDir` (Downloads); `done` contains
  `fallback` (boolean): the folder actually used is `dirname(path)`.
- Labels (week, file name `Semaine 2026-S47 (16–22 novembre).pdf`)
  are built in the UI with `Intl` (fr/en/da i18n), not in Go.
- The current week is preselected when the panel opens.
- Range limited to 366 days; empty or entirely unreadable selection = `error`
  event.
- Aesthetics: washi tape on the week, cream sheets in the preview with
  a spring on appearance, thumbnails flying and confetti on creation. Ticked
  photos get an accent ring and an animated tick (no polaroid frame
  or sequence number, judged unnecessary for an unordered selection).
- `Cmd/Ctrl+P` shortcut (*Weekly planner* menu) in addition to the 📅 button.
- The `scrapLongSide` and `scrapGutter` preferences are adjustable in the panel
  (50–80 mm, 5–10 mm); the PDF folder is adjustable in ⚙️.
