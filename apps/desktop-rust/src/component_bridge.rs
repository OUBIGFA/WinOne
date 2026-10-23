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
            "Capture" if self.args.first().and_then(Value::as_str).is_some_and(|method|matches!(method,"ai.magpie.catalog"|"ai.magpie.search-key"|"ai.accounts.catalog"|"ai.accounts.list"|"ai.accounts.quota"|"ai.accounts.checkin"|"ai.accounts.signin.get"|"ai.configuration.get"|"ai.keys.reveal"|"ai.settings.get"|"ai.usage.get"|"ai.gateway.get"|"ai.icons.read"|"ai.routing.open"|"ai.routing.close"|"ai.routing.plugins.open"|"ai.routing.plugins.close"|"ai.routing.extensions.open"|"ai.routing.extensions.close"|"ai.magpie.privacy.open"|"ai.magpie.privacy.state"|"ai.magpie.privacy.close"))=>false,
            "ChatMedia"|"SaveSettings"|"SetAdministratorPreference"|"RestartAsAdministrator"|"BeginShortcutRecording"|"EndShortcutRecording"|"ExportProfile"|"OpenCaptureWindow"=>true,
            "Chat"=>self.args.first().and_then(Value::as_str).is_some_and(|method|!matches!(method,"file.resolve"|"file.stat"|"files.list"|"files.index"|"files.search"|"files.cancel"|"file.preview"|"media.source"|"media.upload"|"terminal.poll"|"terminal.resize"|"review.turn"|"review.snapshot"|"review.diff"|"review.diff.preview"|"review.file.preview"|"review.metadata"|"review.branches"|"review.commits"|"usage"|"project.git"|"automations.deleted"|"automations.project.read"|"automations.connections.get"|"automations.templates"|"automations.template.apply"|"automations.list"|"automations.get"|"automations.runs"|"automations.events"|"automations.runtime"|"automations.preview"|"catalog"|"activity"|"settings.get"|"resources"|"files"|"state"|"poll"|"draft.get"|"release"|"detach")),
            "DirectoryLinks"=>self.args.first().and_then(Value::as_str).is_some_and(|method|matches!(method,"create"|"create.elevated"|"createBatch"|"createBatch.elevated"|"rename"|"remove"|"forget"|"forgetInvalid"|"inspectBatch"|"scan"|"exportPlan"|"menu.set")),
            "Stroke"=>self.args.first().and_then(Value::as_str).is_some_and(|method|!matches!(method,"snapshot"|"settings.get"|"record.read"|"inspect.target"|"windows.list")),
            "WinTray" if self.args.first().and_then(Value::as_str)==Some("program.state")=>false,
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

// The clicked desktop owns the Windows foreground right. The Host returns an
// already verified child HWND; recheck its PID before restoring/activating it.
// No temporary topmost flag, synthetic input, or attached input queues.
fn activate_quick_assistant(value:&Value)->Result<(),RpcError>{
    use windows_sys::Win32::UI::WindowsAndMessaging::*;
    let failure=||RpcError::new("quick_assistant","快捷助手窗口已关闭或无法激活，请再次点击打开");
    let window=value["window"].as_str().and_then(|s|s.parse::<usize>().ok()).filter(|w|*w!=0).ok_or_else(failure)? as HWND;
    let pid=value["processId"].as_u64().and_then(|p|u32::try_from(p).ok()).filter(|p|*p!=0).ok_or_else(failure)?;
    unsafe{
        let mut actual=0;GetWindowThreadProcessId(window,&mut actual);
        if actual!=pid{return Err(failure());}
        AllowSetForegroundWindow(pid);
        if IsIconic(window)!=0{ShowWindow(window,SW_RESTORE);}
        SetForegroundWindow(window);
        if GetForegroundWindow()!=window{return Err(failure());}
    }
    Ok(())
}

pub fn text<'a>(locale:&str,zh:&'a str,en:&'a str)->&'a str{if locale=="en-US"{en}else{zh}}

