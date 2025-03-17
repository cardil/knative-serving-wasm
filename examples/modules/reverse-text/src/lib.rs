use wasi::http::types::{
    Fields, IncomingRequest, OutgoingBody, OutgoingResponse, ResponseOutparam,
};
use std::collections::HashMap;
use wasi::exports;

wasi::http::proxy::export!(Reverse);

struct Reverse;

impl exports::wasi::http::incoming_handler::Guest for Reverse {
    fn handle(request: IncomingRequest, response_out: ResponseOutparam) {
        let resp = OutgoingResponse::new(Fields::new());
        let body = resp.body().unwrap();

        ResponseOutparam::set(response_out, Ok(resp));

        let pq = request.path_with_query().unwrap();
        let input = fetch_text_query_param(pq);
        let value = reverse_text(input);

        let out = body.write().unwrap();
        out.blocking_write_and_flush(value.as_bytes()).unwrap();
        drop(out);

        OutgoingBody::finish(body, None).unwrap();
    }
}

/**
Get query parameter named "text", or return "Hello, WASI!" if
it's not present
 */
fn fetch_text_query_param(pq: String) -> String {
    urlencoding::decode(&pq)
        .unwrap()
        .split_once("?")
        .and_then(|(_, s)| {
            return Some(querystring::querify(s));
        })
        .map(|q| {
            HashMap::from_iter(q)
        })
        .and_then(|q: HashMap<&str, &str>| {
            q.get("text").cloned()
        })
        .map(|s| {
            s.to_string()
        })
        .unwrap_or("Hello, WASI!".to_string())
}

fn reverse_text(str: String) -> String {
    str.chars().rev().collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_fetch_text_query_param() {
        assert_eq!(fetch_text_query_param("".to_string()), "Hello, WASI!");
        assert_eq!(fetch_text_query_param("?".to_string()), "Hello, WASI!");
        assert_eq!(fetch_text_query_param("?text=Hello".to_string()), "Hello");
        assert_eq!(fetch_text_query_param("?text=Happy%20testing".to_string()), "Happy testing");
    }

    #[test]
    fn test_reverse_text() {
        assert_eq!(reverse_text("".to_string()), "");
        assert_eq!(reverse_text("a".to_string()), "a");
        assert_eq!(reverse_text("ab".to_string()), "ba");
        assert_eq!(reverse_text("abc".to_string()), "cba");
    }
}
