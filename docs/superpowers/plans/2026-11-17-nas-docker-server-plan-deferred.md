# Plan: sorting and tags on the NAS, local network connection

Date: 2026-11-17 · Branch: `feat/nas-qnap-remote` · Status: **deferred** (no code written)
Parent spec: `docs/superpowers/specs/2026-11-16-nas-qnap-remote-design.md`

> **Deferred.** This plan (Go server in Docker) requires Container Station, which
> the user has not installed, and SSH access, which they refuse to enable. With
> a few hundred photos to process, it is not needed for the local
> connection: see `2026-11-17-nas-local-connection-plan.md` (Mac + SMB,
> nothing to install on the NAS). This plan remains the basis for **automatic
> classification** (processing without the Mac) and for remote access through an overlay.
> The rights to impose on the container (D1, D3, D4, hardening) remain valid.

## Goal

From the Electron app on the Mac, on the same local network, connect to a
`photosort-server` running in Docker on the QNAP TS-251D: sort
the loose input, tag the sorted library and produce the weekly
planner PDF. **The container can touch nothing else**: even if compromised, it
modifies neither the iPhone folder nor the rest of the NAS. Remote access will come
in a second plan, without changing what the container is allowed to do.

## Constraints

- Business logic in Go; existing engine reused (`sorter`, `internal/tagger`, `internal/scrapbook`).
- **Input never written to**: three independent locks (QTS rights of the dedicated user, Docker `:ro` mount, guard at server startup).
- Dedicated QTS user `photosort`: read-only on the input, read-write on the library, **no rights elsewhere**.
- Hardened container: non-root, `cap_drop: ALL`, `no-new-privileges`, read-only root, no `privileged`, no host network, no `docker.sock`.
- Local network only; the server listens through a `net.Listener` interface (replaceable later by an overlay).
- TS-251D: 2 cores, 2 to 4 GB shared with QTS and QuMagie → bounded parallelism and memory.
- Branch `feat/nas-qnap-remote`, no merge into `main` without a request, `Co-authored-by` trailer on every commit.
- No new dependency on the Electron side if possible (`node:tls`, `safeStorage`).

## Decisions (adopted by default, to be confirmed)

