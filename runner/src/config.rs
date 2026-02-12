// Copyright 2025 The Knative Authors
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
    
    /// Environment variables to set in the WASM module
    #[serde(default)]
    pub env: Vec<EnvVar>,
    
    /// Volume mounts to expose as WASI preopened directories
    #[serde(default)]
    pub volume_mounts: Vec<VolumeMount>,
    
    /// Resource requirements (memory, CPU limits)
    #[serde(default)]
    pub resources: ResourceRequirements,
    
    /// Network access configuration
    pub network: Option<NetworkSpec>,
}

/// Environment variable configuration
#[derive(Debug, Deserialize, Clone)]
pub struct EnvVar {
    pub name: String,
    #[serde(default)]
    pub value: String,
}

/// Volume mount configuration
#[derive(Debug, Deserialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct VolumeMount {
    pub name: String,
    pub mount_path: String,
    #[serde(default)]
    pub read_only: bool,
    #[serde(default)]
    pub sub_path: String,
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
