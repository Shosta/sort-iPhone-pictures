# Photosort — Context Map

Read this first, then open only the files it points to. Verified against `main`
@ `19007a8` (Go tests all green: `go test -count=1 ./...`). If you change
structure, protocol ops, prefs or CI, update the matching section here.

Docs, UI strings and commit messages are mostly **French**; code identifiers and
comments are English (Electron `src/main` comments are French).

## 1. What it is

Photosort copies photos from "source" folders into a dated library
(never moves/deletes originals):

```text
<dest>/2026/2026-10/[<Place>/]2026-10-20 - iPhone - 1.heic     # EXIF date + camera + counter per day/model
<dest>/Photos_Undated/Photo undated - N.ext                     # no usable date
<dest>/Screenshots/  <dest>/<Y>/<Y-M>/Blurry/  <Y>/<Y-M>/<...> - Almost Duplicates/   # optional groupings
<photo>.xmp                                                     # optional AI tags (dc:subject), next to the copy
```

Three front ends share one Go engine:

| Front end | Entry | Talks to engine via |
|---|---|---|
| CLI | `cmd/photosort` | direct import of `sorter`, `internal/remote`, `internal/tagger` |
| Fyne desktop GUI (simpler: sort only) | `cmd/photosort-gui` → `internal/desktop` | direct import of `sorter` |
| **Electron app (full feature set, released by `package.yml`)** | `photosort-electron/` | spawns Go **sidecar** `cmd/photosort-sidecar`, JSON-lines over stdin/stdout |
| Marketing site | `website/` (Astro, fr/en/da) | independent of the app |

Go module: `sort-iphone-pictures` (go 1.27.4, CGO required: ONNX Runtime, Fyne).

## 2. Repo layout

```text
sorter/                    CORE ENGINE: Plan → Copy, dedupe, near-dupes, blur, date fallback, tag orchestration
cmd/photosort/             CLI (flags → sorter.Engine; --remote*, --tag-*)
cmd/photosort-gui/         Fyne bootstrap (3 lines)
cmd/photosort-sidecar/     JSON-lines server used by Electron (ALL ops; protocol documented in main.go header)
internal/
  desktop/                 Fyne window, tree, thumbnails (only used by photosort-gui)
  thumbs/                  GUI-free decode + thumbnails (JPEG/PNG/TIFF/HEIC/AVIF via WASM, RAW embedded preview), memory budget
  jpeg8/                   fork of Go image/jpeg decoding at 1/8 scale (DC only) → fast hash/AI input
  geo/                     offline GeoNames: city (cities.tsv.gz) + landmarks (places.tsv.gz); gen/ regenerates data
  tagger/                  ONNX scene (Places365) + face (YuNet) + solar moment + EXIF "meta" tags; XMP writer
  devimport/               import from a mounted DCIM volume: detect (Linux gvfs/media), scan, 200 MiB batches, safe transit dir, per-device journal
  remote/                  NAS upload: stage tree of links → rsync (daemon or ssh) transport; error classification
  scrapbook/               weekly printable PDF sheets: scan week, measure ratios, layout (mm), frames/backgrounds, fpdf render
  mapbase/                 offline Natural Earth base map (world.bin.gz), Mercator fit, optional online tile providers + cache
  yearmap/                 "photos of the year" map: group geotagged photos into places, layout pins/cards, PDF
  travelmap/               trip detection (>100 km from home) + route map PDF; reuses yearmap
  appicon/                 embedded PNG icon
photosort-electron/        Electron + React + TS app (see §4)
website/                   Astro static site + node tests
packaging/                 Fyne-app packaging (macOS .app, Linux AppImage via Docker) — NOT the Electron installers
docs/                      nas-qnap.md + superpowers/{specs,plans}/ design docs (French)
.github/workflows/         ci.yml, package.yml (release), pages.yml (website); dependabot.yml
.vscode/                   launch/tasks (Fyne app build tasks, preview/copy debug profiles)
```

## 3. Go engine

### `sorter` (core)

- `Engine{...}.Plan(ctx, sources, dest) (Plan, error)` — no writes. Stages (reported via `Engine.Progress(stage,done,total)`):
  `exif` (parallel read, emits `Draft` entries) → `hash` (SHA-256 bytes/pixels + dHash, only if `SkipDuplicates`/`GroupSimilar`) → `blur` → `similar`.
