# Contributing to Photosort

Thanks for your interest! Photosort is a small, careful project and contributions of all sizes are welcome — bug reports, docs, translations, code. By participating you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Guiding principles

- **Never write to the originals.** Photosort only reads source files. No feature may modify, move, rename or delete a user's original, and no feature may overwrite an existing destination file.
- **Local first.** Nothing should leave the machine without an explicit opt-in.
- **Under-promise, over-deliver.** Documentation and UI copy must state only what is verified.
- **Modular architecture.** The Go engine (`sorter`, `internal/*`) is independent of any front end; the CLI, the Fyne app and the Electron app (through the Go sidecar) all reuse it. Keep new logic in the engine and expose it through the sidecar protocol.

Start with the [architecture map](CONTEXT_MAP.md): it explains the layout, the sidecar protocol and a "where to change what" table.

## Getting started

```sh
git clone <repository-url>
cd <repository>
```

Prerequisites: Go (see `go.mod`), a C compiler (CGO is required for ONNX Runtime and Fyne), and Node.js for the Electron app and the website.

### Go engine, CLI and sidecar

```sh
go build ./...
go vet ./...
go test ./...
```

### Electron app

```sh
cd photosort-electron
npm ci
npm run typecheck
npm test
npm run dev        # run the app in development (builds the sidecar first)
```

### Website

```sh
cd website
npm ci
npm test           # builds the site, then checks it
```

CI also runs `govulncheck`, `actionlint` on the workflows and performance guards on plan streaming; see `.github/workflows/ci.yml`.

## Making a change

1. Create a branch from `main`.
2. Keep the change focused, and add or update tests next to the code you touch (`*_test.go` for Go, `*.test.ts` for the Electron app, `website/tests/` for the site).
3. New UI strings go into **all three** languages (French, English, Danish) in `photosort-electron/src/renderer/i18n.tsx`; website copy likewise.
4. New request fields from the UI must be allow-listed in `photosort-electron/src/main/guards.ts`.
5. Run the checks above, then open a pull request describing what changed and how you verified it.

### Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/) prefixes: `feat:` for features and `fix:` for bug fixes (others such as `docs:`, `test:`, `chore:` are fine). The release script builds the changelog from `feat:` and `fix:` commits.

## Reporting bugs and ideas

Open an issue with the OS, the Photosort version (Preferences or About), what you expected and what happened. **Never attach personal photos**; describe the file type or share a synthetic sample instead.

For security-sensitive reports, please contact the maintainer privately rather than opening a public issue.
