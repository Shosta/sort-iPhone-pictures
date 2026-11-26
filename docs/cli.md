# CLI, Fyne app and packaging

The full feature set (iPhone import, NAS upload, duplicates, scrapbook and maps) lives in the Electron app. This page covers the command-line tool, the simpler Fyne desktop app and the Fyne packaging.

## Command line

Requires Go 1.27 or later (see `go.mod`). From the project root, preview a sort without writing anything:

```sh
go run ./cmd/photosort --source "/path/photos" --destination "/path/library"
```

Copy the photos once the preview looks right:

```sh
go run ./cmd/photosort --source "/path/photos" --destination "/path/library" --apply
```

Several sources and JSON output:

```sh
go run ./cmd/photosort --source "/path/photos" --source "/path/other-photos" --destination "/path/library" --json
```

`--json` can also be combined with `--apply`. `--help` lists the options. The exit code is 1 on a blocking error, 0 otherwise. Skipped files are reported in the summary without making the whole operation fail. Go's own arguments must come before the application's.

Build an executable:

```sh
go build -o bin/photosort ./cmd/photosort
./bin/photosort --help
```

The sorting rules are described in [sorting-rules.md](sorting-rules.md).

## Fyne desktop app (sort only)

```sh
go run ./cmd/photosort-gui
# or
go build -o bin/photosort-gui ./cmd/photosort-gui && ./bin/photosort-gui
```

- Drop one or more folders into the window, or use `Add`.
- The left tree contains only sortable photos and their parent folders. `Clear` removes the sources without deleting files.
- Edit the path above the right panel, or use `Choose`. The path may point to a folder that does not exist yet. The last successfully analysed destination is remembered locally.
- `Sort` starts the copies. The right panel only shows the successful copies of this session, not the existing content of the destination.
- `Cancel` stops the analysis or the copy; completed copies remain. Originals are always kept.
- Ignored files and thumbnail failures are listed under reports.

Thumbnails load in the background. JPEG, PNG, TIFF, HEIC/HEIF and AVIF have decoders; RAW formats use their embedded JPEG preview when one exists. A damaged image, one without a decodable preview, or one over 100 megapixels keeps a file icon and raises a report, without blocking its copy if its EXIF is valid.

Fyne adds native graphics dependencies: Apple Command Line Tools on macOS; a C compiler, pkg-config and OpenGL/X11 development libraries on Linux (see the [Fyne prerequisites](https://docs.fyne.io/started/)). HEIC/AVIF decoding uses a local library if available, otherwise an embedded WASM decoder, with no upload and no mandatory external tool. Dropping external folders still needs manual verification on Linux.

## Packaging the Fyne app

The VS Code tasks `Photosort: build macOS (.app)` and `Photosort: build Linux (AppImage x86_64)` produce the apps in `dist/`. Packaging is isolated in [packaging](../packaging/README.md).

```sh
bash packaging/build.sh macos
bash packaging/build.sh linux
```

The macOS build runs on a Mac. The Linux build uses Docker with a running engine, including from macOS ARM64. This is separate from the Electron installers built by the release workflow.

## Tests and debugging

```sh
go test ./...
go test -race ./...
go vet ./...
```

In VS Code, install the official Go extension if needed. The `Photosort: preview` and `Photosort: copy` launch profiles ask for paths at launch. Set a breakpoint in `Engine.Plan`, `ReadMetadata` or `Copy`, then press F5. The `copy` profile really writes files.

## Publishing to GitHub Enterprise

Do not add personal photos to the repository; use source and destination folders outside the project. To publish, create an empty repository in GitHub Enterprise (no README, licence or server-side gitignore), then:

```sh
git remote add origin <ENTERPRISE_REPOSITORY_URL>
git push -u origin main
```

The Go module name is currently local (`sort-iphone-pictures`) and can be changed to match the repository address once it is known.