- `Copy` / `CopyWithProgress(ctx, plan, onCopied)` — copies (4 workers), never overwrites, removes partial files on error/cancel.
- Types: `Plan{Destination,Entries,Skipped,Duplicates}`, `Entry{Source,Target,Metadata}`, `Issue`, `Duplicate`, `Report`.
- Engine options (all booleans default off): `ByLocation`, `Landmarks`, `SkipDuplicates`, `GroupSimilar`, `DateFallback`, `ScreenshotFolder`, `GroupBlurry` (+ `BlurThreshold`).
  Test seams: `ReadMetadata`, `Locate`, `LocateLandmark`, `Hash`, `Sharpness`, `Now`.
- Hooks for streaming UIs: `Progress`, `Draft`, `OnDuplicate`, `Preview` (called concurrently).
- Files: `sorter.go` (plan/copy/naming/path safety), `metadata.go` (EXIF via `evanoberholster/imagemeta` → `Metadata`),
  `fingerprint.go` (single read per file; `parallel()` worker pool), `dedupe.go`, `similar.go`
  (dHash, `similarWindow`=10 min, `similarDistance`=8, suffix `" - Almost Duplicates"`), `blur.go` (Laplacian variance, threshold 15),
  `datefallback.go` (filename date, then mtime; ignores < 1990), `tags.go` (`PhotosToTag`, `TagPhotos` → writes `.xmp`, never overwrites).
- Supported extensions: jpg jpeg png tif tiff heic heif avif dng cr2 cr3 nef arw rw2 crw (`sorter.IsPhoto`). Videos/Live Photos/AAE unsupported.
- Invariants: originals never touched; no overwrite; source/destination must be disjoint (also via symlinks); symlinks inside sources ignored;
  names sanitized (camera ≤ 40 chars, place ≤ 60).

### Sidecar (`cmd/photosort-sidecar`)

One goroutine per request; `server.run` switch dispatches the ops. **Authoritative protocol doc = header comment of `main.go`**.
Ops: `plan`, `copy`, `remoteTest`, `thumbnail`, `tag`, `weekDays`, `weekPhotos`, `scrapbookLayout`, `scrapbook`,
`yearMapYears`, `yearMapScan`, `yearMapLayout`, `yearMapPdf`, `travelTrips`, `travelScan`, `travelLayout`, `travelPdf`, `importDetect`, `importScan`, `importRun`, `importControl`, `importForget`, `cancel`.

| File | Role |
|---|---|
| `main.go` | `request` struct (every JSON field), server, dispatch, `plan` (draft batching 80 ms, `planChunk` ≤ 2000 entries/~256 KB, then `plan` with counts), `copy`, `thumbnail`, `tag` |
| `cache.go` | thumbnail LRU (96 MiB budget); plan previews prefill it |
| `remote.go` | `prepareRemotePlan` (scratch stage dir), `remoteTest`, `remoteCopy` (stage links → rsync; events `remoteProgress`, `done`) |
| `import.go` | DCIM import ops: copy→sort producer/consumer pipeline (next batch copied while previous sorted), pause gate, auto-pause/resume on disconnect, journal after verified delivery. Request fields `deviceId`, `devicePath` (DCIM folder; empty id → `folder-<hash>`), `mode` all\|new\|period (+`from`/`to` YYYY-MM-DD), `action` pause\|resume |
| `scrapbook.go` | week days/photos, sheet layout, PDF |
| `yearmap.go`, `travel.go` | place/trip scan, layout, PDF, tile-provider backdrop with fallback |
| `*_test.go` | protocol/perf tests (`TestPlanStreams`, `TestPlanChunks`, `TestLargePlan` are CI perf guards) |

Event types: `status`, `draft`, `planChunk`, `plan`, `progress`, `done`, `thumbnail`, `tagged`, `weekDays`, `photos`, `layout`,
`yearMapYears`, `places`, `trips`, `trip`, `importDevices`, `importScan`, `importProgress`, `importPaused`/`importResumed`, `importDone` (import errors carry `code` noDevice/lowDisk/copy + `detail`), `remoteTest`, `remoteProgress`, `error` (`cancelled`, remote `code/detail/resumable`).
Map PDFs report backdrop problems inside `done` (`backdropFailed`, `backdropWarning`); remote results carry `warnings`.
Terminal events (renderer drops its listener): see `TERMINAL` in `renderer/sidecar.ts`.

