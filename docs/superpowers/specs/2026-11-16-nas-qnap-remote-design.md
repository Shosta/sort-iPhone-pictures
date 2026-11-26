# Sorting and tags on a QNAP NAS: scoping

Date: 2026-11-16 · Branch: `feat/nas-qnap-remote` · Status: **scoping; local-connection plan revised (Mac + SMB), awaiting approval** (no code written)

Local-connection plan: `docs/superpowers/plans/2026-11-17-nas-local-connection-plan.md`.
Docker server plan (deferred): `docs/superpowers/plans/2026-11-17-nas-docker-server-plan-deferred.md`.

> **Revision of 2026-11-17.** Container Station is not installed, SSH remains
> disabled, the input contains only a few hundred photos and the existing
> library is massive. The local v1 therefore goes through approach **A'**
> (the app on the Mac, the share mounted over SMB with a dedicated account): nothing
> to install on the NAS. Approach **B** (Docker server) is deferred; it
> remains relevant for automatic classification and remote access. The
> sections "Design (approach B)", "Docker and architecture" and "Performance"
> below describe B and remain valid for this deferred plan.

## Goal

iPhone photos are backed up automatically to a QNAP, in a single
large folder. Be able to **sort** them (date, place, duplicates) and **tag** them
directly on the NAS, from the Electron app, **locally and remotely**.
The business logic stays in Go. Originals are never modified.

## Facts and decisions

Confirmed by the user (✔) or adopted by default (assumption):

| # | Topic | Decision |
| --- | --- | --- |
| F1 | NAS ✔ | **QNAP TS-251D**: Celeron J4005/J4025, **2 cores / 2 threads**, x86_64, 2 or 4 GB of RAM (8 GB maximum). Container Station (Docker) available but **not installed** ✔; **SSH disabled** ✔ (attack surface) |
| F2 | Storage ✔ | The photos are on a NAS disk, in **a single share with two subfolders** (loose input and sorted library) ✔. The library **already exists and is massive** ✔ |
| F3 | Trigger ✔ | **Manual from the app** first; a "sort automatically" checkbox will come later |
| F4 | Remote access ✔ | **None in place**: v1 works on the local network. Remote access afterwards; comparison of myQNAPcloud Link, Tailscale and NetBird below, recommendation Tailscale integrated into the server (to be confirmed) |
| F5 | Existing tags ✔ | The user uses **QNAP's tagging** (QuMagie). Photosort tags are a complement, not a replacement |
| F6 | Sorting (revised) | **Copy** from the input to the library. The hard link is abandoned: with `fs.protected_hardlinks=1`, a user can only link files they own or on which they have read and write access, which contradicts F8. "Move" is not offered in v1; the disk space of the sorted photos adds up (check before copying) |
| F7 | Volume ✔ | **A few hundred photos** in the input; massive library. The cost to watch is walking the library (deduplication), not the copy |
| F8 | Rights ✔ | **Least privilege**: dedicated QTS user, **read-only** on the loose iPhone folder, **write only** on the destination, no other rights. Access software (local network, Tailscale, NetBird) may have flaws: what the dedicated account (and the container, in route B) can touch never depends on the access mode |

## Approaches studied

| | Principle | Advantages | Limits |
| --- | --- | --- | --- |
| A | The Mac does the processing, NAS mounted over SMB with a dedicated account | Nothing to install on the NAS, neither SSH nor Docker; the app already reads one folder and writes to another; fast AI tags on the Mac; no secret in the app | Data travels over the network (a few hundred photos: irrelevant); walking the massive library must be contained; unusable remotely without a VPN; no automation without the Mac |
| **B** | **Go server in Docker on the NAS**; the Electron app connects to it | Processing next to the data; only thumbnails and progress travel; local and remote; automatable | Requires Container Station (root daemon); server code to write; NAS CPU decisive for AI |
| B' | Variant of B: static Go binary (no CGO, no AI) launched by QTS | No Docker needed | No AI tags on the NAS; irrelevant here, the TS-251D handles Docker |
| C | Scheduled task on the NAS, no UI | Fully automatic | No preview or control |

**Revised recommendation: A' for the v1 on the local network** (A hardened: dedicated account with
minimal rights, prior check of read-only access, scoped deduplication,
atomic copy, system names ignored). **B remains the target** for automatic
classification (the "sort automatically" checkbox) and remote access; C follows from it.

Reasons for the revision: B requires Container Station and a root daemon that
the user does not have, for a few hundred photos; the sidecar protocol
(one JSON request, events carrying the request `id`) nevertheless
carries over unchanged to a TLS connection the day B is built.

## Design (approach B)

### Components

- **`cmd/photosort-server`** (new): same engine as `photosort-sidecar`, exposed
  over TLS with the sidecar's line-by-line JSON protocol. A single Go service,
  in a `linux/amd64` Docker image.
- **Transport**: the Electron app gets a second "backend" in its main
  process (`node:tls`): same requests, same events, only the transport
  changes (local process or TLS connection). No WebSocket.
