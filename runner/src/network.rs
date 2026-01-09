use std::collections::HashMap;
use std::future::Future;
use std::net::{IpAddr, SocketAddr, ToSocketAddrs};
use std::pin::Pin;
use wasmtime_wasi::SocketAddrUse;

use crate::config::NetworkSpec;

/// Network checker that resolves hostname patterns at startup
#[derive(Clone)]
pub struct NetworkChecker {
    tcp_bind: Vec<String>,
    tcp_connect: Vec<String>,
    udp_bind: Vec<String>,
    udp_connect: Vec<String>,
    udp_outgoing: Vec<String>,
    /// Maps original hostname patterns to resolved IP patterns
    resolved_patterns: HashMap<String, Vec<String>>,
}

impl NetworkChecker {
    /// Create a new NetworkChecker from NetworkSpec, resolving all hostname patterns
    pub fn new(network: &NetworkSpec) -> Self {
        let mut checker = Self {
            tcp_bind: network.tcp_bind.clone(),
            tcp_connect: network.tcp_connect.clone(),
            udp_bind: network.udp_bind.clone(),
            udp_connect: network.udp_connect.clone(),
            udp_outgoing: network.udp_outgoing.clone(),
            resolved_patterns: HashMap::new(),
        };

        // Collect all unique patterns
        let mut all_patterns = Vec::new();
        all_patterns.extend(network.tcp_bind.iter().cloned());
        all_patterns.extend(network.tcp_connect.iter().cloned());
        all_patterns.extend(network.udp_bind.iter().cloned());
        all_patterns.extend(network.udp_connect.iter().cloned());
        all_patterns.extend(network.udp_outgoing.iter().cloned());
        all_patterns.sort();
        all_patterns.dedup();

        // Resolve hostname patterns
        for pattern in all_patterns {
            if let Some(resolved) = checker.resolve_pattern(&pattern) {
                checker.resolved_patterns.insert(pattern, resolved);
            }
        }

        checker
    }

    /// Resolve a hostname pattern to IP patterns
    /// Returns None if pattern is already an IP or wildcard
    fn resolve_pattern(&self, pattern: &str) -> Option<Vec<String>> {
        let (host_pat, port_pat) = pattern.rsplit_once(':')?;

        // Skip if already an IP address or wildcard
        if host_pat == "*" || host_pat.parse::<IpAddr>().is_ok() {
            return None;
        }

        // Try to resolve the hostname
        let addr_str = format!("{}:{}", host_pat, port_pat);
        match addr_str.to_socket_addrs() {
            Ok(addrs) => {
                let resolved: Vec<String> = addrs
                    .map(|addr| {
                        // Format IPv6 addresses with brackets
                        if addr.is_ipv6() {
                            format!("[{}]:{}", addr.ip(), addr.port())
                        } else {
                            format!("{}:{}", addr.ip(), addr.port())
                        }
                    })
                    .collect();

                if !resolved.is_empty() {
                    eprintln!(
                        "[WASM-RUNNER] Resolved hostname pattern '{}' to {} IP(s): {:?}",
                        pattern,
                        resolved.len(),
                        resolved
                    );
                    Some(resolved)
                } else {
                    eprintln!(
                        "[WASM-RUNNER] Warning: hostname pattern '{}' resolved to no addresses",
                        pattern
                    );
                    None
                }
            }
            Err(e) => {
                eprintln!(
                    "[WASM-RUNNER] Warning: failed to resolve hostname pattern '{}': {}",
                    pattern, e
                );
                None
            }
        }
    }

    /// Get patterns for a specific socket use, including resolved IPs
    fn get_patterns(&self, use_: SocketAddrUse) -> Vec<&str> {
        let original_patterns = match use_ {
            SocketAddrUse::TcpBind => &self.tcp_bind,
            SocketAddrUse::TcpConnect => &self.tcp_connect,
            SocketAddrUse::UdpBind => &self.udp_bind,
            SocketAddrUse::UdpConnect => &self.udp_connect,
            SocketAddrUse::UdpOutgoingDatagram => &self.udp_outgoing,
        };

        let mut patterns: Vec<&str> = original_patterns.iter().map(|s| s.as_str()).collect();

        // Add resolved IP patterns for any hostname patterns
        for original in original_patterns {
            if let Some(resolved) = self.resolved_patterns.get(original) {
                patterns.extend(resolved.iter().map(|s| s.as_str()));
            }
        }

        patterns
    }

