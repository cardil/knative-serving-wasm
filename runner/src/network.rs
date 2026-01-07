use std::future::Future;
use std::net::{IpAddr, SocketAddr};
use std::pin::Pin;
use wasmtime_wasi::SocketAddrUse;

use crate::config::NetworkSpec;

/// Matches a socket address against a list of address patterns.
/// 
/// Pattern format: "host:port" where:
/// - host can be an IP address, hostname, or "*" for wildcard
/// - port can be a number or "*" for wildcard
/// 
/// Examples:
/// - "*:*" - matches any address
/// - "api.example.com:443" - matches specific host and port
/// - "*.internal.svc:*" - matches any subdomain of internal.svc on any port
/// - "10.0.0.1:8080" - matches specific IP and port
pub fn matches_pattern(addr: &SocketAddr, patterns: &[String]) -> bool {
    for pattern in patterns {
        if let Some((host_pat, port_pat)) = pattern.rsplit_once(':') {
            // Check port match
            let port_matches = port_pat == "*" || 
                port_pat.parse::<u16>().ok() == Some(addr.port());
            
            if !port_matches {
                continue;
            }
            
            // Check host match
            let host_matches = if host_pat == "*" {
                true
            } else if let Ok(ip) = host_pat.parse::<IpAddr>() {
                // Direct IP match
                addr.ip() == ip
            } else {
                // Hostname pattern - for now, we can't resolve hostnames in the pattern
                // since we only have the SocketAddr (already resolved IP).
                // This is a limitation - we'd need reverse DNS or the original hostname.
                // For now, treat non-IP patterns as not matching.
                // TODO: Implement hostname pattern matching with reverse DNS or caching
                false
            };
            
            if host_matches {
                return true;
            }
        }
    }
    false
}

/// Build a socket address checker function from NetworkSpec.
/// This function will be called by Wasmtime for each socket operation.
/// Returns an async function as required by wasmtime-wasi.
pub fn build_socket_addr_check(
    network: &NetworkSpec,
) -> impl Fn(SocketAddr, SocketAddrUse) -> Pin<Box<dyn Future<Output = bool> + Send + Sync>> + 'static {
    let tcp_bind = network.tcp_bind.clone();
    let tcp_connect = network.tcp_connect.clone();
    let udp_bind = network.udp_bind.clone();
    let udp_connect = network.udp_connect.clone();
    let udp_outgoing = network.udp_outgoing.clone();
    
    move |addr: SocketAddr, use_: SocketAddrUse| -> Pin<Box<dyn Future<Output = bool> + Send + Sync>> {
        let result = match use_ {
            SocketAddrUse::TcpBind => matches_pattern(&addr, &tcp_bind),
            SocketAddrUse::TcpConnect => matches_pattern(&addr, &tcp_connect),
            SocketAddrUse::UdpBind => matches_pattern(&addr, &udp_bind),
            SocketAddrUse::UdpConnect => matches_pattern(&addr, &udp_connect),
            SocketAddrUse::UdpOutgoingDatagram => matches_pattern(&addr, &udp_outgoing),
        };
        Box::pin(async move { result })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::{IpAddr, Ipv4Addr, SocketAddr};

    #[test]
    fn test_wildcard_pattern() {
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        let patterns = vec!["*:*".to_string()];
        assert!(matches_pattern(&addr, &patterns));
    }

    #[test]
    fn test_wildcard_port() {
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        let patterns = vec!["127.0.0.1:*".to_string()];
        assert!(matches_pattern(&addr, &patterns));
    }

    #[test]
    fn test_specific_ip_and_port() {
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        let patterns = vec!["127.0.0.1:8080".to_string()];
        assert!(matches_pattern(&addr, &patterns));
    }

    #[test]
    fn test_no_match() {
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        let patterns = vec!["192.168.1.1:8080".to_string()];
        assert!(!matches_pattern(&addr, &patterns));
    }

    #[test]
    fn test_multiple_patterns() {
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 8080);
        let patterns = vec![
            "192.168.1.1:8080".to_string(),
            "127.0.0.1:8080".to_string(),
        ];
        assert!(matches_pattern(&addr, &patterns));
    }
}
