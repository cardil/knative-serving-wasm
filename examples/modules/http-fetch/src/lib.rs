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

use wasi::http::types::{
    Fields, IncomingRequest, OutgoingBody, OutgoingResponse, ResponseOutparam,
};
use wasi::exports;
use std::io::{Read, Write};
use std::net::TcpStream;

wasi::http::proxy::export!(Component);

struct Component;

impl exports::wasi::http::incoming_handler::Guest for Component {
    fn handle(request: IncomingRequest, response_out: ResponseOutparam) {
        let response = handle_request(request).unwrap_or_else(|err| {
            let error_json = format!(
                r#"{{"status":null,"body":null,"error":"{}"}}"#,
                err.replace('"', "\\\"")
            );
            create_json_response(500, error_json)
        });

        ResponseOutparam::set(response_out, Ok(response));
    }
}

fn handle_request(request: IncomingRequest) -> Result<OutgoingResponse, String> {
    // Extract target URL from X-Target-URL header or url query parameter
    let target_url = extract_target_url(&request)?;

    // Make outbound HTTP request using standard library
    match fetch_url(&target_url) {
        Ok((status, body)) => {
            let json = format!(
                r#"{{"status":{},"body":"{}","error":null}}"#,
                status,
                body.replace('"', "\\\"").replace('\n', "\\n").replace('\r', "\\r")
            );
            Ok(create_json_response(200, json))
        }
        Err(err) => {
            let json = format!(
                r#"{{"status":null,"body":null,"error":"{}"}}"#,
                err.replace('"', "\\\"")
            );
            Ok(create_json_response(200, json))
        }
    }
}

fn extract_target_url(request: &IncomingRequest) -> Result<String, String> {
    // Try X-Target-URL header first
    let headers = request.headers();
    let entries = headers.entries();
    
    for (key, value) in entries {
        if key.to_lowercase() == "x-target-url" {
            return String::from_utf8(value)
                .map_err(|_| "Invalid UTF-8 in X-Target-URL header".to_string());
        }
    }

    // Try url query parameter
    let path_with_query = request.path_with_query().unwrap_or_default();
    
    if let Some(query_start) = path_with_query.find('?') {
        let query = &path_with_query[query_start + 1..];
        let params = querystring::querify(query);
        
        for (key, value) in params {
            if key == "url" {
                return Ok(value.to_string());
            }
        }
    }

    Err("Missing X-Target-URL header or url query parameter".to_string())
}

fn fetch_url(url: &str) -> Result<(u16, String), String> {
    // Parse URL
    let (scheme, host, port, path) = parse_url(url)?;
    
    if scheme != "http" {
        return Err("Only HTTP is supported (not HTTPS)".to_string());
    }

    // Connect using standard library TcpStream (WASI will intercept this)
    let addr = format!("{}:{}", host, port);
    let mut stream = TcpStream::connect(&addr)
        .map_err(|e| format!("Failed to connect to {}: {}", addr, e))?;

    // Send HTTP request
    let request = format!(
        "GET {} HTTP/1.1\r\nHost: {}\r\nConnection: close\r\n\r\n",
        path, host
    );
    
    stream.write_all(request.as_bytes())
        .map_err(|e| format!("Failed to send request: {}", e))?;

    // Read response
    let mut response_data = Vec::new();
    stream.read_to_end(&mut response_data)
        .map_err(|e| format!("Failed to read response: {}", e))?;

    // Parse HTTP response
    let mut headers = [httparse::EMPTY_HEADER; 64];
    let mut response = httparse::Response::new(&mut headers);
    
    let body_offset = response.parse(&response_data)
        .map_err(|e| format!("Failed to parse response: {:?}", e))?
        .unwrap();

    let status = response.code
        .ok_or_else(|| "No status code in response".to_string())?;

    let body = String::from_utf8_lossy(&response_data[body_offset..]).to_string();

    Ok((status, body))
}

fn parse_url(url: &str) -> Result<(String, String, u16, String), String> {
    let url = url.trim();
    
    // Extract scheme
    let (scheme, rest) = if let Some(pos) = url.find("://") {
        (url[..pos].to_lowercase(), &url[pos + 3..])
    } else {
        return Err("Invalid URL: missing scheme".to_string());
    };

    // Extract host and optional port
    let (host_port, path) = if let Some(pos) = rest.find('/') {
        (&rest[..pos], &rest[pos..])
    } else {
        (rest, "/")
    };

    let (host, port) = if let Some(pos) = host_port.find(':') {
        let host = host_port[..pos].to_string();
        let port = host_port[pos + 1..]
            .parse::<u16>()
            .map_err(|_| "Invalid port number".to_string())?;
        (host, port)
    } else {
        let default_port = match scheme.as_str() {
            "http" => 80,
            "https" => 443,
            _ => return Err(format!("Unsupported scheme: {}", scheme)),
        };
        (host_port.to_string(), default_port)
    };

    Ok((scheme, host, port, path.to_string()))
}

fn create_json_response(status: u16, json_body: String) -> OutgoingResponse {
    let headers = Fields::new();
    headers
        .set(&"content-type".to_string(), &[b"application/json".to_vec()])
        .unwrap();

    let response = OutgoingResponse::new(headers);
    response.set_status_code(status).unwrap();

    let body = response.body().unwrap();
    let body_stream = body.write().unwrap();
    
    body_stream.blocking_write_and_flush(json_body.as_bytes()).unwrap();
    drop(body_stream);
    
    OutgoingBody::finish(body, None).unwrap();

    response
}