### Other `internal/*` quick facts

- `sorter.Engine.TakenFrom/TakenTo` (inclusive, zero = unbounded) filter by capture date; undated photos are excluded when a range is set; counted in `Plan.Filtered`.
- `internal/devimport`: photo ext heic/jpg/jpeg/png/dng (videos only counted); journal `~/.config/photosort/imports/<id>.json` keyed `rel|size|mtime`; transit `os.TempDir()/photosort-<user>` (0700, owner-checked, no symlinks) with `UserCacheDir/photosort/import` fallback. Read-only on the device. Local or NAS destination: with `remote` set, each sorted batch goes through `remoteSend` (quiet mode of `remoteCopy`) and is journaled only after rsync succeeds; network cuts emit `importPaused{reason:network}` and retry.

- `tagger`: `New(dir, Options)`, `Tags(Photo)`, `Close`, `FindAssets` (dir → `$PHOTOSORT_AI_DIR` → `ai/` next to exe), `XMP/WriteXMP`.
  Models: `places.onnx`, `categories.txt`, `yunet.onnx` + ONNX Runtime lib. Tag constants (mostly French labels) live in `tagger.go` (People/Portrait/Groupe), `scene.go`, `sun.go`, `meta.go`.
- `remote`: `Transport` interface (`Upload`, `Test`), impl `Rsync` (`rsync.go`); `Stage/Unstage/Send` (`stage.go`); `Config` (daemon|ssh);
  `Kind`/`Classify`/`Scrub` (`errors.go`) — secrets are scrubbed from errors; password never persisted/logged.
- `thumbs`: `Image/JPEG/Analysis/Decode/Fit/Orient`, `Embedded` (EXIF thumbnail/RAW preview), `budget.go` caps parallel HEIC/AVIF WASM decodes (~36 B/pixel).
- `scrapbook`: `Render(Job)`, `Layout*`, `Scan`, `WeekOf`, papers `a4-landscape|a4-portrait|letter-*`, frames (`none|polaroid|stamp|tape`), styles (`grid|collage`), backgrounds, bleed ≤ 10 mm. Units = **mm**.
- `mapbase`: `Fit`, `Base.Land/Lakes`, `Providers` = **`osm`, `esritopo`** (+ `offline`), `Backdrop`, tile cache `UserCacheDir/Photosort/tiles`. Data: `world.bin.gz` (see `NOTICE.md`; regenerate with `mapbase/gen`).
- `yearmap` / `travelmap`: `Collect`, `Group`, `Compose*`, `Render`; `travelmap.Detect/Build/Steps`.

## 4. Electron app (`photosort-electron/`)

Process model (contextIsolation, sandbox, no nodeIntegration, no webview, all Chromium permissions denied):

```text
renderer (React, src/renderer)  ──window.photosort (preload)──►  main (src/main/index.ts)  ──stdin/stdout JSON lines──►  Go sidecar
```

