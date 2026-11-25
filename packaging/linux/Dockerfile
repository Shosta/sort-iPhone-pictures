# syntax=docker/dockerfile:1
ARG GO_VERSION=1.27.4
FROM golang:${GO_VERSION}-bookworm AS build

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl file build-essential pkg-config \
    libgl1-mesa-dev xorg-dev libxkbcommon-dev libayatana-appindicator3-dev \
    && rm -rf /var/lib/apt/lists/*

# Release figee de linuxdeploy (plus de « continuous » mutable) ; l'empreinte est
# verifiee apres telechargement. Pour changer de version, mettre a jour les deux ARG.
ARG LINUXDEPLOY_VERSION=1-alpha-20251107-1
ARG LINUXDEPLOY_SHA256=c20cd71e3a4e3b80c3483cef793cda3f4e990aca14014d23c544ca3ce1270b4d
ARG LINUXDEPLOY_URL=https://github.com/linuxdeploy/linuxdeploy/releases/download/${LINUXDEPLOY_VERSION}/linuxdeploy-x86_64.AppImage
RUN curl --fail --location --retry 3 "$LINUXDEPLOY_URL" -o /tmp/linuxdeploy.AppImage \
    && echo "${LINUXDEPLOY_SHA256}  /tmp/linuxdeploy.AppImage" | sha256sum --check --strict - \
    && chmod +x /tmp/linuxdeploy.AppImage \
    && cd /opt \
    && /tmp/linuxdeploy.AppImage --appimage-extract \
    && mv squashfs-root linuxdeploy \
    && rm /tmp/linuxdeploy.AppImage

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY sorter/ ./sorter/
COPY packaging/ ./packaging/

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath -o /tmp/photosort-gui ./cmd/photosort-gui \
    && go run ./packaging/icon /tmp/icons

RUN mkdir -p /out \
    && ARCH=x86_64 APPIMAGE_EXTRACT_AND_RUN=1 OUTPUT=/out/Photosort-x86_64.AppImage \
    /opt/linuxdeploy/AppRun --appdir /tmp/Photosort.AppDir \
    --executable /tmp/photosort-gui \
    --desktop-file packaging/linux/photosort.desktop \
    --icon-file /tmp/icons/photosort.png --output appimage \
    && chmod +x /out/Photosort-x86_64.AppImage \
    && /out/Photosort-x86_64.AppImage --appimage-extract >/dev/null \
    && test -x squashfs-root/AppRun \
    && test -x squashfs-root/usr/bin/photosort-gui

FROM scratch AS artifact
COPY --from=build /out/ /