    /// Check if an address matches any pattern for the given socket use
    pub fn check(&self, addr: &SocketAddr, use_: SocketAddrUse) -> bool {
        let patterns = self.get_patterns(use_);
        eprintln!(
            "[WASM-RUNNER] Checking address {} against {} patterns (including resolved): {:?}",
            addr,
            patterns.len(),
            patterns
        );

        for pattern in patterns {
            if let Some((host_pat, port_pat)) = pattern.rsplit_once(':') {
                // Check port match
                let port_matches = port_pat == "*"
                    || port_pat.parse::<u16>().ok() == Some(addr.port());

                eprintln!(
                    "[WASM-RUNNER]   Pattern '{}' - port_matches: {}",
                    pattern, port_matches
                );

                if !port_matches {
                    continue;
                }

                // Check host match
                let host_matches = if host_pat == "*" {
                    eprintln!("[WASM-RUNNER]   Pattern '{}' - wildcard host match", pattern);
                    true
                } else if let Ok(ip) = host_pat.trim_matches(|c| c == '[' || c == ']').parse::<IpAddr>() {
                    // Direct IP match (handle IPv6 brackets)
                    let matches = addr.ip() == ip;
                    eprintln!(
                        "[WASM-RUNNER]   Pattern '{}' - IP match: {}",
                        pattern, matches
                    );
                    matches
                } else {
                    // Hostname pattern - should have been resolved
                    eprintln!(
                        "[WASM-RUNNER]   Pattern '{}' - hostname pattern (should be resolved)",
                        pattern
                    );
                    false
                };

                if host_matches {
                    eprintln!(
                        "[WASM-RUNNER]   ALLOWED: {} matches pattern '{}'",
                        addr, pattern
                    );
                    return true;
                }
            }
        }

        eprintln!(
            "[WASM-RUNNER]   DENIED: {} does not match any pattern",
            addr
        );
        false
    }
}