- **Authentication**: 256-bit token generated on first launch (the server
  keeps only its fingerprint), stored on the Electron side in the system keychain
  (`safeStorage`). TLS mandatory; on the local network, self-signed certificate
  **pinned by fingerprint**, checked before the token is sent.
  The server listens through an interchangeable `net.Listener` interface (local
  network, then Tailscale or NetBird private network, see "Remote access").
- **Paths**: two fixed roots in the container, `/inbox` (read-only) and
  `/library` (write); the server refuses any path outside these roots
  (`..`, links). This validation doubles the system rights, it does not
  replace them.
- **Large files**: only thumbnails (reduced JPEGs) and the weekly planner PDF
  are brought back; originals never leave the NAS.

### Data flow

1. The app connects (address, token); the input and the library are the two
   fixed roots of the server.
2. `plan`: the server reads the EXIF data in place and streams progress and
   drafts (already the current behaviour).
3. `copy`: copy local to the NAS, no network traffic. Input (`:ro`) and
   library are two distinct mounts. In case of an interruption, the operation is
   cancelled with no partial file; re-running the plan resumes the work.
4. `tag`: `.xmp` tags written next to the photos, as today.
5. Weekly planner: the PDF is generated on the NAS then downloaded into the folder
   chosen in the preferences.

### Error handling and security

- Connection lost: operations in progress are cancelled (v1); the app
  reconnects and re-runs the plan. Detached operations and resumption by identifier:
  "remote access" plan.
- Files being synced: ignored as long as their size or modification date
  changes, or their age is below a threshold (60 s).
- No deletion or modification of originals. Writes limited to the
  library (copies, `.xmp` sidecars) and to the container's state folder.
- **The input is never written to**, by three independent locks: QTS rights of the
  dedicated user (F8), Docker `:ro` mount, and a check at server startup,
  which refuses to start if the input is writable.
- Non-root container, no capabilities, read-only root, no `privileged`
  or Docker socket, with access only to the two folders and to its state folder.

### Docker and architecture

Docker does not emulate the processor: the container uses the NAS's CPU and RAM.
Architecture therefore still matters, but the TS-251D is **x86_64**: the
`linux/amd64` image is suitable. What really weighs is the power (2 cores) and the
memory (2 to 4 GB, shared with QTS and QuMagie). The container is limited in
memory and CPU so as not to hinder QuMagie.

### Performance (estimates to be measured)

- On the M2 Mac, AI tagging takes 0.06 to 0.1 s per photo. The 2-core
  Celeron will be noticeably slower (order of magnitude: 10 to 30 times, i.e.
  1 to 3 s per photo, to be confirmed): several hours for tens of
  thousands of photos.
- Sorting (EXIF, duplicates, copies) and metadata tags
  remain light: **enabled by default on the NAS**. AI tags
  (scenes, moment, people) are **optional**, designed to run as a
  background task. QuMagie already provides its own recognition.
- Parallelism bounded to 1 or 2 workers; partial EXIF and HEIC decoding in place,
  at 1/8, as locally.
- We must check that ONNX Runtime runs on this Celeron (no AVX): test
  in step 0.

### Coexistence with QuMagie

- QuMagie must only index the **sorted library**, not the input: the
  photos would exist there twice (the original and its copy), hence duplicates
  in its albums.
- Photosort's `.xmp` files are written next to the photos; QuMagie ignores them,
  its tags remain independent. Displaying Photosort tags in the app
  serves search and the weekly planner.
- The iPhone's automatic backup is never modified: no file
  in the input is written, moved or deleted.

## Step-by-step plan

1. **Local connection, route A'** (plan `2026-11-17-nas-local-connection-plan.md`):
   dedicated QTS account and Advanced Folder Permissions (phase 0b), system names
   ignored, scoped deduplication, atomic copy, prior `nasCheck` check,
   NAS mode in the preferences, acceptance test on the NAS, documentation.
   Tags (metadata and AI) and the weekly planner work unchanged:
   the library is read over SMB, the PDF is written locally.
2. **Docker server (route B, deferred)** (plan `…-nas-docker-server-plan-deferred.md`):
   Go server, token, pinned TLS, fixed roots, `linux/amd64` image and
   hardened `docker-compose.yml`; measurement on the Celeron (plan duration, metadata
   tag, ONNX Runtime). To be resumed for automation and remote
   access, if Container Station is installed.
3. "Sort automatically" checkbox: input monitoring, stable files
   only, resumable (requires processing without the Mac, hence B).
4. Remote access: see below. On route A', the NAS must be reachable
   over SMB from a VPN; **SMB is never exposed to the internet**. With B, the
   server is written around a `net.Listener` interface and the step adds
   the chosen "overlay" implementation. A myQNAPcloud Link **relay test**
   (minimal HTTP container reached via `qlink.to/<id>` from the phone's
   connection sharing) remains possible in parallel: only this test tells whether the
   relay accepts a service other than the QTS apps.

## Remote access (later plan)

User's goal: **as little configuration as possible on the NAS** and
the greatest simplicity of use. No remote access exists today.

### Comparison

