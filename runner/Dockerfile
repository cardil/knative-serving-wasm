FROM docker.io/library/rust:1 AS builder

# Install cross-compilation toolchains for supported non-native architectures
RUN apt-get update && apt-get install -y --no-install-recommends \
      gcc-aarch64-linux-gnu \
      libc6-dev-arm64-cross && \
    rm -rf /var/lib/apt/lists/*

# Add Rust targets for cross-compilation
RUN rustup target add aarch64-unknown-linux-gnu

ARG TARGETARCH

RUN mkdir /code
WORKDIR /code
COPY . .

# Build for the target architecture:
#   amd64 -> native build
#   arm64 -> cross-compile with aarch64-linux-gnu-gcc (no QEMU needed)
RUN case "${TARGETARCH}" in \
      amd64) \
        cargo build --release \
        ;; \
      arm64) \
        CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER=aarch64-linux-gnu-gcc \
        cargo build --release --target aarch64-unknown-linux-gnu && \
        cp target/aarch64-unknown-linux-gnu/release/runner target/release/runner \
        ;; \
      *) echo "Unsupported TARGETARCH: ${TARGETARCH}" && exit 1 ;; \
    esac

FROM gcr.io/distroless/cc-debian12

COPY --from=builder /code/target/release/runner /usr/bin/wasm-runner

CMD ["/usr/bin/wasm-runner"]
