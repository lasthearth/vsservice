# syntax=docker/dockerfile:1
# TODO(supply-chain): pin the BuildKit frontend by digest. `docker/dockerfile:1` is a mutable
# tag, so the frontend that parses this file is not bound to reviewed content.
# Owner command (index digest):
#   docker buildx imagetools inspect --format '{{.Manifest.Digest}}' docker/dockerfile:1
# Owner command (per-platform digests):
#   docker manifest inspect -v docker/dockerfile:1 | jq -r '.[].Descriptor.digest'
# Then replace line 1 with: # syntax=docker/dockerfile:1@sha256:<that-digest>
# Keep the `# syntax=` directive on the first line of this file or Docker reads it as a comment.

# Comments are provided throughout this file to help you get started.
# If you need more help, visit the Dockerfile reference guide at
# https://docs.docker.com/go/dockerfile-reference/

# Want to help us make this template better? Share your feedback here: https://forms.gle/ybq9Krt8jtBL3iCk7

################################################################################
# Create a stage for building the application.
ARG GO_VERSION=1.26
# TODO(supply-chain): pin the build base image by digest. golang:${GO_VERSION} is a mutable tag.
# Owner command (index digest):
#   docker buildx imagetools inspect --format '{{.Manifest.Digest}}' golang:1.26
# Owner command (per-platform digests):
#   docker manifest inspect -v golang:1.26 | jq -r '.[].Descriptor.digest'
# Then use: FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}@sha256:<that-digest> AS build
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
WORKDIR /src

ARG LIBVIPS_VERSION=8.15.5
# SHA-256 of the reviewed libvips release asset vips-${LIBVIPS_VERSION}.tar.xz. Change this value
# only together with LIBVIPS_VERSION, and treat both as reviewed inputs.
# Verified 2026-11-10 from two sources: the asset bytes
# (curl -fsSL .../v8.15.5/vips-8.15.5.tar.xz | shasum -a 256) and the checksum
# file the release publishes (vips-8.15.5.tar.xz.sha256sum, 18652672 bytes).
ARG LIBVIPS_SHA256=2046d2001e60955dc6a85b82afbd6cadc8c34e10ba2f6e5ea09f03fc69eb2603
ARG PROJECT_PATH=./

LABEL org.opencontainers.image.source="https://github.com/lasthearth/vsservice"

# Download dependencies as a separate step to take advantage of Docker's caching.
# Leverage a cache mount to /go/pkg/mod/ to speed up subsequent builds.
# Leverage bind mounts to go.sum and go.mod to avoid having to copy them into
# the container.
RUN --mount=type=bind,source=${PROJECT_PATH}/go.sum,target=go.sum \
    --mount=type=bind,source=${PROJECT_PATH}/go.mod,target=go.mod \
    go mod download -x

RUN DEBIAN_FRONTEND=noninteractive \
    apt-get update && apt-get install -y \
    build-essential \
    pkg-config \
    libc6-dev \
    libglib2.0-dev \
    libjpeg62-turbo-dev \
    libpng-dev \
    libtiff5-dev \
    meson \
    ninja-build \
    libwebp-dev \
    libarchive-dev \
    libexpat1-dev && \
    rm -rf /var/lib/apt/lists/*

# Download, verify, then unpack — in that order and in one RUN step, so a checksum mismatch (or the
# fail-closed placeholder above) stops the build before any libvips code is compiled or installed.
RUN wget -O vips.tar.xz https://github.com/libvips/libvips/releases/download/v${LIBVIPS_VERSION}/vips-${LIBVIPS_VERSION}.tar.xz && \
    echo "${LIBVIPS_SHA256}  vips.tar.xz" | sha256sum -c - && \
    tar -xf vips.tar.xz && \
    cd vips-${LIBVIPS_VERSION} && \
    meson setup build --prefix /usr/local --buildtype=release && \
    meson compile -C build && \
    meson install -C build && \
    ldconfig && \
    cd .. && rm -rf vips-${LIBVIPS_VERSION}* vips.tar.xz

ARG TARGETARCH

# Regenerate protobuf stubs and build the application. The generated `gen/` is
# gitignored, so it must be produced here. buf and the OpenAPI plugin are
# pinned; the Go codegen plugins are the module's `go tool` entries (see
# go.mod), invoked via buf.gen.docker.yaml. docs/v1/openapi.yaml is committed
# and NOT regenerated here — it is the reviewed REST contract.
ARG BUF_VERSION=1.50.0
RUN --mount=type=cache,target=/go/pkg/mod/ \
    go install github.com/bufbuild/buf/cmd/buf@v${BUF_VERSION}

# Copy the source (bind mount is read-only, but codegen writes into the tree).
COPY ${PROJECT_PATH}/ .

RUN --mount=type=cache,target=/go/pkg/mod/ \
    buf generate --template buf.gen.docker.yaml

RUN --mount=type=cache,target=/go/pkg/mod/ \
    CGO_ENABLED=1 GOARCH=$TARGETARCH go build -o /bin/vsservice ./main.go

################################################################################
# Create a new stage for running the application that contains the minimal
# runtime dependencies for the application. This often uses a different base
# image from the build stage where the necessary files are copied from the build
# stage.
#
# The example below uses the alpine image as the foundation for running the app.
# By specifying the "latest" tag, it will also use whatever happens to be the
# most recent version of that image when you build your Dockerfile. If
# reproducability is important, consider using a versioned tag
# (e.g., alpine:3.17.2) or SHA (e.g., alpine@sha256:c41ab5c992deb4fe7e5da09f67a8804a46bd0592bfdf0b1847dde0e0889d2bff).
# TODO(supply-chain): pin the runtime base image by digest. debian:trixie-slim is a mutable tag,
# and this stage is what CI publishes to ghcr.io.
# Owner command (index digest):
#   docker buildx imagetools inspect --format '{{.Manifest.Digest}}' debian:trixie-slim
# Owner command (per-platform digests):
#   docker manifest inspect -v debian:trixie-slim | jq -r '.[].Descriptor.digest'
# Then use: FROM debian:trixie-slim@sha256:<that-digest> AS final
FROM debian:trixie-slim AS final

COPY --from=build /usr/local/lib /usr/local/lib

RUN DEBIAN_FRONTEND=noninteractive \
    apt-get update && \
    apt-get install --no-install-recommends -y \
    ca-certificates \
    tzdata \
    libc6-dev \
    libglib2.0-dev \
    libjpeg62-turbo \
    libpng16-16 \
    libtiff6 \
    libwebp7 \
    libwebpmux3 \
    libwebpdemux2 \
    libarchive13 \
    adduser \
    libexpat1 && \
    apt-get autoremove -y && \
    apt-get autoclean && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    update-ca-certificates

ARG UID=10001
RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    appuser
USER appuser

ENV LD_LIBRARY_PATH="/usr/local/lib"
# Copy the executable from the "build" stage.
COPY --from=build /bin/vsservice /bin/

# Expose the port that the application listens on.
EXPOSE 50051
EXPOSE 6969

# What the container should run when it is started.
ENTRYPOINT [ "/bin/vsservice" ]
