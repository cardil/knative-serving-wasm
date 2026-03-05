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

use serde::Deserialize;
use std::collections::HashMap;

/// WASI configuration passed from the controller via WASI_CONFIG environment variable.
/// This mirrors the WasmModuleSpec from the Go controller.
#[derive(Debug, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct WasiConfig {
    /// OCI image containing the WASM module
    #[serde(default)]
    pub image: String,

    /// Command line arguments to pass to the WASM module
    #[serde(default)]
    pub args: Vec<String>,

    /// Environment variables to set in the WASM module.
    /// Serialized as a JSON object (map) by the controller: {"KEY": "VALUE"}.
    #[serde(default)]
    pub env: HashMap<String, String>,

    /// Directory mounts to expose as WASI preopened directories.
    /// Serialized as "dirs" by the controller.
    #[serde(default)]
    pub dirs: Vec<DirConfig>,

    /// Resource requirements (memory, CPU limits)
    #[serde(default)]
    pub resources: ResourceRequirements,

    /// Network access configuration
    pub network: Option<NetworkSpec>,
}

/// Directory mount configuration as produced by the controller.
#[derive(Debug, Deserialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct DirConfig {
    pub host_path: String,
    pub guest_path: String,
    #[serde(default)]
    pub read_only: bool,
}

/// Resource requirements for the WASM module
#[derive(Debug, Deserialize, Default)]
pub struct ResourceRequirements {
    #[serde(default)]
    pub limits: HashMap<String, String>,
    #[serde(default)]
    pub requests: HashMap<String, String>,
}

impl ResourceRequirements {
    /// Get memory limit, falling back to request if limit is not set
    pub fn get_memory(&self) -> Option<&String> {
        self.limits.get("memory").or_else(|| self.requests.get("memory"))
    }
    
    /// Get CPU limit, falling back to request if limit is not set
    pub fn get_cpu(&self) -> Option<&String> {
        self.limits.get("cpu").or_else(|| self.requests.get("cpu"))
    }
}

/// Network access configuration for WASI sockets
#[derive(Debug, Deserialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct NetworkSpec {
    /// Inherit the host's full network stack
    #[serde(default)]
    pub inherit: bool,
    
    /// Enable DNS resolution (defaults to true when network is specified)
    #[serde(default = "default_true")]
    pub allow_ip_name_lookup: bool,
    
    /// Address patterns allowed for TCP bind
    #[serde(default)]
    pub tcp_bind: Vec<String>,
    
    /// Address patterns allowed for TCP connect
    #[serde(default)]
    pub tcp_connect: Vec<String>,
    
    /// Address patterns allowed for UDP bind
    #[serde(default)]
    pub udp_bind: Vec<String>,
    
    /// Address patterns allowed for UDP connect
    #[serde(default)]
    pub udp_connect: Vec<String>,
    
    /// Address patterns allowed for UDP outgoing datagrams
    #[serde(default)]
    pub udp_outgoing: Vec<String>,
}

fn default_true() -> bool {
    true
}

impl WasiConfig {
    /// Load WASI configuration from the WASI_CONFIG environment variable
    pub fn from_env() -> anyhow::Result<Self> {
        match std::env::var("WASI_CONFIG") {
            Ok(json) => {
                let config: WasiConfig = serde_json::from_str(&json)?;
                Ok(config)
            }
            Err(_) => {
                // No WASI_CONFIG provided, use defaults
                Ok(WasiConfig::default())
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Contract test: the runner must parse the exact JSON produced by the controller.
    /// The golden file is the single source of truth for the wire format.
    /// Both sides must agree: update the golden file and this test together.
    #[test]
    fn test_parse_golden_wasi_config() {
        let golden = std::fs::read_to_string(
            "../pkg/reconciler/wasmmodule/testdata/wasi_config.golden.json",
        )
        .expect("golden file must exist");

        let config: WasiConfig = serde_json::from_str(&golden)
            .expect("golden JSON must parse into WasiConfig");

        // env is a map in the wire format
        assert_eq!(config.env.get("GREETING"), Some(&"hello".to_string()));
        assert_eq!(config.env.get("PORT"), Some(&"8080".to_string()));

        // dirs (not volumeMounts) with hostPath/guestPath
        assert_eq!(config.dirs.len(), 2);
        assert_eq!(config.dirs[0].host_path, "/mnt/data");
        assert_eq!(config.dirs[0].guest_path, "/mnt/data");
        assert!(!config.dirs[0].read_only);
        assert_eq!(config.dirs[1].host_path, "/mnt/ro");
        assert!(config.dirs[1].read_only);

        // args
        assert_eq!(config.args, vec!["--verbose"]);

        // network
        let net = config.network.as_ref().expect("network must be present");
        assert!(net.allow_ip_name_lookup);
        assert_eq!(net.tcp_connect, vec!["example.com:443"]);
    }
}
