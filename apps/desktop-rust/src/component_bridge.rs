//! Existing frontend methods mapped to the existing Host contracts.
use serde::{Deserialize,Serialize};
use serde_json::{Value,json};
use std::sync::Arc;
use winone_desktop::{protocol::RpcError,transport::Client};
use windows_sys::Win32::Foundation::HWND;

#[derive(Deserialize,Debug)]
#[serde(deny_unknown_fields)]
pub struct Request{pub id:u64,pub method:String,pub args:Vec<Value>}
#[derive(Serialize)]
pub struct Reply{pub id:u64,#[serde(skip_serializing_if="Option::is_none")]pub result:Option<Value>,#[serde(skip_serializing_if="Option::is_none")]pub error:Option<String>}
impl Reply{pub fn new(id:u64,result:Result<Value,RpcError>)->Self{match result{Ok(value)=>Self{id,result:Some(value),error:None},Err(error)=>Self{id,result:None,error:Some(error.to_string())}}}}
impl Request{
    pub fn mutates(&self)->bool{
        match self.method.as_str(){
            // Account/configuration reads may start a short-lived worker, but
            // cannot lose a settings edit when the user closes this window.
            "Capture" if self.args.first().and_then(Value::as_str).is_some_and(|method|matches!(method,"ai.magpie.catalog"|"ai.magpie.search-key"|"ai.accounts.catalog"|"ai.accounts.list"|"ai.accounts.signin.get"|"ai.configuration.get"|"ai.settings.get"|"ai.usage.get"|"ai.accounts.quota"|"ai.gateway.get"|"ai.icons.read"|"ai.routing.open"|"ai.routing.close"))=>false,
            "SaveSettings"|"SetAdministratorPreference"|"RestartAsAdministrator"|"BeginShortcutRecording"|"EndShortcutRecording"|"ExportProfile"|"OpenCaptureWindow"=>true,
            "Chat"=>self.args.first().and_then(Value::as_str).is_some_and(|method|!matches!(method,"file.preview"|"terminal.poll"|"terminal.resize"|"review.turn"|"review.snapshot"|"review.diff"|"review.branches"|"review.commits"|"usage"|"project.git"|"automations.deleted"|"automations.project.read"|"automations.connections.get"|"automations.templates"|"automations.template.apply"|"automations.list"|"automations.get"|"automations.runs"|"automations.events"|"automations.runtime"|"automations.preview"|"catalog"|"settings.get"|"resources"|"files"|"state"|"poll"|"draft.get"|"release"|"detach")),
            "DirectoryLinks"=>self.args.first().and_then(Value::as_str).is_some_and(|method|matches!(method,"create"|"create.elevated"|"createBatch"|"createBatch.elevated"|"remove"|"forget"|"inspectBatch"|"scan"|"exportPlan"|"menu.set")),
            "Stroke"=>self.args.first().and_then(Value::as_str).is_some_and(|method|!matches!(method,"snapshot"|"settings.get"|"record.read"|"inspect.target"|"windows.list")),
            "WinTray"|"Modules"|"AlwaysOnTop"|"Selection"|"QuickAssistant"|"Capture"|"Profiles"=>self.args.first().and_then(Value::as_str).is_some_and(|method|!matches!(method,"snapshot"|"catalog"|"settings.get"|"jobs.get"|"packagePreview.start"|"packagePreview.get"|"packagePreview.cancel"|"icons"|"privilege.status"|"import.preview"|"resetWinTray.preview"|"filename.preview"|"draft.get"|"selection.candidates"|"edit.preview")),
            _=>false,
        }
    }
    pub fn parse(text:&str)->Result<Self,RpcError>{
        if text.len()>8*1024*1024{return Err(invalid());}
        let request:Self=serde_json::from_str(text)?;
        if request.id==0||request.id>9_007_199_254_740_991||request.method.len()>64||request.args.len()>5{return Err(invalid());}
        Ok(request)
    }
    pub fn text(&self,index:usize)->Result<&str,RpcError>{self.args.get(index).and_then(Value::as_str).ok_or_else(invalid)}
    pub fn boolean(&self,index:usize)->Result<bool,RpcError>{self.args.get(index).and_then(Value::as_bool).ok_or_else(invalid)}
    pub fn value(&self,index:usize)->Result<Value,RpcError>{self.args.get(index).cloned().ok_or_else(invalid)}
}
fn invalid()->RpcError{RpcError::new("invalid","桌面请求格式无效")}
pub fn text<'a>(locale:&str,zh:&'a str,en:&'a str)->&'a str{if locale=="en-US"{en}else{zh}}

pub fn module_method(namespace:&str,method:&str)->Result<String,RpcError>{
    let allowed=match namespace{
        "wintray"=>matches!(method,"snapshot"|"settings.get"|"settings.save"|"launch"|"jobs.get"|"icons"|"icon.activate"|"recover"|"import.preview"|"startup.retry"),
        "modules"=>matches!(method,"snapshot"|"catalog"|"setEnabled"|"jobs.get"|"recoverConfiguration"|"recoverPackage"|"packagePreview.start"|"packagePreview.get"|"packagePreview.cancel"|"packageRemoval.preview"|"installPackage"|"updatePackage"|"removePackage"|"cleanupPackages"|"resetWinTray.preview"|"resetWinTray"),
        "always-on-top"=>matches!(method,"snapshot"|"settings.get"|"settings.save"|"window.set"|"clear"|"privilege.start"|"privilege.status"),
        "quick-assistant"=>matches!(method,"snapshot"|"settings.get"|"settings.save"|"show"),
        "selection"=>matches!(method,"snapshot"|"settings.get"|"settings.save"),
        "directory-links"=>matches!(method,"browse"|"open"|"pending"|"menu.get"|"menu.set"|"scan.cancel"|"reveal"|"preview"|"previewBatch"|"create"|"create.elevated"|"createBatch"|"createBatch.elevated"|"list"|"inspect"|"inspectBatch"|"scan"|"remove"|"forget"|"exportPlan"),
        "stroke"=>matches!(method,"inspect.target"|"windows.list"|"snapshot"|"settings.get"|"settings.save"|"record.start"|"record.read"|"record.cancel"),
        "chat"=>matches!(method,"file.preview"|"file.save"|"file.create"|"file.open"|"file.reveal"|"terminal.create"|"terminal.poll"|"terminal.write"|"terminal.resize"|"terminal.dispose"|"terminal.restart"|"review.turn.apply"|"review.turn"|"review.snapshot"|"review.diff"|"review.branches"|"review.commits"|"review.mutate"|"title.generate"|"title.regenerate"|"project.permission"|"link.open"|"usage"|"project.git"|"automations.deleted"|"automations.project.read"|"automations.connections.get"|"automations.templates"|"automations.template.apply"|"automations.list"|"automations.get"|"automations.restore"|"automations.project.write"|"automations.project.reconcile"|"automations.project.promote"|"automations.project.clone"|"automations.project.demote"|"automations.silence"|"automations.notify"|"automations.connections.save"|"automations.template.save"|"automations.template.delete"|"automations.create"|"automations.update"|"automations.pause"|"automations.resume"|"automations.delete"|"automations.run"|"automations.runs"|"automations.deleteRun"|"automations.markRead"|"automations.runtime"|"automations.preview"|"automations.events"|"catalog"|"settings.get"|"settings.save"|"resources"|"files"|"attachments.add"|"automation.run"|"project.add"|"project.create"|"project.update"|"project.remove"|"export"|"conversation.new"|"conversation.update"|"conversation.move"|"conversation.delete"|"configure"|"location"|"fork"|"draft.get"|"draft.save"|"open"|"poll"|"prompt"|"edit"|"retry"|"state"|"permission"|"stop"|"compact"|"release"|"detach"),
        "capture"=>matches!(method,"ai.magpie.catalog"|"ai.magpie.search-key"|"ai.models.test"|"ai.icons.store"|"ai.icons.read"|"ai.icons.fetch"|"ai.icons.website"|"ai.icons.cancel"|"ai.usage.get"|"ai.usage.export"|"ai.accounts.quota"|"ai.gateway.get"|"ai.gateway.save"|"ai.gateway.key"|"ai.models.fetch"|"ai.accounts.catalog"|"ai.accounts.list"|"ai.accounts.models"|"ai.accounts.signin.start"|"ai.accounts.signin.get"|"ai.accounts.signin.cancel"|"ai.accounts.signin.submit"|"ai.accounts.signin.open"|"ai.settings.get"|"ai.settings.save"|"ai.configuration.get"|"ai.configuration.save"|"ai.routing.open"|"ai.routing.close"|"snapshot"|"settings.get"|"settings.save"|"filename.preview"|"capture.start"|"capture.unified"|"scroll.stop"|"scroll.retake"|"handoff.release"|"draft.begin"|"draft.get"|"draft.update"|"draft.cancel"|"draft.complete"|"draft.confirm"|"selection.candidates"|"selection.commit"|"draft.copy"|"draft.pin"|"edit.last"|"edit.apply"|"edit.preview"|"copy.last"|"save.last"|"pin.last"|"pins.close"|"jobs.get"),
        _=>false,
    };
    if !allowed{return Err(RpcError::new("unsupported","不支持的模块操作"));}Ok(format!("{namespace}.{method}"))
}

pub fn execute(request:&Request,client:Arc<Client>,directory:&str,pipe:&str,owner:HWND,locale:&str)->Result<Value,RpcError>{
    match request.method.as_str(){
        "Snapshot"=>client.call("snapshot",Value::Null),
        "Chat" if request.text(0)?=="export"=>crate::chat_export::export(&client,serde_json::from_str(request.text(1)?)?,owner,locale),
        "Chat"=>Ok(json!(client.call(&module_method("chat",request.text(0)?)?,serde_json::from_str(request.text(1)?)?)?.to_string())),
        "DirectoryLinks"|"Stroke"=>{let namespace=if request.method=="DirectoryLinks"{"directory-links"}else{"stroke"};Ok(json!(client.call_completing(&module_method(namespace,request.text(0)?)?,serde_json::from_str(request.text(1)?)?)?.to_string()))},
        "SaveSettings"=>client.call_completing("settings.save",request.value(0)?),
        "WinTray"|"Modules"|"AlwaysOnTop"|"Selection"|"QuickAssistant"=>{
            let namespace=match request.method.as_str(){"WinTray"=>"wintray","Modules"=>"modules","Selection"=>"selection","QuickAssistant"=>"quick-assistant",_=>"always-on-top"};
            let method=module_method(namespace,request.text(0)?)?;
            let params=serde_json::from_str(request.text(1)?)?;
            Ok(json!(client.call_completing(&method,params)?.to_string()))
        },
        "Shortcuts"=>Ok(json!(client.call("shortcuts.snapshot",Value::Null)?.to_string())),
        "Residency"=>Ok(json!(client.call("residency.snapshot",Value::Null)?.to_string())),
        "BeginShortcutRecording"=>{let value=client.call("shortcuts.record.start",json!({"processId":std::process::id()}))?;Ok(value["token"].clone())},
        "EndShortcutRecording"=>{client.call("shortcuts.record.stop",json!({"token":request.text(0)?}))?;Ok(Value::Null)},
        "ReadModuleLogs"=>crate::diagnostics::read(directory,request.text(0)?),
        "OpenModuleLogFolder"=>{
            let folder=crate::diagnostics::folder(directory,request.text(0)?)?;
            crate::dialogs::open_folder(owner,folder["path"].as_str().ok_or_else(invalid)?).map_err(|e|RpcError::new("folder",e))?;Ok(Value::Null)
        },
        "ProgramInfo"=>crate::component_program_info::read(directory,request.text(0)?),
        "SetAdministratorPreference"=>Ok(crate::permissions::run(client,owner,pipe,directory,crate::permissions::Action::Preference(request.boolean(0)?))?["settings"].clone()),
        "RestartAsAdministrator"=>Ok(crate::permissions::run(client,owner,pipe,directory,crate::permissions::Action::Restart)?["closing"].clone()),
        "UpdateInfo"=>client.call("updates.info",Value::Null),
        "Capture"=>crate::component_capture::call(request,&client,owner,locale),
        "OpenCaptureWindow"=>crate::component_capture::open_window(&client,request.text(0)?),
        "CheckForUpdates"=>{
            let id=request.text(0)?;let mut value=client.call("updates.check",json!({"id":id}))?;
            let deadline=std::time::Instant::now()+std::time::Duration::from_secs(60);
            while value["state"]=="checking"{
                if client.cancellation_requested(){return Err(RpcError::new("cancelled","检查已取消"));}
                if std::time::Instant::now()>=deadline{client.call("updates.cancel",json!({"id":id}))?;return Err(RpcError::new("timeout","检查更新超时"));}
                std::thread::sleep(std::time::Duration::from_millis(100));value=client.call("updates.status",json!({"id":id}))?;
                if value["id"]!=id{return Err(RpcError::new("protocol","更新检查标识不匹配"));}
            }Ok(value)
        },
        "CancelUpdateCheck"=>{client.call("updates.cancel",json!({"id":request.text(0)?}))?;Ok(Value::Null)},
        "OpenUpdateRelease"=>{
            let value=client.call("updates.release",json!({"id":request.text(0)?}))?;
            crate::dialogs::open_update_release(owner,value["url"].as_str().ok_or_else(invalid)?).map_err(|e|RpcError::new("update",e))?;Ok(Value::Null)
        },
        "ExportProfile"=>{
            let Some(path)=crate::dialogs::save_profile_labeled(owner,text(locale,"导出配置","Export configuration"),text(locale,"WinOne 配置","WinOne configuration")).map_err(|e|RpcError::new("dialog",e))?else{return Ok(json!(""));};
            let value=client.call("profiles.export",json!({"path":path,"name":text(locale,"我的配置","My configuration")}))?;Ok(value["path"].clone())
        },
        "Profiles"=>{
            let method=request.text(0)?;let params:Value=serde_json::from_str(request.text(1)?)?;
            let value=match method{
                "import.preview"=>crate::profile_transfer::receive(&client,params["path"].as_str().ok_or_else(invalid)?,&params["bindings"]),
                "import.apply"=>client.call_completing("profiles.import.apply",params),
                _=>Err(RpcError::new("invalid","不支持的配置操作")),
            }?;Ok(json!(value.to_string()))
        },
        _=>Err(RpcError::new("unsupported",format!("此桌面操作尚未接通：{}",request.method))),
    }
}

#[cfg(test)]mod tests{
    use super::*;
    #[test]fn retry_reaches_the_host_and_keeps_close_protection(){
        assert_eq!(module_method("chat","retry").unwrap(),"chat.retry");
        assert_eq!(module_method("chat","edit").unwrap(),"chat.edit");
        let request=Request{id:1,method:"Chat".into(),args:vec![json!("retry"),json!("{}")]};
        assert!(request.mutates());
    }
    #[test]fn side_panel_actions_cross_the_desktop_bridge(){
        for method in ["file.preview","file.save","file.create","file.open","file.reveal","terminal.create","terminal.poll","terminal.write","terminal.resize","terminal.dispose","terminal.restart","review.turn.apply","review.turn","review.snapshot","review.diff","review.branches","review.commits","review.mutate","title.generate","title.regenerate","project.permission","link.open"]{
            assert_eq!(module_method("chat",method).unwrap(),format!("chat.{method}"));
        }
        assert!(module_method("chat","terminal.deleteHost").is_err());
    }
    #[test]fn magpie_settings_reads_and_connection_tests_reach_the_host(){
        for method in ["ai.magpie.catalog","ai.magpie.search-key","ai.models.test"]{
            assert_eq!(module_method("capture",method).unwrap(),format!("capture.{method}"));
        }
        for method in ["ai.magpie.catalog","ai.magpie.search-key"]{
            let request=Request{id:1,method:"Capture".into(),args:vec![json!(method),json!("{}")]};
            assert!(!request.mutates(),"reading settings must not prevent closing the window");
        }
    }
    #[test]fn folder_browsing_uses_the_real_host_as_a_read_only_operation(){
        assert_eq!(module_method("directory-links","browse").unwrap(),"directory-links.browse");
        let browse=Request{id:1,method:"DirectoryLinks".into(),args:vec![json!("browse"),json!("{\"directory\":\"C:\\\\source\"}")]};
        assert!(!browse.mutates(),"a read must not acquire mutation/close protection");
        let create=Request{id:2,method:"DirectoryLinks".into(),args:vec![json!("createBatch"),json!("{}")]};
        assert!(create.mutates(),"actual creation must retain transaction protection");
    }
    #[test]fn account_and_configuration_reads_do_not_hold_the_window_open(){
        for method in ["ai.accounts.catalog","ai.accounts.list","ai.accounts.signin.get","ai.configuration.get","ai.settings.get","ai.usage.get","ai.gateway.get","ai.routing.open","ai.routing.close"]{
            let request=Request{id:1,method:"Capture".into(),args:vec![json!(method),json!("{}")]};
            assert!(!request.mutates(),"{method} must not guard window close");
        }
        for method in ["ai.configuration.save","ai.gateway.save","ai.accounts.signin.start","ai.accounts.signin.submit"]{
            let request=Request{id:1,method:"Capture".into(),args:vec![json!(method),json!("{}")]};
            assert!(request.mutates(),"{method} must retain transaction protection");
        }
    }
    #[test]fn bridge_rejects_ambiguous_requests_and_module_escape(){
        assert!(Request::parse(r#"{"id":1,"method":"Snapshot","args":[],"extra":true}"#).is_err());
        assert!(Request::parse(r#"{"id":0,"method":"Snapshot","args":[]}"#).is_err());
        assert!(module_method("wintray","application.restart.commit").is_err());
        assert_eq!(module_method("wintray","settings.save").unwrap(),"wintray.settings.save");
    }
    #[test]fn bridge_keeps_value_and_error_results_distinct(){
        assert_eq!(serde_json::to_value(Reply::new(1,Ok(json!("{}")))).unwrap(),json!({"id":1,"result":"{}"}));
        assert!(serde_json::to_value(Reply::new(2,Err(invalid()))).unwrap().get("result").is_none());
    }
}
