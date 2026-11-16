#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

case "${1:-}" in
  macos)
    if [[ "$(uname -s)" != Darwin ]]; then
      printf 'Le build macOS doit etre lance sur macOS.\n' >&2
      exit 1
    fi
    for tool in go xcrun codesign; do
      command -v "$tool" >/dev/null || { printf 'Outil requis : %s\n' "$tool" >&2; exit 1; }
    done
    xcrun --find clang >/dev/null
    bundle="$root/dist/macos/Photosort.app"
    mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
    CGO_ENABLED=1 GOOS=darwin GOARCH="$(go env GOHOSTARCH)" \
      go build -trimpath -o "$bundle/Contents/MacOS/photosort-gui" ./cmd/photosort-gui
    cp packaging/macos/Info.plist "$bundle/Contents/Info.plist"
    go run ./packaging/icon "$bundle/Contents/Resources"
    codesign --force --sign - "$bundle"
    codesign --verify --strict "$bundle"
    printf 'Application generee : %s\n' "$bundle"
    ;;
  linux)
    command -v docker >/dev/null || { printf 'Docker est requis pour le build Linux.\n' >&2; exit 1; }
    if ! docker info >/dev/null 2>&1; then
      printf 'Le moteur Docker est inaccessible. Demarrer Docker Desktop puis relancer la tache.\n' >&2
      exit 1
    fi
    go_version="$(awk '$1 == "go" { print $2; exit }' go.mod)"
    docker build --platform linux/amd64 \
      --build-arg "GO_VERSION=$go_version" \
      --file packaging/linux/Dockerfile --target artifact \
      --output "type=local,dest=$root/dist/linux" "$root"
    printf 'Application generee : %s/dist/linux/Photosort-x86_64.AppImage\n' "$root"
    ;;
  *)
    printf 'Usage : bash packaging/build.sh {macos|linux}\n' >&2
    exit 2
    ;;
esac