// Copyright 2026 The Knative Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

use anyhow::{Error, Result};
use oci_distribution::client::{ClientConfig, ClientProtocol};
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
/// * `insecure_registries` - List of registry hostnames (host[:port]) that should
///                           be accessed over plain HTTP instead of HTTPS.
///
/// # Returns
/// The WASM module binary data
pub async fn fetch_oci_image(imgname: &str, insecure_registries: &[String]) -> Result<Vec<u8>> {
    let protocol = if insecure_registries.is_empty() {
        ClientProtocol::Https
    } else {
        ClientProtocol::HttpsExcept(insecure_registries.to_vec())
    };
    let config = ClientConfig {
        protocol,
        ..ClientConfig::default()
    };
    let oci = Client::new(config);
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_client_protocol_empty_list_uses_https() {
        // When insecure_registries is empty, we should use ClientProtocol::Https
        let insecure: Vec<String> = vec![];
        let protocol = if insecure.is_empty() {
            ClientProtocol::Https
        } else {
            ClientProtocol::HttpsExcept(insecure.clone())
        };
        // Verify it's the Https variant by checking it's not HttpsExcept
        assert!(matches!(protocol, ClientProtocol::Https));
    }

    #[test]
    fn test_client_protocol_single_entry_uses_https_except() {
        let insecure = vec!["registry.local:5000".to_string()];
        let protocol = if insecure.is_empty() {
            ClientProtocol::Https
        } else {
            ClientProtocol::HttpsExcept(insecure.clone())
        };
        match protocol {
            ClientProtocol::HttpsExcept(entries) => {
                assert_eq!(entries, vec!["registry.local:5000"]);
            }
            _ => panic!("expected HttpsExcept variant"),
        }
    }

    #[test]
    fn test_client_protocol_multiple_entries_uses_https_except() {
        let insecure = vec![
            "registry.local:5000".to_string(),
            "my-registry.internal:5000".to_string(),
        ];
        let protocol = if insecure.is_empty() {
            ClientProtocol::Https
        } else {
            ClientProtocol::HttpsExcept(insecure.clone())
        };
        match protocol {
            ClientProtocol::HttpsExcept(entries) => {
                assert_eq!(entries.len(), 2);
                assert!(entries.contains(&"registry.local:5000".to_string()));
                assert!(entries.contains(&"my-registry.internal:5000".to_string()));
            }
            _ => panic!("expected HttpsExcept variant"),
        }
    }
}
