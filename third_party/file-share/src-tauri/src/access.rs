//! WinOne access boundary; the upstream transfer routes remain unchanged behind it.
use std::sync::Arc;
use axum::{extract::{Request, State}, http::{header, HeaderValue, StatusCode}, middleware::Next, response::{IntoResponse, Response}};
use crate::server::ServerInfo;

pub(crate) struct Access {
    pub token: String,
    authorities: Vec<String>,
}
impl Access {
    pub fn new(info: &ServerInfo) -> Self {
        let mut authorities: Vec<_> = info.addresses.iter().map(|a| format!("{}:{}", a.ip, info.port)).collect();
        authorities.push(format!("127.0.0.1:{}", info.port));
        authorities.push(format!("localhost:{}", info.port));
        Self {token: uuid::Uuid::new_v4().simple().to_string(), authorities}
    }
}

pub(crate) async fn protect(State(access): State<Arc<Access>>, request: Request, next: Next) -> Response {
    let authority = request.headers().get(header::HOST).and_then(|v| v.to_str().ok()).unwrap_or("");
    // Reject DNS rebinding and browser writes from unrelated origins.
    if !access.authorities.iter().any(|a| a.eq_ignore_ascii_case(authority)) {
        return (StatusCode::FORBIDDEN, "访问地址不受支持").into_response();
    }
    if let Some(origin) = request.headers().get(header::ORIGIN) {
        if origin.to_str().ok() != Some(format!("http://{authority}").as_str()) {
            return (StatusCode::FORBIDDEN, "请从共享链接打开页面").into_response();
        }
    }
    let key = request.uri().query().and_then(|q| q.split('&').find_map(|p| p.strip_prefix("key=")));
    let path = request.uri().path();
    let share_entry = path == "/" || (path.starts_with("/api/items/") && path.ends_with("/download"));
    if request.method() == axum::http::Method::GET && share_entry && key == Some(access.token.as_str()) {
        // Strip the credential before any browser assets or external links load.
        let mut response = StatusCode::SEE_OTHER.into_response();
        response.headers_mut().insert(header::LOCATION, HeaderValue::from_str(path).unwrap());
        response.headers_mut().insert(header::SET_COOKIE, HeaderValue::from_str(&format!("winone_share={}; HttpOnly; SameSite=Strict; Path=/",access.token)).unwrap());
        return private_response(response);
    }
    let bearer = request.headers().get(header::AUTHORIZATION).and_then(|v| v.to_str().ok()).and_then(|v| v.strip_prefix("Bearer "));
    let cookie = request.headers().get(header::COOKIE).and_then(|v| v.to_str().ok()).and_then(|v| v.split(';').find_map(|p| p.trim().strip_prefix("winone_share=")));
    if bearer != Some(access.token.as_str()) && cookie != Some(access.token.as_str()) {
        return private_response((StatusCode::UNAUTHORIZED, "共享链接已失效，请使用发送方当前的链接或二维码").into_response());
    }
    private_response(next.run(request).await)
}

fn private_response(mut response: Response) -> Response {
    for name in [header::ACCESS_CONTROL_ALLOW_ORIGIN, header::ACCESS_CONTROL_ALLOW_HEADERS, header::ACCESS_CONTROL_ALLOW_METHODS] {
        response.headers_mut().remove(name);
    }
    response.headers_mut().insert(header::CACHE_CONTROL, HeaderValue::from_static("no-store"));
    response.headers_mut().insert(header::REFERRER_POLICY, HeaderValue::from_static("no-referrer"));
    response.headers_mut().insert(header::X_CONTENT_TYPE_OPTIONS, HeaderValue::from_static("nosniff"));
    response
}
