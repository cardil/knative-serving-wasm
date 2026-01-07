use wasi::http::types::{
    Fields, IncomingRequest, OutgoingBody, OutgoingResponse, ResponseOutparam, Scheme,
};
use wasi::exports;

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

    // Make outbound HTTP request
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
    // Parse URL to extract components
    let (scheme, authority, path) = parse_url(url)?;

    // Create outgoing request
    let headers = Fields::new();
    let outgoing_request = wasi::http::types::OutgoingRequest::new(headers);
    
    outgoing_request.set_scheme(Some(&scheme))
        .map_err(|_| "Failed to set scheme".to_string())?;
    outgoing_request.set_authority(Some(&authority))
        .map_err(|_| "Failed to set authority".to_string())?;
    outgoing_request.set_path_with_query(Some(&path))
        .map_err(|_| "Failed to set path".to_string())?;

    // Send request
    let future_response = wasi::http::outgoing_handler::handle(outgoing_request, None)
        .map_err(|e| format!("Failed to send request: {:?}", e))?;

    // Wait for response
    let incoming_response_result = match future_response.get() {
        Some(result) => result,
        None => {
            future_response.subscribe().block();
            future_response
                .get()
                .expect("Response should be ready")
        }
    };

    let incoming_response = incoming_response_result
        .map_err(|e| format!("Request failed: {:?}", e))?
        .map_err(|e| format!("HTTP error: {:?}", e))?;

    let status = incoming_response.status();

    // Read response body
    let incoming_body = incoming_response
        .consume()
        .map_err(|_| "Failed to consume response body".to_string())?;
    
    let body_stream = incoming_body
        .stream()
        .map_err(|_| "Failed to get body stream".to_string())?;

    let mut body_bytes = Vec::new();
    loop {
        match body_stream.read(8192) {
            Ok(chunk) => {
                if chunk.is_empty() {
                    break;
                }
                body_bytes.extend_from_slice(&chunk);
            }
            Err(_) => break,
        }
    }

    let body = String::from_utf8(body_bytes)
        .unwrap_or_else(|_| "<binary data>".to_string());

    Ok((status, body))
}

fn parse_url(url: &str) -> Result<(Scheme, String, String), String> {
    // Simple URL parser
    let url = url.trim();
    
    let (scheme_str, rest) = if let Some(pos) = url.find("://") {
        (&url[..pos], &url[pos + 3..])
    } else {
        return Err("Invalid URL: missing scheme".to_string());
    };

    let scheme = match scheme_str.to_lowercase().as_str() {
        "http" => Scheme::Http,
        "https" => Scheme::Https,
        _ => return Err(format!("Unsupported scheme: {}", scheme_str)),
    };

    let (authority, path) = if let Some(pos) = rest.find('/') {
        (&rest[..pos], &rest[pos..])
    } else {
        (rest, "/")
    };

    Ok((scheme, authority.to_string(), path.to_string()))
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
