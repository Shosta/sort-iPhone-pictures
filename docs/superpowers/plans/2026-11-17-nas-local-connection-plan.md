# Plan: sorting and tags on the NAS, local network connection (Mac + SMB)

Date: 2026-11-17, revised on 2026-11-18 (Tailscale) · Branch: `feat/nas-qnap-remote` · Status: **revised plan, awaiting approval** (no code written)
Parent spec: `docs/superpowers/specs/2026-11-16-nas-qnap-remote-design.md`
Deferred plan (Docker server): `docs/superpowers/plans/2026-11-17-nas-docker-server-plan-deferred.md`

## Why this plan replaces the previous one

User's answers (2026-11-17):
- input and library are **two subfolders of the same share**;
- the library already exists and is **massive**;
- **Container Station is not installed** and **SSH remains disabled** (attack surface);
- the input contains **a few hundred photos**.

Consequences: the Docker server (Container Station + image import) would add
a root daemon and maintenance to the NAS to process a few hundred
photos. The app already knows how to read an input folder and write to a
destination folder: an SMB share mounted on the Mac is enough. **Nothing to install on the
NAS, no new port.** The Mac (Apple M2) also makes AI tags
realistic, which the NAS's Celeron did not guarantee.

## Revision of 2026-11-18: Tailscale

New fact: direct SMB on the local network "does not work optimally" (timeouts from the Mac, correct mount from Kubuntu). The user installs **Tailscale** (WireGuard mesh VPN) on their Linux machine, their iPhone and the QNAP NAS.

What this changes:
- **The transport becomes the tailnet.** The SMB share remains the chosen route, but it is reached through the NAS's MagicDNS name or `100.x` address: no more Bonjour discovery or dependence on the local network, and traffic is encrypted end to end.
- **Remote access, previously out of scope, enters this plan** (through the same SMB mount). It must never widen the rights of the `photosort` account.
- **The constraint "nothing installed on the NAS" is lifted for a single piece of software: Tailscale.** The engine itself does not change.
- **Latency becomes the main risk.** SMB chains many small requests; remotely or through a relay, walking a massive library can become very slow. Scoped deduplication (phase 2) goes from a comfort to a necessity.
- **The reference client becomes the Linux machine** (Tailscale is installed there). The work Mac is not in the tailnet: we do not assume Tailscale can be installed on it (no administrator rights). It stays on the local network, to be diagnosed separately.

## Goal

From the Electron app on a tailnet machine (Linux first, the Mac on the local network): sort the loose
input into the NAS library, tag the library and produce the weekly planner
PDF, with a dedicated QTS account with minimal rights, at the office as well as remotely through
Tailscale. Processing without a client machine (server on the NAS) will come in a later
plan.

## Constraints

- Business logic in Go; existing engine reused (`sorter`, `internal/tagger`, `internal/scrapbook`).
- **Input never written to**, library is the only writable folder.
- Dedicated QTS account `photosort`: read-only on the input, read-write on the library, **no rights elsewhere**, SMB only.
- **Only addition on the NAS: Tailscale.** No SSH, no Container Station, no other third-party QPKG.
- **Never port 445 on the internet.** Tailscale Funnel, Tailscale Serve, subnet routes and exit node: forbidden on the NAS.
- Current local behaviour does not change by default (anything new is an option or a neutral hardening).
- No secret in the app: the mount is done by the OS (macOS keychain).
- Branch `feat/nas-qnap-remote`, no merge into `main` without a request, `Co-authored-by` trailer on every commit.

## Decisions (adopted by default, to be confirmed)

