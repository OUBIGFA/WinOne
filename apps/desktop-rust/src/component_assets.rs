//! Embed the existing frontend build; the Rust host owns no parallel UI source.
use std::borrow::Cow;
use wry::http::{Request,Response};
include!(concat!(env!("OUT_DIR"),"/component_assets.rs"));

pub fn response(request:Request<Vec<u8>>)->Response<Cow<'static,[u8]>>{
    let path=if request.uri().path()=="/"{"/index.html"}else{request.uri().path()};
    let file=FILES.iter().find(|(name,_)|*name==path);
    let mime=match path.rsplit('.').next().unwrap_or(""){
        "html"=>"text/html; charset=utf-8","js"|"mjs"=>"text/javascript; charset=utf-8","wasm"=>"application/wasm","css"=>"text/css; charset=utf-8",
        "png"=>"image/png","svg"=>"image/svg+xml","woff2"=>"font/woff2","ico"=>"image/x-icon",_=>"application/octet-stream",
    };
    Response::builder().status(if file.is_some(){200}else{404}).header("Content-Type",mime)
        .header("X-Content-Type-Options","nosniff")
        .body(Cow::Borrowed(file.map(|(_,bytes)|*bytes).unwrap_or(b"Not found"))).unwrap()
}
