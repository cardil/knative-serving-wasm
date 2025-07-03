FROM docker.io/library/rust:1 as builder

RUN mkdir /code
WORKDIR /code
COPY . .

RUN cargo build --release

FROM gcr.io/distroless/cc-debian12

COPY --from=builder /code/target/release/runner /usr/bin/wasm-runner

CMD ["/usr/bin/wasm-runner"]
