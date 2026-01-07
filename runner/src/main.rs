mod config;
mod network;
mod oci;
mod server;

use anyhow::Result;
use hyper::server::conn::http1;
use std::env;
use std::sync::Arc;
use tokio::net::TcpListener;
use wasmtime::component::{Component, Linker};
use wasmtime::{Config, Engine};
use wasmtime_wasi_http::bindings::ProxyPre;
use wasmtime_wasi_http::io::TokioIo;

use config::WasiConfig;
use server::ServerState;

#[tokio::main]
async fn main() -> Result<()> {
    // Load WASI configuration from environment
    let wasi_config = WasiConfig::from_env()?;
    
    // Log the loaded configuration for debugging
    println!("Loaded WASI configuration:");
    println!("  Image: {}", wasi_config.image);
    println!("  Args: {:?}", wasi_config.args);
    println!("  Env vars: {} entries", wasi_config.env.len());
    println!("  Volume mounts: {} entries", wasi_config.volume_mounts.len());
    if let Some(memory) = wasi_config.resources.get_memory() {
        println!("  Memory: {}", memory);
    }
    if let Some(cpu) = wasi_config.resources.get_cpu() {
        println!("  CPU: {}", cpu);
    }
    if let Some(network) = &wasi_config.network {
        println!("  Network:");
        println!("    Inherit: {}", network.inherit);
        println!("    Allow IP name lookup: {}", network.allow_ip_name_lookup);
        println!("    TCP bind: {:?}", network.tcp_bind);
        println!("    TCP connect: {:?}", network.tcp_connect);
    } else {
        println!("  Network: disabled");
    }

    // Prepare the `Engine` for Wasmtime
    let mut config = Config::new();
    config.async_support(true);
    
    // Enable fuel consumption if CPU limit/request is configured
    if wasi_config.resources.get_cpu().is_some() {
        config.consume_fuel(true);
    }
    
    let engine = Engine::new(&config)?;

    // Get image name from WASI_CONFIG or fall back to IMAGE env var
    let imgname = if !wasi_config.image.is_empty() {
        wasi_config.image.clone()
    } else {
        env::var("IMAGE")?
    };

    // Fetch and decode the Wasm in OCI image
    println!("Fetching WASM module from: {}", imgname);
    let wasm = oci::fetch_oci_image(&imgname).await?;
    println!("WASM module fetched successfully ({} bytes)", wasm.len());

    // Compile the component on the command line to machine code
    let component = Component::from_binary(&engine, &wasm)?;
    println!("WASM component compiled successfully");

    // Prepare the `ProxyPre` which is a pre-instantiated version of the
    // component that we have. This will make per-request instantiation
    // much quicker.
    let mut linker = Linker::new(&engine);
    wasmtime_wasi::p2::add_to_linker_async(&mut linker)?;
    wasmtime_wasi_http::add_only_http_to_linker_async(&mut linker)?;
    let pre = ProxyPre::new(linker.instantiate_pre(&component)?)?;

    // Prepare our server state and start listening for connections.
    let server = Arc::new(ServerState::new(pre, wasi_config));
    let port = env::var("PORT").unwrap_or("8000".to_string());
    let bind = format!("127.0.0.1:{}", port);
    let listener = TcpListener::bind(&bind).await?;
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
