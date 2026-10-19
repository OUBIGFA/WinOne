//! In-process recording service. The C ABI exchanges bounded UTF-8 JSON only;
//! no native pointers, frame bytes or HWNDs are persisted as configuration.
pub mod core;
mod capture;
pub mod encoder;
pub mod session;
use parking_lot::Mutex;
use serde::Deserialize;
use serde_json::{Value,json};
use std::{ffi::{CString,c_char},panic::{AssertUnwindSafe,catch_unwind},sync::OnceLock};
use core::Config;
use session::Session;
static SESSION:OnceLock<Mutex<Option<Session>>>=OnceLock::new();
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Request {method:String,#[serde(default)]config:Option<Config>}
fn handle(input:&str)->Result<Value,String>{
    let request:Request=serde_json::from_str(input).map_err(|e|format!("录制请求无效：{e}"))?;
    let mut current=SESSION.get_or_init(||Mutex::new(None)).lock();
    match request.method.as_str(){
        "start_recording"=>{
            if current.as_ref().is_some_and(Session::active){return Err("已有录制任务，请先停止当前任务".into());}
            let config=request.config.ok_or("缺少录制配置")?;
            *current=Some(Session::start(config)?);
        },
        "pause_recording"=>{current.as_ref().ok_or("当前没有录制任务")?.pause()?;},
        "resume_recording"=>{current.as_ref().ok_or("当前没有录制任务")?.resume()?;},
        "stop_recording"|"cancel_recording"=>{current.as_mut().ok_or("当前没有录制任务")?.finish(request.method=="cancel_recording");},
        "status"=>(),
        _=>return Err("未知录制操作".into()),
    }
    Ok(match current.as_ref(){Some(session)=>serde_json::to_value(session.shared.snapshot()).map_err(|e|e.to_string())?,None=>json!({"phase":"idle"})})
}
/// Executes a recording lifecycle request. Callers must serialize UI actions;
/// the service also serializes them to prevent stop/pause races across threads.
pub fn command(input:&str)->Value {
    match catch_unwind(AssertUnwindSafe(||handle(input))){
        Ok(Ok(status))=>json!({"ok":true,"status":status}),
        Ok(Err(error))=>json!({"ok":false,"error":error}),
        Err(_)=>json!({"ok":false,"error":"录制引擎发生内部错误，请停止录制后重试"}),
    }
}
/// # Safety
/// `input` must reference `length` readable bytes for this call. The returned
/// string belongs to Rust and must be released exactly once with response_free.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn winone_recording_command(input:*const u8,length:usize)->*mut c_char{
    let result=if input.is_null() || length==0 || length>65536 {json!({"ok":false,"error":"录制请求长度无效"})}
    else{match std::str::from_utf8(unsafe{std::slice::from_raw_parts(input,length)}){Ok(text)=>command(text),Err(_)=>json!({"ok":false,"error":"录制请求不是 UTF-8"})}};
    CString::new(result.to_string()).expect("JSON escapes NUL bytes").into_raw()
}
/// # Safety
/// `response` must be a live string returned by winone_recording_command.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn winone_recording_response_free(response:*mut c_char){if !response.is_null(){drop(unsafe{CString::from_raw(response)});}}
#[cfg(test)]
mod tests {
    use super::*;
    #[test] fn invalid_requests_do_not_start_capture(){assert_eq!(command("{}")["ok"],false);assert_eq!(command(r#"{"method":"unknown"}"#)["ok"],false);assert_eq!(command(r#"{"method":"start_recording"}"#)["ok"],false);}
    #[test] fn ffi_rejects_invalid_utf8_and_releases_its_response(){let data=[0xff];let response=unsafe{winone_recording_command(data.as_ptr(),data.len())};let value:Value=serde_json::from_slice(unsafe{std::ffi::CStr::from_ptr(response)}.to_bytes()).unwrap();assert_eq!(value["ok"],false);unsafe{winone_recording_response_free(response)};}
}