| # | Decision | Reason |
| --- | --- | --- |
| D1 | **Copy, no hard link.** The hard link is abandoned | With `fs.protected_hardlinks=1`, a user can only link files they own or on which they have read **and write** access ([kernel doc](https://docs.kernel.org/admin-guide/sysctl/fs.html)): incompatible with a read-only input. Trade-off: the disk space of the sorted photos adds up (space check before copying) |
| D2 | Transport: **TLS + line-by-line JSON**, the sidecar protocol as is | `serve(ctx, io.Reader)` and `send` already exist; Electron speaks it with `node:tls`. No WebSocket: no dependency, and Node 20's global `WebSocket` (Electron 33.4.11 embeds Node 20.18.3) is not guaranteed |
| D3 | **Two fixed roots**: `/inbox` (read-only) and `/library` (write) | Minimal attack surface; no remote folder browser in v1 |
| D4 | Authentication: 256-bit token (the server keeps only its SHA-256 fingerprint) + self-signed certificate **pinned by fingerprint**, checked **before** sending the token | The token never travels to an unrecognised server |
| D5 | Disconnection = **cancellation** of operations in progress; resume by re-running the plan (already-copied duplicates are ignored, to be tested) | Same semantics as the sidecar; detached operations and resumption by identifier deferred to the "remote access" plan |
| D6 | PDF: generated in the server's working folder then brought back by a `fetch` operation (base64 blocks); the renderer does not change | The PDF arrives in the folder chosen in the preferences (Downloads by default) |
| D7 | Image delivered as a **`docker save` archive** (no registry required) | The repository has no remote; import into Container Station |
| D8 | Metadata tags enabled; **AI tags disabled until the NAS step has measured** | Celeron probably 10 to 30 times slower; AVX not guaranteed |

## Phase 0 — Documentation (API inventory)

Done inline: stable standard-library APIs and repository code, no sub-agents.

**Allowed APIs**
- Go (1.27.3): `crypto/tls` (`MinVersion: tls.VersionTLS13`, `tls.NewListener`), `crypto/x509.CreateCertificate`, `crypto/ecdsa` + `elliptic.P256`, `crypto/sha256` (fingerprint of `cert.Raw`), `crypto/subtle.ConstantTimeCompare`, `crypto/rand`, `net.Listener`, `syscall.Access` and `syscall.Statfs` (build tag `unix`), `os.OpenRoot` (already used: `sorter/sorter.go:415`).
- Electron 33.4.11 / Node 20.18.3 (verified in the binary): `tls.connect`, `socket.getPeerCertificate().fingerprint256`, `readline` (already used: `main/index.ts:4`), `safeStorage.isEncryptionAvailable/encryptString/decryptString`.
- Docker 29.8.2 locally; compose keys: `user`, `read_only`, `tmpfs`, `cap_drop`, `security_opt: no-new-privileges:true`, `mem_limit`, `cpus`, `pids_limit`, `:ro` volumes.
- ONNX Runtime: `photosort-electron/scripts/fetch-ai-assets.mjs` already knows how to fetch `linux-amd64` (pinned sha256): to be reused in the image.

**Do not**
- Do not expose `aiDir`, `outDir`, `fallbackDir` to network clients: the server imposes its own.
- Do not present `os.Root` as a confinement of the engine: it is used only for the copy; the real barrier is the system (user, mount), path validation is only the second one.
- Do not use `rejectUnauthorized:false` without checking the fingerprint before any write; never store the token in clear text.
- Do not run the container as root, nor mount the Docker socket.

**Not verified (to be resolved in phase 7)**: application of QTS rights to Docker mounts; compose in Container Station; UID/GID of the QTS user; AVX and ONNX Runtime on the Celeron; readability of the copies by QuMagie.

## Phase 1 — Refactor with no change of behaviour

**Output**: the sidecar's server becomes a reusable package.
**Verification**: `go test ./cmd/photosort-sidecar/... ./internal/rpc/...`: existing tests unchanged and green; the sidecar starts and responds as before.

- [ ] Move `server`, `thumbCache` and the operations (`cmd/photosort-sidecar/{main,cache,scrapbook}.go`) to `internal/rpc` (`NewServer`, `Serve(ctx, in, out)`), with a `Policy` interface called before each request (default identity: everything is allowed, like the sidecar)
- [ ] Make the PDF output folder and the number of workers configurable (options)
- [ ] `cmd/photosort-sidecar/main.go`: thin call, behaviour unchanged
- [ ] Move the tests along with the code (`main_test.go`, `scrapbook_test.go`)

## Phase 2 — Engine: copy options and space check `[parallel with 1]`

**Output**: the copy accepts a file mode; free space can be checked.
**Verification**: `go test ./sorter/...`; `CopyWithProgress` keeps mode 0600 by default (local behaviour unchanged).

- [ ] `sorter.CopyOptions{FileMode}` + `CopyWithOptions`; `CopyWithProgress` becomes its wrapper (today `copyEntry` writes with 0600, which would make the copies unreadable for QuMagie and SMB: `sorter/sorter.go:435`)
- [ ] `FreeSpace(dir)` function (`syscall.Statfs`, build tag `unix`) and comparison with the plan's size
- [ ] Tests: mode 0644 applied; clean failure if space is lacking; copy interrupted with no partial file (existing behaviour, to be locked in by a test)

## Phase 3 — `photosort-server` server `[checkpoint]`

**Output**: `cmd/photosort-server` binary + `internal/remote` package.
**Verification**: `go test ./internal/remote/... ./cmd/photosort-server/...` (integration on `127.0.0.1:0`, temporary folders, real TLS client).

- [ ] Contract: `docs/nas-protocol.md` (`hello`, `status`, `ping`, `fetch`, error codes, protocol version)
- [ ] **Roots and paths**: `Roots{Inbox, Library}`, `Resolve(path)` (refusal of `..`, of paths outside the roots, of the neighbouring prefix `/inbox2`, of symbolic links that escape), non-nested roots
- [ ] **Per-operation policy**: `plan` (sources under `/inbox`, destination forced to `/library`), `copy` (by server `planId`: the client injects no path), `thumbnail` (under `/inbox` or `/library`), `tag` and `weekDays/weekPhotos/scrapbook*` (under `/library` only), `fetch` (working folder only), `cancel`, `ping`, `status`; any other operation is refused
- [ ] **Startup check (fail closed)**: refuse if uid 0; refuse if `/inbox` is writable (`syscall.Access(W_OK)`, which takes `EROFS` and ACLs into account); `/library` and the state folder must accept writing (witness file created then deleted); `selfcheck` subcommand for the NAS
- [ ] **TLS and token**: on first start, ECDSA P-256 certificate (SAN from `PHOTOSORT_HOSTS`), random token displayed only once in the logs with the certificate fingerprint; only the token's hash is written (`0600`)
- [ ] **Connection**: `hello` expected within 5 s, line limited to 4 KB before authentication, constant-time comparison, delay after failures per address, **a single active client** (the second receives `busy`), `ping` every 15 s and closing after 45 s of silence, operations cancelled on close
- [ ] `status`: startup check results, version, free space, AI available; `fetch`: 256 KB blocks of files from the working folder only; PDFs cleaned up after being brought back or after 1 h
- [ ] Audit log on standard output (time, address, operation, result; never the token)
- [ ] AI mode: see risk 1 (probe in a child process)
- [ ] Tests: wrong token, no `hello`, line too long, second client, forbidden operation, path escapes, `plan` then `copy` with files at 0644 and **input identical before/after** (tree checksum), `fetch` byte for byte, interruption mid-copy with no partial file

## Phase 4 — Docker image and compose `[parallel with 5]`

**Output**: `Dockerfile`, `docker-compose.yml`, `scripts/build-nas-image.sh`, archive `dist/photosort-server-amd64.tar`.
**Verification** (local Docker): hardened container started with a `:ro` input; `docker exec … touch /inbox/x` fails (`Read-only file system`); `docker inspect` shows neither `Privileged` nor capabilities; phase 3 integration test replayed against the container; image size noted.

- [ ] Multi-stage Dockerfile (Go with CGO, `libonnxruntime.so` and models via `fetch-ai-assets` for the target platform, thin runtime image, numeric `USER`)
- [ ] Compose: `user: "<uid>:<gid>"`, `/inbox:ro`, `/library`, state folder, `read_only`, `tmpfs: /tmp`, `cap_drop: [ALL]`, `no-new-privileges`, `mem_limit`, `cpus`, `pids_limit`, port published on the NAS's LAN address (`IP:8443:8443`), never `privileged` nor host network
- [ ] Script: `linux/amd64` build (emulated on the M2) and `linux/arm64` (local tests), compressed `docker save` export
- [ ] Lock in with a test what is already true in the code: `tagger.New` initialises ONNX Runtime only for scenes and people (`internal/tagger/tagger.go:117`), so metadata and moment tags work without `libonnxruntime.so`

## Phase 5 — Electron, main process `[parallel with 4]`

**Output**: second network backend behind the same interface as the sidecar.
**Verification**: `npm run typecheck`; `node --test` (Node 24 type stripping, no dependency) against a real `photosort-server` run locally.

- [ ] `src/main/backend.ts`: `Backend` interface (`send`, events); `LocalSidecar` (current code) and `RemoteServer`; `ipcMain.on('sidecar:send')` switches depending on the mode
- [ ] `src/main/remote.ts`: `probe(host, port)` (reads the fingerprint without sending anything); `connect` with `fingerprint256` verification at `secureConnect`, **before** writing the `hello`; heartbeat; synthetic `error` event on connection loss; manual reconnection
- [ ] Bringing the PDF back: on the `done` event of a `scrapbook`, download by `fetch` into the folder chosen in the preferences, then rewrite `path` as a local path: the renderer is not modified
- [ ] `src/main/secrets.ts`: token encrypted by `safeStorage` in `userData`; if encryption is unavailable, write nothing and ask for the token again each session
- [ ] IPC and `preload`: `nas.probe`, `nas.connect`, `nas.forget`, `nas.status`, `backend.use`
- [ ] Tests: a different fingerprint is refused **without a single byte being sent** (fake TLS server that counts the bytes received); `hello`; thumbnail; `fetch` with identical checksum

## Phase 6 — Electron, interface `[checkpoint: visual review]`

**Output**: connection screen and NAS mode.
**Verification**: `npm run typecheck`, `npm run build`, manual walkthrough against the local container, screenshot.

- [ ] Connection screen: address, port, token; step "Compare the fingerprint with the one in the container logs" before validation
- [ ] Status card: animated security list (input read-only ✔, library writable ✔, non-root ✔, TLS pinned ✔, free space, version); red and explicit if a check fails
- [ ] NAS mode: "This Mac / NAS" selector; input and library shown as fixed roots, with no folder pickers or drag-and-drop; "AI tags" checkbox hidden until the server announces it
- [ ] Disconnection banner with "Reconnect"; notification of the PDF brought back
- [ ] fr/en/da i18n strings; animations in the existing style (`framer-motion`)

## Phase 7 — Acceptance test on the NAS `[checkpoint: user action]`

**Output**: `docs/nas/installation.md` (step-by-step guide in French) and `docs/nas/measurements.md` (measurements).
**Verification**: acceptance checklist signed by the user.

- [ ] Guide: create the QTS user `photosort` (non-administrator, no application access), rights matrix on the two shared folders (input: read-only; library: read-write; all others: no access), state folder, import of the archive into Container Station, compose, reading the token and fingerprint
- [ ] Acceptance: `status` entirely green; **negative proof**: temporarily give write access on the input, the container must refuse to start, then restore; record `id photosort` and `fs.protected_hardlinks`
- [ ] QuMagie reads the copies (0644 rights); index only the library
- [ ] Measurements on a sample: plan duration, metadata tags, one AI tag, ONNX Runtime startup, memory peak (`docker stats`), CPU; decision on AI tags and worker bounds

## Phase 8 — Documentation and CI `[parallel]`

**Verification**: valid YAML; the produced archive loads with `docker load`.

- [ ] README (NAS section) and update of the spec (decisions D1 to D8)
- [ ] Optional CI step building the `linux/amd64` image as an artefact (`workflow_dispatch`)

## Risks

1. **ONNX Runtime without AVX on the Celeron**: a `SIGILL` would kill the server and cannot be recovered within the process → initialisation probe in a **child process** at startup; on failure, `status` announces `ai:false` and AI tags remain unavailable, the rest works.
2. **QTS rights not applied to Docker mounts as expected** → the locks are independent (`:ro` mount and startup guard fail closed); verified by the negative proof in phase 7.
3. **Disk**: the copy doubles the space of the sorted photos → space check before copying, estimate given to the user.
4. **Mac asleep mid-copy** → clean cancellation (no partial file), resume with a new plan.
5. **Malicious image in the input targeting a decoder**: mitigated by the very purpose of the plan (non-root, no capabilities, no write access to the input, nothing else mounted).
6. **Token and fingerprint at first connection**: the user must compare the fingerprint with the logs; the token never leaves before this validation.
7. **Container Station**: compose support and role of `user:` not verified → phase 7.

## Out of scope

Remote access (second plan: Tailscale `tsnet` or NetBird, or myQNAPcloud Link with MFA; the invariant above applies to it as is), "sort automatically" checkbox, hard link, remote folder browser, automatic discovery (mDNS), multi-user, detached operations with resumption by identifier, moving or deleting originals.

## Open questions

1. Are the input and the library **two distinct shared folders** (simple rights) or two subfolders of the same share (advanced QTS ACLs)?
2. Does the sorted library already exist, or will it be created empty? With how much free space on the disk?
3. Is Container Station installed and SSH access allowed (to record `id photosort`; otherwise phase 7 goes through the interface)?
4. Order of magnitude of the number of photos in the input?