| Criterion | myQNAPcloud Link | Tailscale | NetBird |
| --- | --- | --- | --- |
| Principle | QNAP relay `qlink.to/<id>`, QNAP account | Meshed WireGuard private network, coordination by Tailscale Inc. | Same; open-source client, self-hostable control plane |
| On the NAS | QNAP app + QNAP account | Official QPKG app, **or none** if integrated into the server (`tsnet`) | No QPKG; Docker container (`netbirdio/netbird`), **or none** if integrated (`client/embed`) |
| On the internet router | Nothing | Nothing | Nothing |
| On the Mac | Nothing (if the relay is suitable) | Tailscale app + account | NetBird app + account (Google, GitHub... via OIDC) |
| Exposure | **Public URL**: the whole internet reaches the service's page, hence token and TLS mandatory | Private (visible only to the account's devices) | Private |
| Cost | Included with the NAS | Free for personal use (exact limits not verified) | Free: 5 users, 100 machines (verified) |
| Licence | Closed | BSD-3 clients; proprietary coordination (Headscale, community-run, as an alternative) | BSD-3 client; management, signal and relay under AGPLv3 |
| Go integration | — | **`tsnet`**: mature, HTTPS server with automatic certificate | **`client/embed`**: `Dial` and `ListenTCP`, more recent, heavier dependencies (to be measured) |
| TLS | Provided by the relay | Automatic certificate (HTTPS to be enabled once in the Tailscale console) | No automatic certificate; the WireGuard tunnel already encrypts, token kept |
| Control-plane self-hosting | No | No (Headscale unofficial) | Yes, but requires a public VM, a domain and ports 443 and 3478: contrary to the desired simplicity, hence NetBird Cloud |
| Main risk | Relay of a non-QTS service **not verified** | Dependence on a vendor | Younger ecosystem; no official QNAP integration |

QVPN and the myQNAPcloud DDNS (`id.myqnapcloud.com`) are ruled out: they require
opening a port on the router (unusable behind CGNAT, service exposed
directly to the internet).

### Deciding finding: the overlay is built into the Go server

Tailscale (`tsnet`) and NetBird (`client/embed`) both exist as **Go
libraries**: the `photosort-server` server can join the private network **itself**,
with no QNAP app, no additional container, no NAS network configuration.
Setup on the NAS comes down to **a `docker-compose.yml` and an authentication
key** to enter once. Advantages: no exposed port, a single
container, no dependence on the vendor's QPKG.

On the Mac side, the user installs the chosen service's app (v1 of remote
access). A possible evolution (outside v1): integrate the same library into
the Mac's Go sidecar, so that there is **nothing to install** and a pairing
code is enough.

### Recommendation

1. **v1: local network only.** TLS with pinned self-signed certificate, token.
2. **Remote access: Tailscale via `tsnet` in the server.** Reasons:
   most mature library, automatic HTTPS certificate, official fallback via
   QPKG, widely used Mac and mobile apps. NAS configuration is limited to the
   authentication key in the compose file.
3. **NetBird remains an easy swap**: the server only needs a
   `net.Listener`, the client only a `Dialer`. To be preferred if open source of the
   control plane matters more than simplicity; otherwise it is not justified.
4. **myQNAPcloud Link: secondary option**, only if the relay test
   succeeds; its public URL is its main flaw.

Decision to be confirmed by the user before tackling remote access. Points to check at that
time: limits of the Tailscale free plan, key expiry (disable
expiry of the NAS node in the console), size of the Docker image with the
chosen library.

### Remote access security (user's remarks)

- **MFA**: the public URL of myQNAPcloud Link is a flaw, but multi-factor
  authentication on the QNAP account strengthens access; to be weighed against the lack of
  exposure of a private network.
- **Flaws**: Tailscale, NetBird (like any access software) may have some.
  Design invariant: the NAS's security **does not depend** on the access mode.
  A dedicated QTS account (read-only on the input, write only on the
  library, nothing else) and a hardened container (F8) limit the damage if
  the server or the overlay is compromised. The overlay adds no rights.

## Out of scope (YAGNI)

Direct exposure to the internet, multi-user, synchronisation with QuMagie
albums, moving or deleting originals, ARM architecture,
named face recognition.

## Open questions

1. ~~Photo volume~~: a few hundred in the input (F7).
2. ~~Step 0 on the NAS~~: not applicable on route A' (neither SSH nor Container Station).
3. ~~Hard link or copy~~: copy, imposed by the read-only input (F6).
4. Remote access (second plan): on route A', a VPN giving access to the NAS
   (SMB never exposed to the internet); on route B, Tailscale integrated into the server
   (recommended), NetBird (open source), or myQNAPcloud Link (public URL,
   strengthened by the QNAP account's MFA, if the relay test succeeds). The choice
   does not affect the rights of the dedicated account (F8).
5. ~~One or two shares~~: one share, two subfolders; existing and
   massive library (F2). Open: does it follow Photosort's `YYYY/YYYY-MM`
   tree (determines the scoped deduplication)?
6. ~~Container Station, SSH~~: neither (F1).
7. Advanced Folder Permissions (global NAS setting): accepted to separate
   the rights of the two subfolders? Otherwise, the input is protected only by the app.
8. Does QuMagie index the input, the library, or both?