| File | Role |
|---|---|
| `src/main/index.ts` | window, menu (Prefs ⌘, / Tags ⌘T / Scrapbook ⌘P), sidecar spawn (`resources/bin/photosort-sidecar`, env `PHOTOSORT_AI_DIR`), IPC handlers, safeStorage secrets (`remote-secrets.json`), `PathApprovals` for `shell:reveal`, AI asset provisioning |
| `src/main/guards.ts` | **IPC security**: `SIDECAR_OPS` allow-list, `FIELDS` allow-list+types+size caps, `validateSidecarRequest`, `isAllowedExternalUrl` (https only), `isTrustedPage`, `checkOpenablePath`, secret account/value checks. Pure module, tested with `node --test` |
| `src/main/assets.ts` | downloads Places365/YuNet models + ONNX Runtime 1.31.1 into `userData/ai` with pinned SHA-256 (duplicated checksums in `scripts/fetch-ai-assets.mjs`) |
| `src/preload/index.ts` | `contextBridge` API: `send`, `onEvent`, `pickFolders/pickFile`, `secrets.*`, `pathsFromDrop`, menu hooks, `getAssets/retryAssets/onAssets`, `downloadsDir`, `openPath` |
| `src/renderer/App.tsx` | main screen/state machine (`Phase`: idle→analyzing→ready→copying→done), drag-drop sources, draft/plan streaming, copy animation, tag job, remote flow |
| `src/renderer/sidecar.ts` | `request(payload, listener)` with id routing + `planChunk` assembly; thumbnail loader (LIFO queue, LRU 1500, 256 px) |
| `src/renderer/planStream.ts` | reassembles `planChunk`s and verifies counts |
| `src/renderer/prefs.ts` | `Prefs` in `localStorage['prefs']` (+ legacy keys); defaults: dedupe/similar/fallback/screenshots ON, blurry OFF, tags ON |
| `src/renderer/i18n.tsx` | **all UI strings** `fr` / `en-GB` / `da` (`Key = keyof typeof fr`; add a key to all three) |
| `src/renderer/themes.ts` | 8 themes → CSS vars |
| `src/renderer/types.ts` | shared TS types (`Plan`, `Entry`, `TagJob`, `Scrap*`, `Year*`, `Travel*`…) + `window.photosort` typing |
| `src/renderer/components/` | `Preferences`, `TagsPanel`, `ScrapbookPanel` (+`Calendar`), `YearMapPanel`, `TravelMapPanel`, `RemotePanel`, `DropZone`, `ImportCard` (DCIM *source picker*: polls `importDetect` every 3 s, replaces DropZone while a device is present or the manual link is open; reports `{devicePath, deviceId, mode, from, to}` to `App`, no action button; portal `⋯` menu), `IssueList`, `Thumb`, `VirtualGrid`, `Flights`, `Select`, `Flag` |
| `scripts/` | `build-sidecar.mjs` (go build, CGO, GOOS/GOARCH), `fetch-ai-assets.mjs`, `checksums.mjs`, `audit-gate.mjs`, `after-pack.cjs` (Linux launcher), `rename-dev-electron.mjs` |
| `electron-builder.yml` | dmg / portable exe / AppImage+deb; macOS signing+notarization only when secrets present |

## 5. Website (`website/`)

Astro 7, static, **no JS/remote resources required**. Pages `src/pages/[lang]/{index,tri-distant}.astro`, root `index.astro` redirects by browser language.
Copy in `src/i18n/{fr,en,da}.ts` (typed by `Copy` in `types.ts`; change all three) and `nas.ts` (NAS guide, 3 langs).
Components `src/components/*.astro`, styles `src/styles/global.css`. Env: `PUBLIC_REPOSITORY_URL`, `ASTRO_SITE`, `ASTRO_BASE`
(internal links must use `import.meta.env.BASE_URL`). Tests: `tests/site.test.mjs` run on the built `dist/` (structure parity across languages, hreflang, no dead links, honest claims).

## 6. Commands

| Task | Command (cwd) |
|---|---|
| Go build / vet / test | `go build ./... && go vet ./... && go test -count=1 ./...` (repo root; needs C toolchain for Fyne/ONNX) |
| Go perf guards (as CI) | `go test -count=1 -run 'TestBudget\|TestRunWasm\|TestDecodeHEIC' ./internal/thumbs` ; `go test -count=1 -run 'TestPlanStreams\|TestPlanChunks\|TestLargePlan' ./cmd/photosort-sidecar` |
| Run CLI | `go run ./cmd/photosort --source <dir> --destination <dir> [--apply] [--json]` |
| Run Fyne GUI | `go run ./cmd/photosort-gui` |
| Electron dev | `npm ci && npm run dev` (`photosort-electron/`; builds sidecar first) |
| Electron checks | `npm run typecheck && npm test && npm run audit:gate` |
| Electron package | `npm run dist[:mac\|:win\|:linux]` (+ `checksums`) |
| Website | `npm ci && npm test` (`website/`; builds then tests; `ASTRO_BASE=/repo npm test` simulates Pages) |
| Fyne packaging | `bash packaging/build.sh macos\|linux` |
| Regenerate geo data | `go run ./internal/geo/gen cities1000.txt internal/geo/cities.tsv.gz` (and `poi allCountries.txt internal/geo/places.tsv.gz`) |

CI (`.github/workflows/ci.yml`, all actions pinned by SHA): jobs `go` (build, vet, test, govulncheck), `perf`, `electron`, `website`, `workflows` (actionlint).
`package.yml` builds installers on push to `main`/tag `v*` after CI; `pages.yml` deploys `website/**`.

## 7. Where to change what (recipes)

