# Plan: remote sorting to the QNAP NAS via rsync (folder picker, deduplication)

Date: 2026-11-23 · Status: **to be validated** (no code written for this step)
Replaces, for the transport, `2026-11-17-nas-local-connection-plan.md` (SMB), which remains as a fallback.

## Context and agreed decisions

| # | Decision |
| --- | --- |
| D1 | **rsync** is the transport (resume with `--partial`, optional `--checksum` verification, progress). No reimplementation. |
| D2 | **Sources stay on the PC.** Analysis, sorting, tags, duplicates, blur, screenshots: local, as today. The NAS is only a **destination** offered at copy time. |
| D3 | The local sort is written to a temporary tree of symbolic links, then `rsync -aL --partial --ignore-existing` sends it. No intermediate copy. |
| D4 | **Never delete**: neither `--delete` nor `--remove-source-files`. Originals are never touched. |
| D5 | QNAP: **a single rsync user**, which cannot be restricted to one folder. Security therefore comes from the program, not from NAS permissions. |
| D6 | Modes: rsync daemon (module + password, never in clear text in logs or arguments) and rsync over SSH (SSH is refused by the user on the NAS: the daemon is the main mode). |
| D7 | macOS: prefer the Homebrew rsync (`/opt/homebrew/bin`, `/usr/local/bin`); `/usr/bin/rsync` (openrsync) is limited, with fallback and warning. |
| D8 | User guide on the **static site** (not the wiki). |
| D9 | Dropped: USB import from the iPhone, watched folder. |

Already delivered in `main`: `internal/remote` (`Transport` interface, rsync, link staging), CLI, sidecar, `RemotePanel` (connection test, simulation, progress, FR/EN/DA errors), `tri-distant` guide.

## Goal of this step

1. Choose the destination on the NAS **without typing a path** (folder discovery).
2. Do not resend what the NAS already has (deduplication against the remote library).
3. Also send the tags created locally.
4. Update the guide.

## Phase 1 — Remote folder picker

- [x] `internal/remote`: `ListDir(ctx, target, path)` based on `rsync --list-only` (non-recursive, `-d`); parsing of the output (name, type, size, date); first level = list of the daemon's modules.
- [x] `remoteList` sidecar op; errors reuse the existing messages (password, unknown module, host unreachable).
- [x] UI: level-by-level browser in `RemotePanel` (opened on demand, current path, "Choisir ce dossier" (Choose this folder) button), loading indicator, cancellation.
- [x] **Favourite folder**: last folder remembered in the preferences.
- [x] **Folder creation**: a starting folder several levels deep requires `--mkpath` (rsync ≥ 3.2.3); otherwise, a clear message asking the user to install the Homebrew rsync.
- [x] Tests: simulated rsync output (fake executable); FR/EN/DA i18n.

**Point to check on the NAS**: if the QNAP module points at the whole share, the picker sees everything below it. Advice: a module that points only at the library.

## Phase 2 — Deduplication against the remote library

- [x] Library listing: `rsync --list-only -r` on the chosen folder (name, size, date); cached locally, with a validity period.
- [x] A photo is **skipped** if the library already contains a file at the same destination path, or with the same name, the same size and the same capture date (finer fingerprint below).
- [x] **Fingerprint index**: a lightweight file (SHA-256, size, date) of the uploaded photos, kept locally and copied to the NAS; detects a duplicate stored elsewhere or renamed.
- [x] Report before sending: to send / already present / skipped, also visible in simulation.
- [x] Tests: simulated library with identical, renamed and moved paths.

## Phase 3 — Tags

- [x] First check **where the tags are stored** today (`internal/tagger`).
- [x] Send them with the photos, via rsync: files next to the photos (XMP) or an index file, to be chosen after checking.
- [x] Never overwrite tags already present on the NAS without saying so.

## Phase 4 — Safeguards (D4, D5)

- [x] Single function that builds the rsync arguments; whitelist of options.
- [x] **Automated test**: no generated command contains `--delete*` or `--remove-source-files`.
- [x] The simulation (`--dry-run`) is offered before the first upload to a new destination.
- [x] Write only under the folder chosen as the library.

## Phase 5 — Site guide

- [x] Update the `tri-distant` page (FR/EN/DA): remote folder choice, deduplication, tags, safeguards, recommended QNAP module, single-user limitation.

## Phase 6 — Acceptance test on the QNAP `[user action]`

- [ ] Connection test, folder browsing, simulation, real upload of a small batch.
- [ ] Re-run: nothing is resent. Resume after an interruption: the upload restarts.
- [ ] Check in QuMagie.
- [ ] Report any error messages that were not understood, for the guide.

## Risks

- The macOS system rsync is old: some options are missing (progress, `--mkpath`) → detection and fallback.
- Listing a library of 200,000 photos with `--list-only -r`: slow over a remote link → cache and manual refresh.
- Behaviour of the QTS rsync server not verified (module limits, folder visibility) → test in phase 6.

## Out of scope

Reading or sorting from the NAS, USB import from the iPhone, watched folder, Docker server on the NAS, SFTP/WebDAV.
