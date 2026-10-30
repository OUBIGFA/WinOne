//! Settings actions and window lifetime use the existing capture Worker UI.
use serde_json::{Value,json};
use winone_desktop::protocol::RpcError;
use windows_sys::Win32::Foundation::HWND;
use crate::component_bridge::Request;

pub fn valid_id(id:&str)->bool{id.len()==32&&id.bytes().all(|b|b.is_ascii_digit()||(b'a'..=b'f').contains(&b))}
fn failure(message:&str)->RpcError{RpcError::new("capture",message)}
pub fn call(request:&Request,client:&winone_desktop::transport::Client,owner:HWND,locale:&str)->Result<Value,RpcError>{
    let method=request.text(0)?;let mut params:Value=serde_json::from_str(request.text(1)?)?;
    if !params.is_null()&&!params.is_object(){return Err(failure("截图参数无效"));}
    let endpoint=crate::component_bridge::module_method("capture",method)?;
    if method=="ai.usage.export" { return crate::usage_export::export(client,params,owner,locale); }
    if method=="ai.archive.export" { return crate::usage_export::export_archive(client,params,owner,locale); }
    if matches!(method,"save.last"|"edit.apply"|"draft.complete"){
        let mut choose=true;
        if method=="draft.complete"{
            let draft=client.call("capture.draft.get",json!({"expectedId":params["expectedId"]}))?;
            if draft["draft"]["id"]!=params["expectedId"]||draft["draft"]["revision"]!=params["revision"]{return Err(failure("截图草稿已变化，请重新加载"));}
            choose=params["saveCopy"]!=true&&(draft["draft"]["settings"]["askWhereToSave"]==true||params["saveAs"]==true);
            if let Some(values)=params.as_object_mut(){values.remove("path");}
        }
        if choose{
            let name=client.call("capture.filename.preview",json!({}))?;
            let Some(path)=crate::dialogs::save_png_labeled(owner,crate::component_bridge::text(locale,"截图另存为","Save capture as"),name["name"].as_str().unwrap_or("WinOne.png"),crate::component_bridge::text(locale,"PNG 图像","PNG image")).map_err(|e|failure(&e))?else{return Ok(json!("{\"state\":\"cancelled\"}"));};
            params["path"]=json!(path);
        }
    }
    if matches!(method,"capture.unified"|"scroll.retake"|"pin.last"|"draft.pin"|"draft.complete"){
        if params.is_null(){params=json!({});}
        let snapshot=client.call("snapshot",Value::Null)?;params["language"]=snapshot["locale"].clone();
    }
    Ok(json!(client.call_completing(&endpoint,params)?.to_string()))
}

pub fn open_window(client:&winone_desktop::transport::Client,id:&str)->Result<Value,RpcError>{
    if !valid_id(id){return Err(failure("截图标识无效"));}
    let opened=client.call("application.open",json!({"draftId":id}))?;
    let window_id=opened["windowId"].as_str().filter(|id|!id.is_empty()).ok_or_else(||failure("截图窗口未能启动"))?;
    if opened["accepted"]!=true{return Err(failure("截图窗口未能启动"));}
    loop{
        let status=client.call("application.capture.window.status",json!({"windowId":window_id}))?;
        if status["id"]!=window_id{return Err(failure("截图窗口状态不匹配"));}
        match status["state"].as_str(){Some("completed")=>return Ok(Value::Null),Some("running")=>{},_=>return Err(failure(status["error"].as_str().unwrap_or("截图窗口异常退出"))),}
        if client.cancellation_requested(){return Err(failure("截图窗口等待已取消"));}
        std::thread::sleep(std::time::Duration::from_millis(200));
    }
}
