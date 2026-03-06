FROM --platform=$BUILDPLATFORM docker.io/library/rust:1 AS builder

# Enable arm64 packages for cross-compilation sysroot
RUN dpkg --add-architecture arm64

# Install cross-compilation toolchains and arm64 OpenSSL sysroot
RUN apt-get update && apt-get install -y --no-install-recommends \
      gcc-aarch64-linux-gnu \
      libc6-dev-arm64-cross \
      libssl-dev:arm64 \
      pkg-config \
    && rm -rf /var/lib/apt/lists/*

# Add Rust targets for cross-compilation
RUN rustup target add aarch64-unknown-linux-gnu

ARG TARGETARCH

RUN mkdir /code
WORKDIR /code

# Pre-fetch dependencies for layer caching: copy manifests only,
# fetch all crates, build a placeholder binary to cache compiled deps,
# then copy real source.
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && echo 'fn main() {}' > src/main.rs && cargo fetch --locked && rm -rf src
RUN mkdir src && echo 'fn main() {}' > src/main.rs && \
    case "${TARGETARCH}" in \
      amd64) \
        cargo build --release 2>/dev/null; true \
        ;; \
      arm64) \
        PKG_CONFIG_ALLOW_CROSS=1 \
        PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig \
        CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER=aarch64-linux-gnu-gcc \
        cargo build --release --target aarch64-unknown-linux-gnu 2>/dev/null; true \
        ;; \
    esac && \
    rm -rf src target

COPY . .

# Build for the target architecture:
#   amd64 -> native build
#   arm64 -> cross-compile with aarch64-linux-gnu-gcc (no QEMU needed)
RUN case "${TARGETARCH}" in \
      amd64) \
        cargo build --release \
        ;; \
      arm64) \
        PKG_CONFIG_ALLOW_CROSS=1 \
        PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig \
        CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER=aarch64-linux-gnu-gcc \
        cargo build --release --target aarch64-unknown-linux-gnu && \
        cp target/aarch64-unknown-linux-gnu/release/runner target/release/runner \
        ;; \
      *) echo "Unsupported TARGETARCH: ${TARGETARCH}" && exit 1 ;; \
    esac

FROM gcr.io/distroless/cc-debian12

COPY --from=builder /code/target/release/runner /usr/bin/wasm-runner

CMD ["/usr/bin/wasm-runner"]
