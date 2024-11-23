use anyhow::{Error, Result};
use oci_distribution::secrets::RegistryAuth;
use oci_distribution::{Client, Reference};
use wasmtime::{Engine, Linker, Module, Store};
use wasmtime_wasi_http::proxy::exports::wasi;
use wasmtime_wasi_http::WasiHttpCtx;

#[tokio::main]
async fn main() -> Result<()> {
    // Modules can be compiled through either the text or binary format
    let engine = Engine::default();

    let wasm = fetch_oci_image("docker.io/library/hello-world:latest").await?;

    let module = Module::new(&engine, &wasm)?;

    // Create a `Linker` which will be later used to instantiate this module.
    // Host functionality is defined by name within the `Linker`.
    let mut linker = Linker::new(&engine);

    WasiHttpCtx::add_to_linker(&mut linker)?;

    // All wasm objects operate within the context of a "store". Each
    // `Store` has a type parameter to store host-specific data, which in
    // this case we're using `4` for.
    let mut store = Store::new(&engine, 4);
    let instance = linker.instantiate(&mut store, &module)?;
    let guest = instance.get_export(&mut store, "Guest::handle")?;

    let gg = wasi::http::incoming_handler::Guest::new(&guest)?;


    println!("Hello, world!");

    Ok(())
}

const BAD_NUM_OF_LAYERS_ERR: Error = Error::msg("expected to have one layer");
const WASM_MEDIA_TYPE: &str = "application/vnd.wasm.content.layer.v1+wasm";
const WASM_MEDIA_TYPE_LEGACY: &str = "application/vnd.module.wasm.content.layer.v1+wasm";

async fn fetch_oci_image(imgname: &str) -> Result<Vec<u8>> {
    let mut oci = Client::default();
    let imgref: Reference = imgname.parse()?;
    // TODO: use a real auth, taken from the K8s cluster
    let imgauth = &RegistryAuth::Anonymous;
    let accpected_media_types = Vec::from([WASM_MEDIA_TYPE, WASM_MEDIA_TYPE_LEGACY]);
    let image = oci.pull(&imgref, imgauth, accpected_media_types).await?;
    if image.layers.len() != 1 {
        return Err(BAD_NUM_OF_LAYERS_ERR.context(format!(
            "expected to have one layer, got {}",
            image.layers.len()
        )));
    }
    let wasm = image.layers.first().ok_or(BAD_NUM_OF_LAYERS_ERR)?;

    Ok(wasm.data.clone())
}