pub fn module_method(namespace:&str,method:&str)->Result<String,RpcError>{
    let allowed=match namespace{
        "wintray"=>matches!(method,"program.state"|"program.window"|"program.stop"|"snapshot"|"settings.get"|"settings.save"|"launch"|"jobs.get"|"icons"|"icon.activate"|"recover"|"import.preview"|"startup.retry"),
        "modules"=>matches!(method,"snapshot"|"catalog"|"setEnabled"|"jobs.get"|"recoverConfiguration"|"recoverPackage"|"packagePreview.start"|"packagePreview.get"|"packagePreview.cancel"|"packageRemoval.preview"|"installPackage"|"updatePackage"|"removePackage"|"cleanupPackages"|"resetWinTray.preview"|"resetWinTray"),
        "always-on-top"=>matches!(method,"snapshot"|"settings.get"|"settings.save"|"window.set"|"clear"|"privilege.start"|"privilege.status"),
        "quick-assistant"=>matches!(method,"snapshot"|"settings.get"|"settings.save"|"show"),
        "selection"=>matches!(method,"snapshot"|"settings.get"|"settings.save"),
        "directory-links"=>matches!(method,"browse"|"open"|"pending"|"menu.get"|"menu.set"|"scan.cancel"|"reveal"|"preview"|"previewBatch"|"create"|"create.elevated"|"createBatch"|"createBatch.elevated"|"rename"|"list"|"inspect"|"inspectBatch"|"scan"|"remove"|"forget"|"forgetInvalid"|"exportPlan"),
        "stroke"=>matches!(method,"inspect.target"|"windows.list"|"snapshot"|"settings.get"|"settings.save"|"record.start"|"record.read"|"record.cancel"),
        "chat"=>matches!(method,"file.resolve"|"file.stat"|"files.list"|"files.index"|"files.search"|"files.cancel"|"thinking"|"fast"|"agents"|"file.preview"|"media.source"|"media.upload"|"media.attach"|"file.save"|"file.create"|"file.open"|"file.reveal"|"terminal.create"|"terminal.poll"|"terminal.write"|"terminal.resize"|"terminal.dispose"|"terminal.restart"|"review.turn.apply"|"review.turn"|"review.snapshot"|"review.diff"|"review.diff.preview"|"review.file.preview"|"review.metadata"|"review.branches"|"review.commits"|"review.mutate"|"review.push"|"title.generate"|"title.regenerate"|"project.permission"|"link.open"|"usage"|"project.git"|"automations.deleted"|"automations.project.read"|"automations.connections.get"|"automations.templates"|"automations.template.apply"|"automations.list"|"automations.get"|"automations.restore"|"automations.project.write"|"automations.project.reconcile"|"automations.project.promote"|"automations.project.clone"|"automations.project.demote"|"automations.silence"|"automations.notify"|"automations.connections.save"|"automations.template.save"|"automations.template.delete"|"automations.create"|"automations.update"|"automations.pause"|"automations.resume"|"automations.delete"|"automations.run"|"automations.runs"|"automations.deleteRun"|"automations.markRead"|"automations.runtime"|"automations.preview"|"automations.events"|"catalog"|"activity"|"settings.get"|"settings.save"|"resources"|"files"|"attachments.add"|"attachments.upload"|"automation.run"|"project.add"|"project.create"|"project.update"|"project.remove"|"export"|"conversation.new"|"conversation.update"|"conversation.move"|"conversation.delete"|"configure"|"location"|"fork"|"draft.get"|"draft.save"|"open"|"poll"|"prompt"|"edit"|"retry"|"state"|"permission"|"question"|"stop"|"pause"|"resume"|"compact"|"release"|"detach"),
        "capture"=>matches!(method,"ai.magpie.catalog"|"ai.magpie.search-key"|"ai.models.test"|"ai.icons.store"|"ai.icons.read"|"ai.icons.fetch"|"ai.icons.website"|"ai.icons.cancel"|"ai.usage.get"|"ai.usage.export"|"ai.gateway.get"|"ai.gateway.save"|"ai.gateway.key"|"ai.models.fetch"|"ai.accounts.catalog"|"ai.accounts.list"|"ai.accounts.models"|"ai.accounts.signin.start"|"ai.accounts.signin.get"|"ai.accounts.signin.cancel"|"ai.accounts.signin.submit"|"ai.accounts.signin.open"|"ai.settings.get"|"ai.settings.save"|"ai.configuration.get"|"ai.keys.reveal"|"ai.configuration.save"|"ai.routing.open"|"ai.routing.close"|"ai.routing.plugins.open"|"ai.routing.plugins.close"|"ai.routing.extensions.open"|"ai.routing.extensions.close"|"ai.magpie.privacy.open"|"ai.magpie.privacy.state"|"ai.magpie.privacy.action"|"ai.magpie.privacy.close"|"snapshot"|"settings.get"|"settings.save"|"filename.preview"|"capture.start"|"capture.unified"|"scroll.stop"|"scroll.retake"|"handoff.release"|"draft.begin"|"draft.get"|"draft.update"|"draft.cancel"|"draft.complete"|"draft.confirm"|"selection.candidates"|"selection.commit"|"draft.copy"|"draft.pin"|"edit.last"|"edit.apply"|"edit.preview"|"copy.last"|"save.last"|"pin.last"|"pins.close"|"jobs.get"),
        _=>false,
    };
    if !allowed{return Err(RpcError::new("unsupported","不支持的模块操作"));}Ok(format!("{namespace}.{method}"))
}