/// Build a socket address checker function from NetworkSpec.
/// This function will be called by Wasmtime for each socket operation.
/// Returns an async function as required by wasmtime-wasi.
///
/// This function resolves hostname patterns at startup to avoid DNS lookups
/// during runtime checks.
pub fn build_socket_addr_check(
    network: &NetworkSpec,
) -> impl Fn(SocketAddr, SocketAddrUse) -> Pin<Box<dyn Future<Output = bool> + Send + Sync>> + 'static {
    let checker = NetworkChecker::new(network);
    
    eprintln!("[WASM-RUNNER] build_socket_addr_check called - creating NetworkChecker with hostname resolution");
    
    move |addr: SocketAddr, use_: SocketAddrUse| -> Pin<Box<dyn Future<Output = bool> + Send + Sync>> {
        eprintln!("[WASM-RUNNER] socket_addr_check closure invoked for {} with use {:?}", addr, use_);
        let result = checker.check(&addr, use_);
        eprintln!("[WASM-RUNNER] socket_addr_check result: {}", result);
        Box::pin(async move { result })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};

    fn create_network_spec(tcp_connect: Vec<String>) -> NetworkSpec {
        NetworkSpec {
            inherit: false,
            allow_ip_name_lookup: true,
            tcp_bind: vec![],
            tcp_connect,
            udp_bind: vec![],
            udp_connect: vec![],
            udp_outgoing: vec![],
        }
    }

    #[test]
    fn test_wildcard_pattern() {
        let spec = create_network_spec(vec!["*:*".to_string()]);
        let checker = NetworkChecker::new(&spec);
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        assert!(checker.check(&addr, SocketAddrUse::TcpConnect));
    }

    #[test]
    fn test_wildcard_port() {
        let spec = create_network_spec(vec!["127.0.0.1:*".to_string()]);
        let checker = NetworkChecker::new(&spec);
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        assert!(checker.check(&addr, SocketAddrUse::TcpConnect));
    }

    #[test]
    fn test_specific_ip_and_port() {
        let spec = create_network_spec(vec!["127.0.0.1:8080".to_string()]);
        let checker = NetworkChecker::new(&spec);
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        assert!(checker.check(&addr, SocketAddrUse::TcpConnect));
    }

    #[test]
    fn test_no_match() {
        let spec = create_network_spec(vec!["192.168.1.1:8080".to_string()]);
        let checker = NetworkChecker::new(&spec);
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        assert!(!checker.check(&addr, SocketAddrUse::TcpConnect));
    }

    #[test]
    fn test_multiple_patterns() {
        let spec = create_network_spec(vec![
            "192.168.1.1:8080".to_string(),
            "127.0.0.1:8080".to_string(),
        ]);
        let checker = NetworkChecker::new(&spec);
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        assert!(checker.check(&addr, SocketAddrUse::TcpConnect));
    }

    #[test]
    fn test_hostname_resolution_localhost() {
        // localhost should resolve to 127.0.0.1
        let spec = create_network_spec(vec!["localhost:8080".to_string()]);
        let checker = NetworkChecker::new(&spec);
        
        // Check that the resolved IP is stored
        assert!(checker.resolved_patterns.contains_key("localhost:8080"));
        
        // Check that 127.0.0.1:8080 matches
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        assert!(checker.check(&addr, SocketAddrUse::TcpConnect));
    }

    #[test]
    fn test_mixed_hostname_and_ip_patterns() {
        let spec = create_network_spec(vec![
            "localhost:8080".to_string(),
            "192.168.1.1:9090".to_string(),
        ]);
        let checker = NetworkChecker::new(&spec);
        
        // localhost should resolve
        let addr1 = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        assert!(checker.check(&addr1, SocketAddrUse::TcpConnect));
        
        // Direct IP should work
        let addr2 = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(192, 168, 1, 1)), 9090);
        assert!(checker.check(&addr2, SocketAddrUse::TcpConnect));
        
        // Non-matching should fail
        let addr3 = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)), 8080);
        assert!(!checker.check(&addr3, SocketAddrUse::TcpConnect));
    }

    #[test]
    fn test_ipv6_pattern() {
        let spec = create_network_spec(vec!["[::1]:8080".to_string()]);
        let checker = NetworkChecker::new(&spec);
        let addr = SocketAddr::new(IpAddr::V6(Ipv6Addr::new(0, 0, 0, 0, 0, 0, 0, 1)), 8080);
        assert!(checker.check(&addr, SocketAddrUse::TcpConnect));
    }

    #[test]
    fn test_ipv6_hostname_resolution() {
        // localhost should resolve to both IPv4 and IPv6
        let spec = create_network_spec(vec!["localhost:8080".to_string()]);
        let checker = NetworkChecker::new(&spec);
        
        // Check IPv6 loopback
        let addr = SocketAddr::new(IpAddr::V6(Ipv6Addr::new(0, 0, 0, 0, 0, 0, 0, 1)), 8080);
        // This may or may not match depending on system DNS configuration
        // Just verify the checker doesn't panic
        let _ = checker.check(&addr, SocketAddrUse::TcpConnect);
    }

    #[test]
    fn test_different_socket_uses() {
        let spec = NetworkSpec {
            inherit: false,
            allow_ip_name_lookup: true,
            tcp_bind: vec!["127.0.0.1:8080".to_string()],
            tcp_connect: vec!["192.168.1.1:9090".to_string()],
            udp_bind: vec!["127.0.0.1:5353".to_string()],
            udp_connect: vec![],
            udp_outgoing: vec!["*:*".to_string()],
        };
        let checker = NetworkChecker::new(&spec);
        
        let addr1 = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        assert!(checker.check(&addr1, SocketAddrUse::TcpBind));
        assert!(!checker.check(&addr1, SocketAddrUse::TcpConnect));
        
        let addr2 = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(192, 168, 1, 1)), 9090);
        assert!(!checker.check(&addr2, SocketAddrUse::TcpBind));
        assert!(checker.check(&addr2, SocketAddrUse::TcpConnect));
        
        let addr3 = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 5353);
        assert!(checker.check(&addr3, SocketAddrUse::UdpBind));
        
        let addr4 = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(8, 8, 8, 8)), 53);
        assert!(checker.check(&addr4, SocketAddrUse::UdpOutgoingDatagram));
    }

    #[test]
    fn test_invalid_hostname_pattern() {
        // Invalid hostname should be logged but not crash
        let spec = create_network_spec(vec![
            "invalid.hostname.that.does.not.exist.example:8080".to_string(),
            "127.0.0.1:8080".to_string(),
        ]);
        let checker = NetworkChecker::new(&spec);
        
        // The valid IP pattern should still work
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        assert!(checker.check(&addr, SocketAddrUse::TcpConnect));
    }

    #[test]
    fn test_port_mismatch() {
        let spec = create_network_spec(vec!["localhost:8080".to_string()]);
        let checker = NetworkChecker::new(&spec);
        
        // Wrong port should not match
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 9090);
        assert!(!checker.check(&addr, SocketAddrUse::TcpConnect));
    }
}