| # | Decision | Reason |
| --- | --- | --- |
| N1 | **Route A': the app on a client machine, the share mounted over SMB** with the dedicated account, **through Tailscale**. The Docker server remains deferred | No SSH or Container Station; the engine does not change; Tailscale handles access and encryption |
| N2 | Rights: QTS **Advanced Folder Permissions** (per-subfolder rights: read, read-write, deny) | Documented by QNAP; only way to separate two subfolders of the same share without Docker. Global NAS setting: see risks |
| N3 | Mounting by the **OS** (Finder `smb://`, Windows network drive, `mount.cifs`, GVFS under KDE) on the NAS's MagicDNS name; the app mounts nothing and never sees the password | The keychain manages the secret; no dependency |
| N4 | **Prior check** `nasCheck` (Go): the input must refuse writing, the library must accept it | Second barrier: if the QTS ACL is misconfigured, the app refuses to launch the operation (fails closed) |
| N5 | **Copy** (no hard link) to the library | A hard link does not work over an SMB mount and contradicts the read-only input |
| N6 | **Scoped** deduplication (option): only walk, in the library, the months concerned by the plan | `loadDestination` (`sorter/dedupe.go:44`) walks the whole destination with one `lstat` per file: costly over SMB with a massive library |
| N7 | Ignore **system** folders and files (`.@__thumb`, `@Recycle`, `@Recently-Snapshot`, `@eaDir`, `._*`, `.DS_Store`) | `Plan` (`sorter.go:122`) and `deduper` skip no hidden folder whereas `tags.go:39` and `scrapbook/scan.go:101` already do; QTS creates `.@__thumb` everywhere |
| N8 | **Atomic** copy: write to `<target>.part` then rename | A network interruption would otherwise leave a truncated file under the final name (`copyEntry`, `sorter.go:415`), later mistaken for a valid photo |
| N9 | Weekly planner and tags unchanged: library read over SMB, **PDF written locally** (Downloads) | Nothing to bring back |
| N10 | AI tags **allowed** (computed on the client machine) | The Celeron blocker disappears |
| N11 | **Minimal Tailscale ACLs**: only the device that runs Photosort (dedicated tag) reaches the NAS, and only on port **445**; the iPhone does not access it; no subnet route, no exit node, neither Serve nor Funnel on the NAS | The tailnet must not reopen what the QTS rights close: a stolen tailnet device must not reach the QTS administration interface |
| N12 | **Key expiry disabled for the NAS** (Tailscale admin console) | Otherwise the NAS leaves the tailnet when its key expires and remote access drops without warning |
| N13 | The QTS rights (`photosort` account, advanced permissions) remain **the main barrier**; Tailscale does not replace them | Even if a tailnet device is compromised, it only finds an account with minimal rights |
| N14 | **Scoped deduplication enabled by default in NAS mode** when the library follows `YYYY/YYYY-MM`, and **relay detected**: warning in the acceptance test if `tailscale ping` goes through a DERP relay | One `lstat` per file on a high-latency link is the worst case; a relay multiplies latency |
| N15 | Number of parallel copies **adjustable** (4 by default), to be tuned from the acceptance test | On a high-latency link, more workers hide latency; to be measured, not guessed |
| N16 | The Docker server (TLS, token, pinned certificate) remains **deferred**; Tailscale would already provide authentication and transport encryption, which would simplify D2 and D4 | To be resumed only if processing without a client machine becomes necessary |

## Phase 0 — Documentation (API inventory)

Done inline: stable standard-library APIs and repository code, no sub-agents.

**Verified in the code**
- `Plan` walks the sources with `WalkDir` without skipping hidden folders (`sorter.go:122`); `deduper.loadDestination` walks the whole destination (`dedupe.go:44-60`) and only hashes files of the **same size** as a source (`dedupe.go:69`); the destination is compared only **byte for byte** (`register`, `dedupe.go:90`).
- `copyEntry` creates the final file with `O_EXCL`, mode 0600 (`sorter.go:435`) and only cleans up on a caught error.
- `copyWorkers = 4` (`sorter.go:330`): suited to a network disk.
- `tags.go:39` and `scrapbook/scan.go:101` skip folders whose name starts with `.`.