| Goal | Touch |
|---|---|
| New sort rule / folder grouping | `sorter/sorter.go` (+ `Engine` field) → CLI flag in `cmd/photosort/main.go` → sidecar `request` field + `s.plan` in `cmd/photosort-sidecar/main.go` → `FIELDS` in `src/main/guards.ts` → `Prefs` + toggle in `prefs.ts`/`Preferences.tsx` + `App.tsx` plan request → 3 languages in `i18n.tsx` → README(s) → tests in `sorter/sorter_test.go`, `cmd/photosort/main_test.go`, `cmd/photosort-sidecar/main_test.go` |
| New sidecar op | handler in `cmd/photosort-sidecar/*.go` + `case` in `server.run` + header doc; add op (and any new field) to `SIDECAR_OPS`/`FIELDS` in `guards.ts` + `guards.test.ts`; if it ends with a one-shot event add its type to `TERMINAL` in `renderer/sidecar.ts` |
| New tag type | `internal/tagger/*` (+ `Options`), `sorter/tags.go`, sidecar `tag` op, `tagX` field in guards/prefs/TagsPanel, i18n, README table |
| New PDF page type | model on `internal/yearmap` / `travelmap` (layout in mm + `Render`) and `cmd/photosort-sidecar/{yearmap,travel}.go`, panel on `YearMapPanel.tsx` |
| New UI string / language | `src/renderer/i18n.tsx` (all 3 langs) ; website: `src/i18n/*.ts` (all langs) |
| NAS / remote | `internal/remote/*`, `cmd/photosort-sidecar/remote.go`, `RemotePanel.tsx`, `renderer/remote.ts` (error → text), `docs/nas-qnap.md`, `website/src/i18n/nas.ts` |
| Security hardening of IPC/paths | `src/main/guards.ts` + `guards.test.ts`, then `index.ts` handlers |
| Installer / release | `electron-builder.yml`, `scripts/`, `.github/workflows/package.yml`, Electron README "Signature, notarisation…" |

## 8. Conventions & gotchas

- **Never** modify/move/delete originals; never overwrite destination files. Keep it that way in any new feature.
- Renderer input is untrusted: every new request field must be allow-listed in `guards.ts`. `aiDir` is intentionally NOT allowed from the renderer.
- Sizes in scrapbook/yearmap/travelmap are **millimetres**; the renderer draws the sidecar layout as-is (SVG paths in mm).
- Plans are streamed in chunks; never send a monolithic plan line (keeps each line well under 1 MB). The full plan (with metadata) stays in the sidecar keyed by `planId` for `copy`.
- Thumbnails: during plan/copy only 2 concurrent decodes (`busyThumbs`); previews seen while planning are cached.
- Offline by default: only map tiles (opt-in providers) and the one-time AI model download touch the network; no coordinate leaves the machine otherwise.
- Stale docs to be aware of: the sidecar header still mentions providers "positron, topo, satellite" (code has `osm`, `esritopo`);
  root `README.md` still describes only the Fyne GUI + CLI and an "initial commit" GitHub section; `go.mod` module name is local.
- The Fyne GUI/`packaging/` track is separate from the Electron release track (`package.yml` runs electron-builder and ships Electron installers).
- Docs under `docs/superpowers/` are design history; the NAS "docker server" plan is **deferred** (no code).
- Repo git branches of interest: `main`, `feat/website`, `feat/nas-qnap-remote` (NAS work), several session branches.
- Test files: Go `*_test.go` beside code (+ `internal/*/testdata`); Electron `src/main/*.test.ts`, `src/renderer/*.test.ts`, `scripts/*.test.{cjs,mjs}` (run with `node --test`, no framework); website `tests/site.test.mjs`.

## 9. Backlog (ideas not started)

Agreed order of interest; none is implemented.

1. **Configurable folder tree** — let the user choose the destination layout with a template (`{year}`, `{month}`, `{day}`, `{city}`, `{country}`, `{device}`) and presets (`Year/Month`, `Year/Month-Day`, `Year/Place`, `Year/Event`, flat). Live preview on a sample photo; same rule for local sort and NAS upload; fallback folders for undated/unlocated photos. Offline geocoding already supplies place variables. Touch `sorter/sorter.go` (path builder), sidecar request + `guards.ts` `FIELDS`, `prefs.ts`, `i18n.tsx`, `internal/remote` staging.
2. Incremental sort (skip already-sorted files), Live Photos pairing, dry-run report / rollback, events, timezone handling, filters, renaming, conversion, profiles, licence/Pro.
3. Fallback: SMB track for the NAS (`docs/nas-qnap.md`).

