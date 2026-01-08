use anyhow::{Error, Result};
use oci_distribution::secrets::RegistryAuth;
use oci_distribution::{Client, Reference};

const OCI_WASM_MEDIA_TYPE: &str = "application/wasm";
const WASM_MEDIA_TYPE: &str = "application/vnd.wasm.content.layer.v1+wasm";
const WASM_MEDIA_TYPE_LEGACY: &str = "application/vnd.module.wasm.content.layer.v1+wasm";

fn bad_num_of_layers_err() -> Error {
    Error::msg("expected to have one layer")
}

/// Fetch a WASM module from an OCI registry.
///
/// # Arguments
/// * `imgname` - The OCI image reference (e.g., "ghcr.io/example/module:latest")
///              Can also include the "oci://" prefix which will be stripped.
///
/// # Returns
/// The WASM module binary data
pub async fn fetch_oci_image(imgname: &str) -> Result<Vec<u8>> {
    let oci = Client::default();
    // Strip the oci:// prefix if present (used by Knative/WASI conventions)
    let imgname = imgname.strip_prefix("oci://").unwrap_or(imgname);
    let imgref: Reference = imgname.parse()?;
    // TODO: use a real auth, taken from the K8s cluster
    let imgauth = &RegistryAuth::Anonymous;
    let accepted_media_types = Vec::from([
        OCI_WASM_MEDIA_TYPE,
        WASM_MEDIA_TYPE,
        WASM_MEDIA_TYPE_LEGACY,
    ]);
    let image = oci.pull(&imgref, imgauth, accepted_media_types).await?;
    if image.layers.len() != 1 {
        return Err(bad_num_of_layers_err().context(format!(
            "expected to have one layer, got {}",
            image.layers.len()
        )));
    }
    let wasm = image.layers.first().ok_or(bad_num_of_layers_err())?;

    Ok(wasm.data.clone())
}