**Verified in the QNAP documentation**: *Control Panel > Privilege > Shared Folders > Advanced Permissions > Enable Advanced Folder Permissions* allows setting read, read-write or deny per user for each folder and subfolder ([QTS 5.2](https://docs.qnap.com/operating-system/qts/5.2.x/en-us/configuring-advanced-folder-permissions-16E9201B.html), [QNAP FAQ](https://www.qnap.com/en/how-to/faq/article/should-i-enable-advanced-folder-permissions-or-windows-acl-support)). "Windows ACL" is an alternative, configured from Windows Explorer: not chosen.

**Do not**
- Never test writing to the input other than by an attempt that **must fail**; if it succeeds, delete the probe and refuse.
- Do not store a password in the app; do not mount a share from the app.
- Do not expose SMB (port 445) to the internet, nor forward it on the router.
- Do not change the default local behaviour (full deduplication of the destination kept).

**Not verified (to be resolved in phase 6)**: effect of Advanced Folder Permissions on existing access; mode of created files (0600) as seen by QuMagie and the other accounts; SMB security options available in the installed QTS version (minimum SMB version, signing); the account's access to QTS applications ("Application Privilege" restriction); actual duration of walking the library over SMB; Unicode normalisation of accented place names; organisation of the existing library (see questions).

## Phase 0b — Prepare the NAS `[user, no lines of code]`

**Output**: a `photosort` account that can only read the input and write the library, and a share mounted on the Mac.
**Verification** (manual, 5 checks): with this account mounted on the Mac, (1) creating a file in the input fails, (2) deleting or renaming a photo in the input fails, (3) creating then deleting a file in the library succeeds, (4) the other folders of the share and the other shares are denied, (5) the existing accounts (the iPhone backup, QuMagie) work as before.

Step-by-step procedure: [`docs/nas-qnap.md`](../../nas-qnap.md). Summary:
- [ ] Back up the QTS configuration (*Backup / Restore*) before any change.
- [ ] Create the `photosort` user, long and unique password, **outside** the `administrators` group.
- [ ] Enable *Advanced Folder Permissions* (global setting).
- [ ] On the share: `photosort` read-write on the library, **read-only** on the input, **deny** on the rest; deny on all other shares. QNAP rule: a read-only right or a deny on the root applies to the subfolders; the root therefore gets read-write with *Only admin can create files and folders* and *Only the owner can delete the contents* (owner `admin`).
- [ ] Limit the applications accessible to the account (SMB only; no File Station, FTP, SSH).
- [ ] SMB options (to be confirmed in QTS): minimum version SMB 3, signing enabled.
- [ ] Mount `smb://<nas>/<share>` from the Finder with this account, save the password in the keychain.

## Phase 0c — Prepare Tailscale `[user, no lines of code]`

**Output**: the Linux machine reaches the NAS by its MagicDNS name, over a direct connection, and only this Linux machine can reach port 445.
**Verification** (manual):
- [ ] `tailscale status` on the Linux machine lists the NAS and the iPhone; the NAS appears as connected.
- [ ] `tailscale ping <nas>` eventually replies **"via … direct"** (not "via DERP" continuously). A relay is usable but slower: note which one.
- [ ] `tailscale ip -4 <nas>` returns a `100.x` address.
- [ ] From the Linux machine, `smbclient -L //<nas> -U photosort` (or the mount from step 7) succeeds through the MagicDNS name.
- [ ] From the iPhone **without** ACL authorisation, the NAS remains inaccessible on 445 (once the ACLs are set).

Tasks:
- [ ] **Origin of the package on the NAS**: install Tailscale from the official QNAP catalogue or the package published by Tailscale, not from a third-party source; note the version. **(to be confirmed in your App Center)**
- [ ] Admin console: **disable key expiry** for the NAS (N12) and give it a tag (for example `tag:nas`).
- [ ] ACL (N11): explicit rule `work Linux -> tag:nas:445`; implicit deny for the rest. Do not advertise subnet routes or an exit node from the NAS.
- [ ] Check that neither Funnel nor Serve is active on the NAS.
- [ ] Linux: mount the share. Two options, to be compared in the acceptance test: `mount.cifs` (administrator rights needed, or an `fstab` entry with `user`) or GVFS/Dolphin (`smb://<nas>/<SHARE>`, no administrator rights, path under `/run/user/<uid>/gvfs/`). GVFS goes through FUSE: test that `O_EXCL` creation and the rename of the atomic copy (phase 3) work there.
- [ ] Opening `smb://` access by the MagicDNS name may require allowing only the tailnet address in *Microsoft Networking host access*: **to be confirmed** (the source address seen by QTS depends on Tailscale's network mode on the NAS).

**Not verified**: whether the QNAP Tailscale package uses a TUN device or "userspace networking" mode, and what QTS then sees as source address; the real throughput of an SMB mount through the tunnel.

## Phase 1 — Engine: ignore system names

**Output**: a single point decides which folders and files are ignored, used by the four walks.
**Verification**: `go test ./sorter/... ./internal/scrapbook/...`; test with a tree containing `.@__thumb/x.jpg`, `@Recycle/y.jpg`, `._IMG_1.HEIC`, `.DS_Store`, `vrai/IMG_2.HEIC`: only `vrai/IMG_2.HEIC` is seen by `Plan`, `deduper`, `PhotosToTag` and `scanMonth`.

Tasks:
- [ ] Add `SkipDir(name)` and `SkipFile(name)` in `sorter` (hidden name `.`, prefix `@`, `._*`, `.DS_Store`, `Thumbs.db`).
- [ ] Use them in `Plan` (`sorter.go:122`), `deduper.loadDestination` (`dedupe.go:44`), `PhotosToTag` (`tags.go:39`) and `scanMonth` (`scan.go:101`, which keeps its current rule plus the new one).
- [ ] Tests (`sorter_test.go`, `tags_test.go`, `scan_test.go`).

## Phase 2 — Engine: scoped deduplication `[after 1: same files]`

**Output**: `Engine.ScopedDestination` option; default in the engine, behaviour unchanged; **enabled by default by Electron's NAS mode** (N14).
**Verification**: `go test ./sorter/...`; test with 24 months of dummy files and 2 months in the plan: the walk only visits those 2 months (counter of visited files, no stopwatch), and a byte-for-byte duplicate in one of those months is still detected. Without the option, all months are visited.

Tasks:
- [ ] `newDeduper(destination, scope)`: if `scope` is non-empty, `loadDestination` only walks `<destination>/<YYYY>/<YYYY-MM>` for the months present in the plan; the list comes from step 1 of `Plan` (each photo's folder is already computed before the fingerprints).
- [ ] Document the assumption: an identical file has the same EXIF date, therefore the same month, **if the library follows Photosort's tree**. Otherwise, leave the option disabled.
- [ ] Expose `scopedDestination` in the sidecar's `plan` request (optional boolean, default `false`).

## Phase 3 — Engine: atomic copy and resumption after interruption `[after 2]`

**Output**: `copyEntry` writes to `<target>.part` then renames; an interruption never leaves a truncated file under the final name.
**Verification**: `go test ./sorter/...`; test: source truncated mid-copy (failing reader) → the final target does not exist, the `.part` is deleted; an orphan `.part` from a previous run is overwritten; an existing target is never replaced; a successful copy keeps the exact content.

Tasks:
- [ ] `copyEntry` (`sorter.go:415`): open `<target>.part` with `O_CREATE|O_TRUNC|O_WRONLY`, `Sync`, check that the target does not exist (`root.Lstat`), `root.Rename`.
- [ ] Retry (3 times, increasing wait) on transient network errors during the copy (`EIO`, `ETIMEDOUT`, `ENOTCONN`, `ESTALE`, `EHOSTDOWN`); clear failure afterwards, resumption consists in re-running the plan (already-copied duplicates ignored).
- [ ] **Conditional**: if the acceptance test (phase 6) shows that mode 0600 makes the copies unreadable for QuMagie or other accounts, add `CopyOptions{FileMode}` (0644 on the NAS, 0600 locally).

## Phase 4 — Prior check `nasCheck` `[parallel with 1 to 3: other files]`

**Output**: `nasCheck` sidecar operation (same request envelope as `plan`).
**Verification**: `go test ./cmd/photosort-sidecar/...`; tests on temporary folders: input at `0555` → `inputReadOnly: true`; writable input → `inputReadOnly: false` **and probe deleted**; writable library → `libraryWritable: true`; missing path or not a folder → clear error.

Tasks:
- [ ] `internal/nascheck` package: `Check(input, library string) (Result, error)` with `{mounted, inputReadOnly, libraryWritable, problems[]}`. The input probe is a file creation that **must** fail (`EACCES`, `EPERM`, `EROFS`); any other error is a closed failure; if it succeeds, immediate deletion and `inputReadOnly: false`. The library probe is a hidden file `.photosort-probe-<random>` created then deleted.
- [ ] `nasCheck` operation in `cmd/photosort-sidecar/main.go` and test in `main_test.go`.
- [ ] Note: on Windows, `EACCES` translates to `ERROR_ACCESS_DENIED`; the package normalises via `errors.Is(err, fs.ErrPermission)`.

## Phase 5 — Electron: NAS mode `[after 2 and 4; checkpoint: visual review]`

**Output**: in the preferences, a "NAS" block (input folder, library, "library organised by Photosort" checkbox = scoped deduplication, "check the input is read-only" checkbox ticked by default); before each sort or tag in NAS mode, a `nasCheck` check with a clear message on failure; input and library remembered as default folders.
**Verification**: `npm run typecheck`; screenshot of the preferences screen and of the failure message; manual test: writable input → sort refused with the explanation; NAS unmounted → message "folder not found, mount the share".

Tasks:
- [ ] Preferences and types (`photosort-electron/src/renderer/…`), fr/en/da translations.
- [ ] `nasCheck` call before `plan`, and propagation of `scopedDestination`.
- [ ] Error messages: share not mounted, writable input, read-only library.
- [ ] Visual review (light/dark themes).

## Phase 6 — Acceptance test on the NAS `[checkpoint: user action]`

**Output**: a quantified report.
**Verification**: run on the real photos and note:
- [ ] the 5 checks of phase 0b;
- [ ] duration of `plan` with and without scoped deduplication, on the real library;
- [ ] duration of copying a few hundred photos;
- [ ] a byte-for-byte duplicate photo is indeed ignored;
- [ ] an accented place (e.g. `Orléans`) does not create two folders;
- [ ] QuMagie sees the copies after reindexing, the `.xmp` files do not disturb it;
- [ ] mode of created files (0600): readable by the other accounts? if not, conditional task of phase 3;
- [ ] metadata then AI tags on one month, reading and writing of `.xmp` over SMB;
- [ ] deliberate Wi-Fi cut during a copy: no truncated file, clean resumption.

## Phase 7 — Documentation `[parallel]`

- [ ] `docs/nas-qnap.md` (written, without screenshots): adjust it after the acceptance test (menus actually observed, possible screenshots) and link it from the README.
- [ ] `README.md`: "QNAP NAS on the local network" section, pointing to this document; Windows (network drive) and Linux (`mount.cifs`) notes.
- [ ] Spec: status updated.

## Risks

- **Advanced Folder Permissions is global** → configuration backup beforehand, check 5 of phase 0b; rollback possible by disabling it (the QNAP doc notes effects on existing ACLs).
- **Only two locks on the input** (QTS rights, prior check) instead of three with Docker → check run at every launch, fails closed, account with no other rights, the engine never opens for writing outside the destination.
- **SMB performance not measured** (and **latency increased by the tunnel**, especially through a DERP relay) → scoped deduplication, adjustable parallel copy, progress already displayed; `tailscale ping` direct/relay and figures recorded in phase 6. If SMB remains too slow even on a direct connection, the fallback route is the server on the NAS (deferred plan), whose security Tailscale would simplify.
- **Tailscale on the NAS is third-party code installed with elevated privileges** → official package only, updates followed, minimal ACLs (N11), neither Serve nor Funnel nor routes; a compromised tailnet device must only reach port 445, behind the `photosort` account.
- **NAS key expired** → expiry disabled (N12); otherwise access drops without a clear message.
- **Work Mac outside the tailnet** → it stays on the LAN; the local SMB diagnosis (`~/photosort-smb-diag.sh`) remains to be handled separately, without blocking the Linux machine.
- **iPhone photos**: the iPhone, already in the tailnet, could send its photos to the input remotely; outside this plan, and not to be opened beyond port 445 limited by the ACL.
- **Mac asleep or Wi-Fi cut during a copy** → atomic copy, idempotent re-run.
- **The `photosort` account leaks** → it can only read the input and write the library; no QTS application, no SSH.
- **Library organised differently from Photosort** → the scope option stays disabled; full walk with progress.

## Out of scope

- Docker server on the NAS and automatic classification without a client machine: later plan (see N16).
- NetBird, myQNAPcloud Link and QVPN: ruled out, Tailscale is installed.
- Tailscale detection by the app: the app mounts nothing and does not talk to Tailscale; it only shows a generic message "NAS unreachable: is Tailscale connected?".
- Free-space check before copying (a few hundred photos).
- Moving or deleting originals, remote folder browser, automatic mounting from the app.

## Open questions

1. Do you validate the **SMB through Tailscale** route (Linux first, Mac on the LAN), with the Docker server remaining deferred?
2. Do you agree to enable **Advanced Folder Permissions** (global NAS setting)? Otherwise, the input would be protected only by the app (less strict).
3. Does the existing library already follow Photosort's `YYYY/YYYY-MM` tree? (determines scoped deduplication)
4. Does QuMagie index the input, the library, or both?
5. Is the NAS in TUN mode or "userspace networking" with the Tailscale package installed, and is `tailscale ping` direct?
6. Do you want the iPhone to be able to drop its photos into the input remotely (separate plan)?