Rejected: watched folder, native USB/libimobiledevice import (superseded by the mounted-DCIM import below).

### Import from a mounted DCIM volume (iPhone, SD card, camera) — IMPLEMENTED (design record)

Implemented in `internal/devimport`, `cmd/photosort-sidecar/import.go`, `components/ImportCard.tsx`. NAS destination supported via `remoteSend`.

**DCIM = a source of the normal Analyze/Sort flow** (user feedback 4): `ImportCard` only picks the volume and mode. The usual **Analyser** sends `plan` with `devicePath/deviceId/mode/from/to` (sidecar `devicePlan`: filter by mode/period + journal for "new" + photo extensions, EXIF read read-only from the device via `Engine.Include`; plan sources are `<canonical DCIM>/<rel>` so thumbnails and pending-list matching work). The usual **Trier / Simuler** sends `copy` with that `planId`: `deviceCopy` → `runImport` (200 MiB transit batches, size check, pause/resume on disconnect or network cut, journal only after verified delivery — never in dry run, transit cleanup, local or NAS via `remoteSend`). Events: `progress` (entry.source = device path), `importProgress/importPaused/importResumed`, final `done` (NAS summary fields included). Dedupe/similar/blurry grouping is therefore per batch. The legacy `importRun` op remains for tests.

- **Source**: Linux detects an already-mounted `DCIM` (GVFS `/run/user/<uid>/gvfs/afc:host=<UDID>`, `/media`, `/run/media`); also watches for plug-in after launch. No mounting/pairing in the app; help message if nothing found ("unlock the iPhone, tap Trust"). macOS/Windows: pick the source manually; batch-copy code stays platform-neutral.
- **Read-only**: never modify/delete anything on the device.
- **Modes**: All / New (default) / Period (two date pickers, start 00:00 → end 23:59:59, year included, default current year). Period = pre-filter on file mtime ±1 day during copy, exact filter on capture date (existing date fallback) at sort.
- **Files**: HEIC, JPG, JPEG, PNG, DNG only; videos, `.AAE` etc. ignored (report "N videos ignored"); no sidecar pairing.
- **Transit**: `/tmp/photosort-<user>/` mode 0700 (owner-checked, no symlink following), fallback `~/.cache/photosort/import/` if free space is short; free-space check before each batch. App empties it itself silently at start and before each import; the user never cleans anything.
- **Batches**: ~200 MB each (a larger file = its own batch), oldest first, 2-batch pipeline (copy N+1 while sorting N). Per batch: copy to `.part` + size check → sort with the usual options (local or NAS destination) → journal entry only after verified delivery → delete batch.
- **Disconnect**: auto-pause with message, auto-resume on replug (same device id); no partial sort without consent.
- **Journal**: `~/.config/photosort/imports/<UDID>.json`, atomic write; key = device id + relative path + size + mtime; "New" = absent from journal (instant, no hashing); SHA-256 dedupe at sort remains the safety net; deleted-after-sort photos are not re-imported unless "Re-import everything"; "Forget this device" button.
- **Screen (confirmed brief)**: keeps the existing dark/aurora world; reuse `Calendar` and the Destination panel patterns. A single "Device detected" card replaces the drop zone while a volume is plugged in (link back to manual folder drop). States: (1) ready — device name, 3-way selector All / New (default) / Period (two date pickers shown only for Period), live "N photos to import, from X to Y" count with a small counting animation that never blocks the button, primary "Import and sort", secondary menu with "Re-import everything" and "Forget this device"; (2) running — same card becomes progress: phase Copy/Sort, batch n/N, photos, elapsed, bar, Pause/Cancel; (3) done — summary (imported, duplicates skipped, videos ignored) then back to ready. Also design: no device + help text, locked phone, disconnect/auto-resume, zero new photos, low disk space, empty period, copy error. FR/EN/DA, keyboard/a11y, narrow window collapses to one column. Out of scope v1: thumbnails of device photos, per-photo selection.
- Touched: new `internal/devimport` (detect, batch copy, journal), sidecar ops + `guards.ts` `FIELDS`, `sorter` date-range filter, new Import card + i18n (3 langs).
