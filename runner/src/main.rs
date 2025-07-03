use std::env;
use anyhow::{bail, Error, Result};
use hyper::server::conn::http1;
use oci_distribution::secrets::RegistryAuth;
use oci_distribution::{Client, Reference};
use std::sync::Arc;
use tokio::net::TcpListener;
use wasmtime::component::{Component, Linker, ResourceTable};
use wasmtime::{Config, Engine, Store};
use wasmtime_wasi::p2::{IoView, WasiCtx, WasiCtxBuilder, WasiView};
use wasmtime_wasi_http::bindings::http::types::Scheme;
use wasmtime_wasi_http::bindings::ProxyPre;
use wasmtime_wasi_http::body::HyperOutgoingBody;
use wasmtime_wasi_http::io::TokioIo;
use wasmtime_wasi_http::{WasiHttpCtx, WasiHttpView};

#[tokio::main]
async fn main() -> Result<()> {
    // Prepare the `Engine` for Wasmtime
    let mut config = Config::new();
    config.async_support(true);
    let engine = Engine::new(&config)?;

    let imgname = env::var("IMAGE")?;

    // Fetch and decode the Wasm in OCI image
    let wasm = fetch_oci_image(imgname.as_str()).await?;

    // Compile the component on the command line to machine code
    let component = Component::from_binary(&engine, &wasm)?;

    // Prepare the `ProxyPre` which is a pre-instantiated version of the
    // component that we have. This will make per-request instantiation
    // much quicker.
    let mut linker = Linker::new(&engine);
    wasmtime_wasi::p2::add_to_linker_async(&mut linker)?;
    wasmtime_wasi_http::add_only_http_to_linker_async(&mut linker)?;
    let pre = ProxyPre::new(linker.instantiate_pre(&component)?)?;

    // Prepare our server state and start listening for connections.
    let server = Arc::new(KnativeGuestServer { pre });
    let port = env::var("PORT").unwrap_or("8000".to_string());
    let bind = format!("127.0.0.1:{}", port);
    let listener = TcpListener::bind(bind).await?;
    println!("Listening on {}", listener.local_addr()?);

    loop {
        // Accept a TCP connection and serve all of its requests in a separate
        // tokio task. Note that for now this only works with HTTP/1.1.
        let (client, addr) = listener.accept().await?;
        println!("serving new client from {addr}");

        let server = server.clone();
        tokio::task::spawn(async move {
            if let Err(e) = http1::Builder::new()
                .keep_alive(true)
                .serve_connection(
                    TokioIo::new(client),
                    hyper::service::service_fn(move |req| {
                        let server = server.clone();
                        async move { server.handle_request(req).await }
                    }),
                )
                .await
            {
                eprintln!("error serving client[{addr}]: {e:?}");
            }
        });
    }
}

struct KnativeGuestServer {
    pre: ProxyPre<MyClientState>,
}

impl KnativeGuestServer {
    async fn handle_request(
        &self,
        req: hyper::Request<hyper::body::Incoming>,
    ) -> Result<hyper::Response<HyperOutgoingBody>> {
        // Create per-http-request state within a `Store` and prepare the
        // initial resources  passed to the `handle` function.
        let mut store = Store::new(
            self.pre.engine(),
            MyClientState {
                table: ResourceTable::new(),
                wasi: WasiCtxBuilder::new().inherit_stdio().build(),
                http: WasiHttpCtx::new(),
            },
        );
        let (sender, receiver) = tokio::sync::oneshot::channel();
        let req = store.data_mut().new_incoming_request(Scheme::Http, req)?;
        let out = store.data_mut().new_response_outparam(sender)?;
        let pre = self.pre.clone();

        // Run the http request itself in a separate task so the task can
        // optionally continue to execute beyond after the initial
        // headers/response code are sent.
        let task = tokio::task::spawn(async move {
            let proxy = pre.instantiate_async(&mut store).await?;

            if let Err(e) = proxy
                .wasi_http_incoming_handler()
                .call_handle(store, req, out)
                .await
            {
                return Err(e);
            }

            Ok(())
        });

        match receiver.await {
            // If the client calls `response-outparam::set` then one of these
            // methods will be called.
            Ok(Ok(resp)) => Ok(resp),
            Ok(Err(e)) => Err(e.into()),

            // Otherwise the `sender` will get dropped along with the `Store`
            // meaning that the oneshot will get disconnected and here we can
            // inspect the `task` result to see what happened
            Err(_) => {
                let e = match task.await {
                    Ok(Ok(())) => {
                        bail!("guest never invoked `response-outparam::set` method")
                    }
                    Ok(Err(e)) => e,
                    Err(e) => e.into(),
                };
                Err(e.context("guest never invoked `response-outparam::set` method"))
            }
        }
    }
}

struct MyClientState {
    wasi: WasiCtx,
    http: WasiHttpCtx,
    table: ResourceTable,
}
impl IoView for MyClientState {
    fn table(&mut self) -> &mut ResourceTable {
        &mut self.table
    }
}
impl WasiView for MyClientState {
    fn ctx(&mut self) -> &mut WasiCtx {
        &mut self.wasi
    }
}

impl WasiHttpView for MyClientState {
    fn ctx(&mut self) -> &mut WasiHttpCtx {
        &mut self.http
    }
}


const OCI_WASM_MEDIA_TYPE: &str = "application/wasm";
const WASM_MEDIA_TYPE: &str = "application/vnd.wasm.content.layer.v1+wasm";
const WASM_MEDIA_TYPE_LEGACY: &str = "application/vnd.module.wasm.content.layer.v1+wasm";

fn bad_num_of_layers_err() -> Error {
    Error::msg("expected to have one layer")
}

async fn fetch_oci_image(imgname: &str) -> Result<Vec<u8>> {
    let oci = Client::default();
    let imgref: Reference = imgname.parse()?;
    // TODO: use a real auth, taken from the K8s cluster
    let imgauth = &RegistryAuth::Anonymous;
    let accpected_media_types = Vec::from([
        OCI_WASM_MEDIA_TYPE,
        WASM_MEDIA_TYPE,
        WASM_MEDIA_TYPE_LEGACY,
    ]);
    let image = oci.pull(&imgref, imgauth, accpected_media_types).await?;
    if image.layers.len() != 1 {
        return Err(bad_num_of_layers_err().context(format!(
            "expected to have one layer, got {}",
            image.layers.len()
        )));
    }
    let wasm = image.layers.first().ok_or(bad_num_of_layers_err())?;

    Ok(wasm.data.clone())
}
