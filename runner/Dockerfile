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

# Pre-fetch dependency crates into ~/.cargo/registry in a stable layer
# (only invalidated when Cargo.toml/Cargo.lock change). This avoids
# re-downloading crates on every source change without risking stale
# placeholder build artifacts interfering with the real build.
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && \
  echo 'fn main() {}' > src/main.rs && \
  cargo fetch --locked \
  && rm -rf src

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
