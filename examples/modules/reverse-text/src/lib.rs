use wasi::http::types::{
    Fields, IncomingRequest, OutgoingBody, OutgoingResponse, ResponseOutparam,
};
use std::collections::HashMap;

wasi::http::incoming_handler::export!(Reverse);

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
    pq.split_once("?")
        .and_then(|(_, s)| {
            return Some(querystring::querify(s))
        })
        .map(|q| {
            HashMap::from_iter(q)
        })
        .map(|q| {
            q.get("text")
        })
        .unwrap_or(Some("Hello, WASI!".to_string()))
        .unwrap()
}

fn reverse_text(str: String) -> String {
    str.chars().rev().collect()
}