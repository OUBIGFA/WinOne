//! Native file selection for usage exports. CSV bytes remain in the Host so a
//! large export never crosses the bounded desktop response frame.
use serde_json::{Value,json};
use windows_sys::Win32::Foundation::HWND;
use winone_desktop::{protocol::RpcError,transport::Client};

pub fn export(client:&Client,params:Value,owner:HWND,locale:&str)->Result<Value,RpcError>{
    let result=save_result(params,||crate::dialogs::save_csv_labeled(owner,
        crate::component_bridge::text(locale,"导出用量 CSV","Export usage CSV"),
        "WinOne-usage.csv",crate::component_bridge::text(locale,"CSV 表格","CSV spreadsheet")),
        |request|client.call_completing("capture.ai.usage.export.save",request))?;
    Ok(json!(result.to_string()))
}

fn save_result(mut query:Value,choose:impl FnOnce()->Result<Option<String>,String>,save:impl FnOnce(Value)->Result<Value,RpcError>)->Result<Value,RpcError>{
    if query.is_null(){query=json!({});}
    let Some(fields)=query.as_object_mut()else{return Err(RpcError::new("export","用量筛选条件无效"));};
    // A renderer-supplied destination is never forwarded, even if an older UI
    // included one. Only the system save dialog chooses the destination.
    fields.remove("path");
    let Some(path)=choose().map_err(|error|RpcError::new("dialog",error))?else{return Ok(json!({"state":"cancelled"}));};
    let result=save(json!({"query":query,"path":path}))?;
    if result["state"]!="saved"||result["path"].as_str()!=Some(path.as_str())||result["rows"].as_u64().is_none(){return Err(RpcError::new("export","无法确认 CSV 已保存"));}
    Ok(result)
}

pub fn export_archive(client:&Client,params:Value,owner:HWND,locale:&str)->Result<Value,RpcError>{
    let result=save_archive_result(params,||crate::dialogs::save_file(owner,
        crate::component_bridge::text(locale,"导出请求归档","Export request archive"),
        "JSON\0*.json\0","json","WinOne-request.json"),
        |request|client.call_completing("capture.ai.archive.export.save",request))?;
    Ok(json!(result.to_string()))
}

// Only a native-selected path is forwarded. Cancellation never starts I/O.
fn save_archive_result(params:Value,choose:impl FnOnce()->Result<Option<String>,String>,save:impl FnOnce(Value)->Result<Value,RpcError>)->Result<Value,RpcError>{
    let Some(archive)=params["archive"].as_str()else{return Err(RpcError::new("export","请求归档标识无效"));};
    let Some(path)=choose().map_err(|error|RpcError::new("dialog",error))?else{return Ok(json!({"state":"cancelled"}));};
    let result=save(json!({"archive":archive,"path":path}))?;
    if result["state"]!="saved"||result["path"].as_str()!=Some(path.as_str()){return Err(RpcError::new("export","无法确认归档已保存"));}
    Ok(result)
}

#[cfg(test)]mod tests{
    use super::*;
    #[test]fn cancellation_never_calls_the_host_save_operation(){
        assert_eq!(save_result(json!({"period":"7d"}),||Ok(None),|_|panic!("cancelled export called Host")).unwrap(),json!({"state":"cancelled"}));
        assert!(save_result(json!([]),||panic!("invalid query opened dialog"),|_|unreachable!()).is_err());
    }
    #[test]fn archive_destination_comes_from_native_dialog(){
        assert_eq!(save_archive_result(json!({"archive":"2026-10-26/a"}),||Ok(None),|_|panic!("cancelled archive export called Host")).unwrap(),json!({"state":"cancelled"}));
        let result=save_archive_result(json!({"archive":"2026-10-26/a","path":"untrusted.json"}),||Ok(Some("chosen.json".into())),|request|{
            assert_eq!(request,json!({"archive":"2026-10-26/a","path":"chosen.json"}));
            Ok(json!({"state":"saved","path":"chosen.json"}))
        }).unwrap();
        assert_eq!(result["path"],"chosen.json");
        assert!(save_archive_result(json!({"archive":"2026-10-26/a"}),||Ok(Some("chosen.json".into())),|_|Ok(json!({"state":"saved","path":"wrong.json"}))).is_err());
        assert!(crate::component_bridge::module_method("capture","ai.archive.read").is_ok());
        assert!(crate::component_bridge::module_method("capture","ai.archive.export").is_ok());
        assert!(crate::component_bridge::module_method("capture","ai.archive.export.save").is_err());
    }
    #[test]fn only_native_selected_path_reaches_host_and_only_saved_receipts_succeed(){
        let query=json!({"period":"7d","offset":15,"path":"untrusted.csv"});
        let result=save_result(query,||Ok(Some("selected.csv".into())),|request|{
            assert_eq!(request,json!({"query":{"period":"7d","offset":15},"path":"selected.csv"}));
            Ok(json!({"state":"saved","path":"selected.csv","rows":10000}))
        }).unwrap();
        assert_eq!(result["rows"],10000);
        assert!(save_result(json!({}),||Ok(Some("selected.csv".into())),|_|Ok(json!({"csv":"not a saved receipt"}))).is_err());
    }
}