pub fn execute(request:&Request,client:Arc<Client>,directory:&str,pipe:&str,owner:HWND,locale:&str)->Result<Value,RpcError>{
    match request.method.as_str(){
        "ReadClipboardFiles"=>crate::browser_capture::read_clipboard_files(owner).map(|paths|json!(paths)).map_err(|error|RpcError::new("clipboard",error)),
        "Snapshot"=>client.call("snapshot",Value::Null),
        "Chat" if request.text(0)?=="export"=>crate::chat_export::export(&client,serde_json::from_str(request.text(1)?)?,owner,locale),
        "Chat"=>Ok(json!(client.call(&module_method("chat",request.text(0)?)?,serde_json::from_str(request.text(1)?)?)?.to_string())),
        "DirectoryLinks"|"Stroke"=>{let namespace=if request.method=="DirectoryLinks"{"directory-links"}else{"stroke"};Ok(json!(client.call_completing(&module_method(namespace,request.text(0)?)?,serde_json::from_str(request.text(1)?)?)?.to_string()))},
        "SaveSettings"=>client.call_completing("settings.save",request.value(0)?),
        "ChatMedia"=>{
            let method=request.text(0)?;
            if !matches!(method,"copy"|"copy-file"|"save-as"){return Err(RpcError::new("media","不支持的文件操作"));}
            let params:Value=serde_json::from_str(request.text(1)?)?;
            if method=="copy-file"{
                let file=client.call("chat.file.resolve",params)?;
                let path=file["path"].as_str().ok_or_else(invalid)?;
                crate::browser_capture::copy_file(owner,path).map_err(|error|RpcError::new("media",error))?;
                return Ok(json!(json!({"success":true}).to_string()));
            }
            // The Host resolves the saved conversation's file boundary. The
            // native action receives only a verified image, never an arbitrary
            // URL or executable; bytes stay outside the 1 MB JSON pipe.
            let file=client.call("chat.file.preview",params.clone())?;
            if file["kind"]!="image"{return Err(RpcError::new("media","内容不是图片"));}
            let path=file["resolvedPath"].as_str().or_else(||file["path"].as_str()).ok_or_else(invalid)?;
            if method=="copy"{
                let image_file=std::fs::File::open(path).map_err(|e|RpcError::new("media",e.to_string()))?;
                let mut reader=std::io::Read::take(image_file,72*1024*1024+1);let mut data=Vec::new();
                std::io::Read::read_to_end(&mut reader,&mut data).map_err(|e|RpcError::new("media",e.to_string()))?;
                if data.len()<24||data.len()>72*1024*1024{return Err(RpcError::new("media","PNG 导出大小无效"));}
                let width=u32::from_be_bytes(data[16..20].try_into().unwrap())as u64;let height=u32::from_be_bytes(data[20..24].try_into().unwrap())as u64;
                if width>16384||height>16384||width*height>4096*4096+16384{return Err(RpcError::new("media","PNG 导出尺寸过大"));}
                let original=if let Some(path)=params["originalPath"].as_str(){let value=client.call("chat.file.preview",json!({"id":params["id"],"path":path}))?;if value["kind"]!="image"{return Err(RpcError::new("media","原文件不是图片"));}Some(value["path"].as_str().ok_or_else(invalid)?.to_string())}else{None};
                crate::browser_capture::copy_png_with_extras(owner,&data,params["plainText"].as_str(),original.as_deref()).map_err(|e|RpcError::new("media",e))?;
                Ok(json!(json!({"success":true}).to_string()))
            }else{
                let name=file["name"].as_str().unwrap_or("image.png");
                let extension=std::path::Path::new(name).extension().and_then(|x|x.to_str()).unwrap_or("png");
                let filter=format!("Image\0*.{}\0",extension);
                let title=if locale.starts_with("zh"){ "另存图片" }else{ "Save image as" };
                let selected=crate::dialogs::save_file(owner,title,&filter,extension,name).map_err(|e|RpcError::new("media",e))?;
                if let Some(target)=selected{std::fs::copy(path,&target).map_err(|e|RpcError::new("media",e.to_string()))?;Ok(json!(json!({"canceled":false,"path":target}).to_string()))}
                else{Ok(json!(json!({"canceled":true}).to_string()))}
            }
        },
        "WinTray"|"Modules"|"AlwaysOnTop"|"Selection"|"QuickAssistant"=>{
            let namespace=match request.method.as_str(){"WinTray"=>"wintray","Modules"=>"modules","Selection"=>"selection","QuickAssistant"=>"quick-assistant",_=>"always-on-top"};
            let method=module_method(namespace,request.text(0)?)?;
            let params=serde_json::from_str(request.text(1)?)?;
            let result=client.call_completing(&method,params)?;
            if method=="quick-assistant.show"{activate_quick_assistant(&result)?;}
            Ok(json!(result.to_string()))
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
        "ReadDroppedImage"=>{
            // Match the avatar picker's 1 MiB bound. Format/content validation
            // remains in the existing avatar store rather than this transport.
            use std::io::Read;
            let path=request.text(0)?;
            let extension=std::path::Path::new(path).extension().and_then(|value|value.to_str()).unwrap_or("").to_ascii_lowercase();
            if !matches!(extension.as_str(),"png"|"jpg"|"jpeg"|"gif"|"webp"|"ico"|"svg"){return Err(RpcError::new("image",text(locale,"请选择支持的图片文件","Choose a supported image file")));}
            let mut bytes=Vec::new();std::fs::File::open(path)?.take((1<<20)+1).read_to_end(&mut bytes)?;
            if bytes.is_empty()||bytes.len()>1<<20{return Err(RpcError::new("image",text(locale,"请选择不超过 1 MB 的图片","Choose an image up to 1 MB")));}
            Ok(json!(crate::icon::base64_encode(&bytes)))
        },
        "InspectDroppedPaths"=>{
            let paths:Vec<String>=serde_json::from_value(request.value(0)?)?;
            if paths.is_empty()||paths.len()>1024{return Err(invalid());}
            // Resolve type on a worker before replacing a draft path. A failed
            // stat never turns a dropped file into an assumed directory.
            let mut entries=Vec::with_capacity(paths.len());
            for path in paths{
                if !std::path::Path::new(&path).is_absolute(){return Err(RpcError::new("path",text(locale,"请拖入完整的本地路径","Drop an absolute local path")));}
                let metadata=std::fs::metadata(&path).map_err(|error|RpcError::new("path",format!("{path}: {error}")))?;
                if !metadata.is_file()&&!metadata.is_dir(){return Err(invalid());}
                entries.push(json!({"path":path,"kind":if metadata.is_dir(){"directory"}else{"file"}}));
            }
            Ok(json!(entries))
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
    #[test]fn quick_activation_rejects_missing_stale_and_overflowing_targets(){
        for value in [json!({}),json!({"window":"0","processId":1}),json!({"window":"-1","processId":1}),json!({"window":"123","processId":4294967296u64}),json!({"window":"123","processId":1})]{
            assert!(activate_quick_assistant(&value).is_err());
        }
    }
    #[test]fn retry_reaches_the_host_and_keeps_close_protection(){
        assert_eq!(module_method("chat","retry").unwrap(),"chat.retry");
        assert_eq!(module_method("chat","edit").unwrap(),"chat.edit");
        let request=Request{id:1,method:"Chat".into(),args:vec![json!("retry"),json!("{}")]};
        assert!(request.mutates());
    }
    #[test]fn structured_questions_cross_the_bridge_as_explicit_user_responses(){
        assert_eq!(module_method("chat","question").unwrap(),"chat.question");
        let request=Request{id:1,method:"Chat".into(),args:vec![json!("question"),json!("{}") ]};
        assert!(request.mutates());
    }
    #[test]fn chat_activity_is_a_read_only_host_route(){
        // Sidebar polling must neither be rejected nor acquire mutation/close
        // protection while another conversation is executing a command.
        assert_eq!(module_method("chat","activity").unwrap(),"chat.activity");
        let request=Request{id:1,method:"Chat".into(),args:vec![json!("activity"),json!("{}")]};
        assert!(!request.mutates());
        assert!(module_method("chat","activity.reset").is_err());
    }
    #[test]fn side_panel_actions_cross_the_desktop_bridge(){
        for method in ["file.resolve","file.stat","files.list","files.index","files.search","files.cancel","thinking","fast","agents","file.preview","file.save","file.create","file.open","file.reveal","terminal.create","terminal.poll","terminal.write","terminal.resize","terminal.dispose","terminal.restart","review.turn.apply","review.turn","review.snapshot","review.diff","review.branches","review.commits","review.mutate","title.generate","title.regenerate","project.permission","link.open"]{
            assert_eq!(module_method("chat",method).unwrap(),format!("chat.{method}"));
        }
        assert!(module_method("chat","terminal.deleteHost").is_err());
    }
    #[test]fn provider_key_reveal_is_an_explicit_read_only_route(){
        assert_eq!(module_method("capture","ai.keys.reveal").unwrap(),"capture.ai.keys.reveal");
        let request=Request{id:1,method:"Capture".into(),args:vec![json!("ai.keys.reveal"),json!("{\"provider\":\"one\",\"id\":\"primary\"}")]};
        assert!(!request.mutates(),"viewing a saved key must not hold the window open");
        assert!(module_method("capture","ai.keys.export").is_err());
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
    #[test]fn entry_rename_reaches_the_host_with_close_protection(){
        // Rename is a real file mutation: admitting the route without the
        // transaction guard would let the window close before its result.
        assert_eq!(module_method("directory-links","rename").unwrap(),"directory-links.rename");
        let request=Request{id:3,method:"DirectoryLinks".into(),args:vec![json!("rename"),json!("{}")]};
        assert!(request.mutates());
    }
    #[test]fn account_and_configuration_reads_do_not_hold_the_window_open(){
        for method in ["ai.accounts.catalog","ai.accounts.list","ai.accounts.quota","ai.accounts.checkin","ai.accounts.signin.get","ai.configuration.get","ai.settings.get","ai.usage.get","ai.gateway.get","ai.routing.open","ai.routing.close"]{
            let request=Request{id:1,method:"Capture".into(),args:vec![json!(method),json!("{}")]};
            assert!(!request.mutates(),"{method} must not guard window close");
        }
        for method in ["ai.configuration.save","ai.gateway.save","ai.accounts.signin.start","ai.accounts.signin.submit"]{
            let request=Request{id:1,method:"Capture".into(),args:vec![json!(method),json!("{}")]};
            assert!(request.mutates(),"{method} must retain transaction protection");
        }
    }
    #[test]fn plugin_manager_viewers_use_the_host_without_holding_window_close(){
        for method in ["ai.routing.plugins.open","ai.routing.plugins.close","ai.routing.extensions.open","ai.routing.extensions.close"]{
            assert_eq!(module_method("capture",method).unwrap(),format!("capture.{method}"));
            let request=Request{id:1,method:"Capture".into(),args:vec![json!(method),json!("null")]};
            assert!(!request.mutates(),"opening a managed document must not guard window close");
        }
        assert!(module_method("capture","ai.routing.plugins.remove").is_err());
    }
    #[test]fn privacy_lifecycle_uses_the_host_and_guards_only_actions(){
        // Opening a viewer must neither download a model nor trap window close;
        // explicit enable/delete/cancel operations retain transaction protection.
        for method in ["ai.magpie.privacy.open","ai.magpie.privacy.state","ai.magpie.privacy.close","ai.magpie.privacy.action"]{
            assert_eq!(module_method("capture",method).unwrap(),format!("capture.{method}"));
            let request=Request{id:1,method:"Capture".into(),args:vec![json!(method),json!("{}") ]};
            assert_eq!(request.mutates(),method=="ai.magpie.privacy.action");
        }
        assert!(module_method("capture","ai.magpie.privacy.remove").is_err());
    }
    #[test]fn program_window_actions_keep_reads_and_mutations_separate(){
        for method in ["program.state","program.window","program.stop"] {
            assert_eq!(module_method("wintray",method).unwrap(),format!("wintray.{method}"));
            let request=Request{id:1,method:"WinTray".into(),args:vec![json!(method),json!("{}")]};
            assert_eq!(request.mutates(),method!="program.state");
        }
        assert!(module_method("wintray","program.kill-all").is_err());
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
