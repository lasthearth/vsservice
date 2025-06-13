# syntax=docker/dockerfile:1

# Comments are provided throughout this file to help you get started.
# If you need more help, visit the Dockerfile reference guide at
# https://docs.docker.com/go/dockerfile-reference/

# Want to help us make this template better? Share your feedback here: https://forms.gle/ybq9Krt8jtBL3iCk7

################################################################################
# Create a stage for building the application.
ARG GO_VERSION=1.24
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
WORKDIR /src

ARG LIBVIPS_VERSION=8.15.5
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
    libglib2.0-dev \
    libjpeg62-turbo-dev \
    libtiff5-dev \
    meson \
    ninja-build \
    libwebp-dev \
    libarchive-dev \
    libexpat1-dev && \
    rm -rf /var/lib/apt/lists/*

RUN wget https://github.com/libvips/libvips/releases/download/v${LIBVIPS_VERSION}/vips-${LIBVIPS_VERSION}.tar.xz && \
    tar -xf vips-${LIBVIPS_VERSION}.tar.xz && \
    cd vips-${LIBVIPS_VERSION} && \
    meson setup build --prefix /usr/local --buildtype=release && \
    meson compile -C build && \
    meson install -C build && \
    ldconfig && \
    cd .. && rm -rf vips-${LIBVIPS_VERSION}*

ARG TARGETARCH

# Build the application.
# Leverage a cache mount to /go/pkg/mod/ to speed up subsequent builds.
# Leverage a bind mount to the current directory to avoid having to copy the
# source code into the container.
RUN --mount=type=bind,source=./${PROJECT_PATH}/,target=. \
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
FROM debian:bookworm-slim AS final

COPY --from=build /usr/local/lib /usr/local/lib

RUN DEBIAN_FRONTEND=noninteractive \
    apt-get update && \
    apt-get install --no-install-recommends -y \
    ca-certificates \
    tzdata \
    libglib2.0-0 \
    libjpeg62-turbo \
    libpng16-16 \
    libtiff6 \
    libwebp7 \
    libwebpmux3 \
    libwebpdemux2 \
    libarchive13 \
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
