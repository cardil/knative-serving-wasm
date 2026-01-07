use anyhow::{bail, Result};
use std::sync::Arc;
use wasmtime::component::ResourceTable;
use wasmtime::{ResourceLimiter, Store, StoreLimits, StoreLimitsBuilder};
use wasmtime_wasi::p2::{IoView, WasiCtx, WasiCtxBuilder, WasiView};
use wasmtime_wasi_http::bindings::http::types::Scheme;
use wasmtime_wasi_http::bindings::ProxyPre;
use wasmtime_wasi_http::body::HyperOutgoingBody;
use wasmtime_wasi_http::{WasiHttpCtx, WasiHttpView};

use crate::config::WasiConfig;
use crate::network;

/// Server state shared across all requests
pub struct ServerState {
    pub pre: ProxyPre<ClientState>,
    pub wasi_config: Arc<WasiConfig>,
}

impl ServerState {
    pub fn new(pre: ProxyPre<ClientState>, wasi_config: WasiConfig) -> Self {
        Self {
            pre,
            wasi_config: Arc::new(wasi_config),
        }
    }

    /// Handle an incoming HTTP request by instantiating the WASM module
    pub async fn handle_request(
        &self,
        req: hyper::Request<hyper::body::Incoming>,
    ) -> Result<hyper::Response<HyperOutgoingBody>> {
        // Create per-http-request state within a `Store` and prepare the
        // initial resources passed to the `handle` function.
        let limits = build_store_limits(&self.wasi_config);
        let mut store = Store::new(
            self.pre.engine(),
            ClientState {
                table: ResourceTable::new(),
                wasi: build_wasi_ctx(&self.wasi_config)?,
                http: WasiHttpCtx::new(),
                limits,
            },
        );
        
        // Set fuel if CPU limit is configured
        if let Some(fuel) = get_fuel_limit(&self.wasi_config) {
            store.set_fuel(fuel)?;
        }
        
        store.limiter(|state| &mut state.limits);
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

/// Per-request client state
pub struct ClientState {
    pub wasi: WasiCtx,
    pub http: WasiHttpCtx,
    pub table: ResourceTable,
    pub limits: StoreLimits,
}

impl IoView for ClientState {
    fn table(&mut self) -> &mut ResourceTable {
        &mut self.table
    }
}

impl WasiView for ClientState {
    fn ctx(&mut self) -> &mut WasiCtx {
        &mut self.wasi
    }
}

impl WasiHttpView for ClientState {
    fn ctx(&mut self) -> &mut WasiHttpCtx {
        &mut self.http
    }
}

impl ResourceLimiter for ClientState {
    fn memory_growing(
        &mut self,
        current: usize,
        desired: usize,
        maximum: Option<usize>,
    ) -> anyhow::Result<bool> {
        self.limits.memory_growing(current, desired, maximum)
    }

    fn table_growing(
        &mut self,
        current: usize,
        desired: usize,
        maximum: Option<usize>,
    ) -> anyhow::Result<bool> {
        self.limits.table_growing(current, desired, maximum)
    }
}

/// Build a WasiCtx from the WASI configuration.
/// This applies all the configuration options from the WasmModule spec.
fn build_wasi_ctx(config: &WasiConfig) -> Result<WasiCtx> {
    let mut builder = WasiCtxBuilder::new();
    
    // Always inherit stdio (as per design)
    builder.inherit_stdio();
    
    // Add command line arguments
    if !config.args.is_empty() {
        builder.args(&config.args);
    }
    
    // Add environment variables
    for env_var in &config.env {
        builder.env(&env_var.name, &env_var.value);
    }
    
    // Add preopened directories from volume mounts
    for mount in &config.volume_mounts {
        use std::path::PathBuf;
        use wasmtime_wasi::{DirPerms, FilePerms};
        
        // Build the host path, applying subPath if specified
        let host_path: PathBuf = if mount.sub_path.is_empty() {
            PathBuf::from(&mount.mount_path)
        } else {
            PathBuf::from(&mount.mount_path).join(&mount.sub_path)
        };
        
        let guest_path = &mount.mount_path;
        
        let (dir_perms, file_perms) = if mount.read_only {
            (DirPerms::READ, FilePerms::READ)
        } else {
            (DirPerms::all(), FilePerms::all())
        };
        
        // Fail fast if the directory doesn't exist
        if !host_path.exists() {
            return Err(anyhow::anyhow!(
                "Volume mount '{}' path does not exist: {}",
                mount.name,
                host_path.display()
            ));
        }
        builder.preopened_dir(&host_path, guest_path, dir_perms, file_perms)?;
    }
    
    // Configure network access
    if let Some(network) = &config.network {
        if network.inherit {
            // Full network access
            builder.inherit_network();
        } else {
            // Granular network permissions
            let has_any_permission = !network.tcp_bind.is_empty()
                || !network.tcp_connect.is_empty()
                || !network.udp_bind.is_empty()
                || !network.udp_connect.is_empty()
                || !network.udp_outgoing.is_empty();
            
            if has_any_permission {
                let check = network::build_socket_addr_check(network);
                builder.socket_addr_check(check);
            }
        }
        
        // Set DNS resolution permission
        builder.allow_ip_name_lookup(network.allow_ip_name_lookup);
    }
    
    Ok(builder.build())
}

/// Build StoreLimits from WASI configuration.
/// Parses memory limits from Kubernetes resource quantities.
/// Falls back to requests if limits are not specified.
fn build_store_limits(config: &WasiConfig) -> StoreLimits {
    let builder = StoreLimitsBuilder::new();
    
    // Parse memory limit/request
    let builder = if let Some(memory_str) = config.resources.get_memory() {
        if let Some(bytes) = parse_memory_quantity(memory_str) {
            builder.memory_size(bytes)
        } else {
            builder
        }
    } else {
        builder
    };
    
    builder.build()
}

/// Get fuel limit from CPU resource configuration.
/// Converts Kubernetes CPU quantities to Wasmtime fuel units.
/// Falls back to requests if limits are not specified.
/// 1 millicore = 1,000,000 fuel units (1m = 1M fuel)
fn get_fuel_limit(config: &WasiConfig) -> Option<u64> {
    config.resources.get_cpu().and_then(|cpu_str| {
        parse_cpu_quantity(cpu_str).map(|millicores| {
            // Convert millicores to fuel: 1m = 1M fuel
            millicores * 1_000_000
        })
    })
}

/// Parse Kubernetes memory quantity to bytes.
/// Supports: Ki, Mi, Gi, Ti, Pi, Ei (binary) and k, M, G, T, P, E (decimal)
fn parse_memory_quantity(s: &str) -> Option<usize> {
    let s = s.trim();
    
    // Try to parse with suffix
    if let Some(num_str) = s.strip_suffix("Ei") {
        num_str.parse::<usize>().ok().map(|n| n * 1024 * 1024 * 1024 * 1024 * 1024 * 1024)
    } else if let Some(num_str) = s.strip_suffix("Pi") {
        num_str.parse::<usize>().ok().map(|n| n * 1024 * 1024 * 1024 * 1024 * 1024)
    } else if let Some(num_str) = s.strip_suffix("Ti") {
        num_str.parse::<usize>().ok().map(|n| n * 1024 * 1024 * 1024 * 1024)
    } else if let Some(num_str) = s.strip_suffix("Gi") {
        num_str.parse::<usize>().ok().map(|n| n * 1024 * 1024 * 1024)
    } else if let Some(num_str) = s.strip_suffix("Mi") {
        num_str.parse::<usize>().ok().map(|n| n * 1024 * 1024)
    } else if let Some(num_str) = s.strip_suffix("Ki") {
        num_str.parse::<usize>().ok().map(|n| n * 1024)
    } else if let Some(num_str) = s.strip_suffix('E') {
        num_str.parse::<usize>().ok().map(|n| n * 1_000_000_000_000_000_000)
    } else if let Some(num_str) = s.strip_suffix('P') {
        num_str.parse::<usize>().ok().map(|n| n * 1_000_000_000_000_000)
    } else if let Some(num_str) = s.strip_suffix('T') {
        num_str.parse::<usize>().ok().map(|n| n * 1_000_000_000_000)
    } else if let Some(num_str) = s.strip_suffix('G') {
        num_str.parse::<usize>().ok().map(|n| n * 1_000_000_000)
    } else if let Some(num_str) = s.strip_suffix('M') {
        num_str.parse::<usize>().ok().map(|n| n * 1_000_000)
    } else if let Some(num_str) = s.strip_suffix('k') {
        num_str.parse::<usize>().ok().map(|n| n * 1_000)
    } else {
        // No suffix, parse as bytes
        s.parse::<usize>().ok()
    }
}

/// Parse Kubernetes CPU quantity to millicores.
/// Supports: m (millicores) and whole numbers (cores)
/// Examples: "100m" = 100, "1" = 1000, "0.5" = 500
fn parse_cpu_quantity(s: &str) -> Option<u64> {
    let s = s.trim();
    
    if let Some(num_str) = s.strip_suffix('m') {
        // Already in millicores
        num_str.parse::<u64>().ok()
    } else if let Ok(cores) = s.parse::<f64>() {
        // Convert cores to millicores
        Some((cores * 1000.0) as u64)
    } else {
        None
    }
}
