# Remote sort to a NAS (rsync)

The Electron app can send the sorted result straight to a NAS (QNAP, etc.) through rsync, with no intermediate copy. Open "Destination distante (NAS)" (remote destination) and tick the NAS upload option. Fields:

- **Protocol**: `daemon` (the NAS rsync server, port 873, password) or `SSH`.
- **Host, port, user**: the NAS address and the rsync account.
- **Shared folder of the NAS (rsync module)**: the *share name* defined on the NAS (QNAP: Network Services > Rsync Server). It is not a path: it is the entry point, which points to a real folder on the NAS (e.g. `Photos`). Left empty, the "Browse…" button lists the available modules.
- **Folder to store the photos in, inside that share** (optional): a subfolder of the library, e.g. `Library`. Photos arrive in `module/folder/2026/2026-10/...`. Empty = root of the module. With SSH, this is the full path on the NAS.
- **Browse…**: navigates the NAS folders level by level; "Choose this folder" fills the field.
- **Password**: typed each time, or remembered in the system keychain.
- **Verify integrity (checksum)** and **Do not resend photos already present** (by path, name+size+date, or SHA-256 content).

QNAP tip: create a module that points only to the photo library folder, not to a whole volume. Photosort only writes into the chosen folder and never deletes anything (no `--delete`, no `--remove-source-files`). Use `Test connection` then `Simulate upload` to check before sending.

For the full QNAP setup (accounts, permissions, Tailscale) see [nas-qnap.md](nas-qnap.md).
