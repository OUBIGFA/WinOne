//! Cindy .cshare save selection. Only native code supplies the destination;
//! complete session bytes and optional passwords never enter browser replies.
use serde_json::{Value,json};
use windows_sys::Win32::Foundation::HWND;
use winone_desktop::{protocol::RpcError,transport::Client};

pub fn export(client:&Client,params:Value,owner:HWND,locale:&str)->Result<Value,RpcError>{
    let result=save_result(params,||crate::dialogs::save_cshare_labeled(owner,
        crate::component_bridge::text(locale,"导出完整任务","Export complete task"),"WinOne-task.cshare",
        crate::component_bridge::text(locale,"Cindy 任务文件","Cindy task archive")),
        |request|client.call("chat.export",request))?;
    Ok(json!(result.to_string()))
}

fn save_result(mut params:Value,choose:impl FnOnce()->Result<Option<String>,String>,save:impl FnOnce(Value)->Result<Value,RpcError>)->Result<Value,RpcError>{
    let Some(fields)=params.as_object_mut()else{return Err(RpcError::new("chat_export","任务导出参数无效"));};
    if fields.keys().any(|key|!matches!(key.as_str(),"id"|"password"|"excludeMedia"))
        ||fields.get("id").and_then(Value::as_str).is_none_or(|id|id.is_empty())
        ||fields.get("password").is_some_and(|value|value.as_str().is_none_or(|text|text.chars().count()>4096))
        ||fields.get("excludeMedia").is_some_and(|value|!value.is_boolean()){
        return Err(RpcError::new("chat_export","任务导出参数无效"));
    }
    let Some(path)=choose().map_err(|error|RpcError::new("dialog",error))?else{return Ok(json!({"status":"canceled"}));};
    fields.insert("targetPath".into(),json!(path));
    let result=save(params)?;
    match result["status"].as_str(){
        Some("ok") if result["filePath"].as_str()==Some(path.as_str())&&matches!(result["fidelity"].as_str(),Some("full"|"partial"|"db-only"))=>Ok(result),
        Some("oversize") if result["limitBytes"].as_u64().is_some_and(|limit|limit>0&&result["totalBytes"].as_u64().is_some_and(|total|total>limit))=>Ok(result),
        _=>Err(RpcError::new("chat_export","无法确认任务已导出")),
    }
}

#[cfg(test)]mod tests{
    use super::*;
    #[test]fn cancellation_and_renderer_paths_never_write_an_archive(){
        assert_eq!(save_result(json!({"id":"task"}),||Ok(None),|_|panic!("cancelled save called Host")).unwrap(),json!({"status":"canceled"}));
        assert!(save_result(json!({"id":"task","targetPath":"untrusted.cshare"}),||panic!("untrusted destination opened dialog"),|_|unreachable!()).is_err());
    }
    #[test]fn native_path_and_full_or_partial_receipt_are_preserved(){
        let result=save_result(json!({"id":"task","password":"secret","excludeMedia":true}),||Ok(Some("selected.cshare".into())),|request|{
            assert_eq!(request,json!({"id":"task","password":"secret","excludeMedia":true,"targetPath":"selected.cshare"}));
            Ok(json!({"status":"ok","filePath":"selected.cshare","fidelity":"partial","mediaMissing":1}))
        }).unwrap();
        assert_eq!(result["fidelity"],"partial");
        assert!(save_result(json!({"id":"task"}),||Ok(Some("selected.cshare".into())),|_|Ok(json!({"status":"ok","filePath":"wrong.cshare","fidelity":"full"}))).is_err());
    }
}
