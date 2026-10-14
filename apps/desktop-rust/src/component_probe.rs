//! Installer startup check: real UI/assets, isolated read-only data, no workers.
use crate::{component_bridge::Request,component_ui};
use serde_json::{Value,json};
use std::{path::PathBuf,sync::{Arc,Mutex},io::Write};
use winone_desktop::protocol::RpcError;
use windows_sys::Win32::{Foundation::*,UI::WindowsAndMessaging::*};

pub const TIMER:usize=0x5511;
pub struct Probe{
    nonce:String,modules:Value,seen_snapshot:bool,seen_modules:bool,frame:bool,surface:bool,
    pub browser:Option<(u32,u64)>,pub completed:bool,previous:Option<RECT>,probe_size:Option<(i32,i32)>,
}
fn invalid()->RpcError{RpcError::new("probe","启动检查尚未就绪")}
impl Probe{
    pub fn request(&mut self,window:HWND,request:&Request)->Option<Result<Value,RpcError>>{
        Some((||match request.method.as_str(){
            "Snapshot"=>{self.seen_snapshot=true;Ok(json!({"locale":"zh-CN","administrator":false,"settings":{"schemaVersion":1,"revision":0,"theme":"light","language":"zh-CN","captureShortcut":"","pinShortcut":"","startup":{"runAtLogon":false,"startMinimized":true,"preferAdministrator":false}},"modules":self.modules.as_array().ok_or_else(invalid)?.iter().map(|m|json!({"id":m["id"],"name":m["name"]["zh-CN"],"state":"disabled","detail":""})).collect::<Vec<_>>()}))},
            "Modules" if request.text(0)?=="snapshot"=>{self.seen_modules=true;Ok(json!(json!({"revision":0,"modules":self.modules.as_array().ok_or_else(invalid)?.iter().map(|m|json!({"manifest":m,"enabled":false,"runtime":{"id":m["id"],"state":"disabled"}})).collect::<Vec<_>>()} ).to_string()))},
            "InitializeWindow"=>{self.frame=true;Ok(json!({"maximized":false}))},
            "StartupProbeNonce"=>Ok(json!(self.nonce)),
            "BeginCaptureSurfaceProbe"=>{
                if request.text(0)?!=self.nonce||!self.frame||self.previous.is_some(){return Err(invalid());}
                let mut previous=RECT::default();unsafe{if GetWindowRect(window,&mut previous)==0{return Err(std::io::Error::last_os_error().into());}}
                let (width,height)=unsafe{(GetSystemMetrics(SM_CXVIRTUALSCREEN),GetSystemMetrics(SM_CYVIRTUALSCREEN))};
                if width<=0||height<=0{return Err(invalid());}
                self.previous=Some(previous);self.probe_size=Some((width,height));
                unsafe{if SetWindowPos(window,std::ptr::null_mut(),0,0,width,height,SWP_NOZORDER|SWP_NOACTIVATE)==0{return Err(std::io::Error::last_os_error().into());}PostMessageW(window,WM_SIZE,0,0);}
                Ok(json!({"width":width,"height":height}))
            },
            "EndCaptureSurfaceProbe"=>{
                if request.text(0)?!=self.nonce{return Err(invalid());}
                let (width,height)=self.probe_size.ok_or_else(invalid)?;
                let number=|index|request.args.get(index).and_then(Value::as_f64).filter(|v|v.is_finite()&&*v>0.0).ok_or_else(invalid);
                let scale=number(3)?;
                let mut client=RECT::default();unsafe{GetClientRect(window,&mut client);}
                if (number(1)?*scale-width as f64).abs()>1.0||(number(2)?*scale-height as f64).abs()>1.0||client.right!=width||client.bottom!=height{return Err(invalid());}
                let previous=self.previous.take().ok_or_else(invalid)?;
                unsafe{if SetWindowPos(window,std::ptr::null_mut(),previous.left,previous.top,previous.right-previous.left,previous.bottom-previous.top,SWP_NOZORDER|SWP_NOACTIVATE)==0{return Err(std::io::Error::last_os_error().into());}PostMessageW(window,WM_SIZE,0,0);}
                let mut restored=RECT::default();unsafe{GetWindowRect(window,&mut restored);}
                if (restored.left,restored.top,restored.right,restored.bottom)!=(previous.left,previous.top,previous.right,previous.bottom){return Err(invalid());}
                self.surface=true;Ok(Value::Null)
            },
            "CompleteStartupProbe"=>{
                if request.text(0)?!=self.nonce||!self.seen_snapshot||!self.seen_modules||!self.frame||!self.surface{return Err(invalid());}
                self.completed=true;unsafe{PostMessageW(window,WM_CLOSE,0,0);}Ok(Value::Null)
            },
            "WindowFrameState"|"SettingsNavigation"|"SetSettingsActivity"|"SettingsClosePending"|"CancelSettingsClose"|"ApproveSettingsClose"=>return Err(RpcError::new("delegate","")),
            _=>Err(RpcError::new("probe","启动检查不执行产品操作")),
        })()).filter(|r|!r.as_ref().is_err_and(|e|e.code=="delegate"))
    }
}

pub fn requested(args:&[String])->bool{args.iter().any(|arg|arg.starts_with("--check-ui")||arg.starts_with("--probe-root"))}

// Keep the read-only UI probe aligned with the same catalog used by the Host.
// A stale hard-coded module count rejected the added selection assistant.
fn valid_modules(value:&Value)->bool {
    let expected:Value=serde_json::from_str(include_str!("../../../internal/catalog/builtin.json")).expect("bundled module catalog");
    let Some(items)=value.as_array() else{return false};
    let expected=expected.as_array().expect("module catalog array");
    items.len()==expected.len()&&expected.iter().all(|entry|items.iter().filter(|item|item["id"]==entry["id"]).count()==1)
}
pub fn run(args:&[String])->Result<(),String>{
    if args.len()!=4||args[0]!="--check-ui"||args[2]!="--probe-root"||args[1].is_empty()||args[1].len()>128{return Err("启动检查参数无效".into());}
    let executable=std::env::current_exe().map_err(|e|e.to_string())?;
    let scratch=PathBuf::from(&args[3]);if !scratch.is_absolute(){return Err("启动检查需要绝对临时路径".into());}
    let scratch=scratch.canonicalize().map_err(|e|e.to_string())?;
    let installation=executable.parent().ok_or("程序路径无效")?.canonicalize().map_err(|e|e.to_string())?;
    let installed=installation.to_string_lossy().to_lowercase();let temporary=scratch.to_string_lossy().to_lowercase();
    if !scratch.is_dir()||temporary==installed||temporary.starts_with(&(installed+"\\")){return Err("启动检查临时目录必须在安装目录外".into());}
    // The existing Host probe returns the shared catalog without opening a
    // profile or starting module workers. Bind its report to the same nonce.
    let output=crate::startup::command()?.args(["--check-startup",&args[1]]).output().map_err(|e|e.to_string())?;
    if !output.status.success()||output.stdout.len()>32768{return Err("后台启动自检失败".into());}
    let report:Value=serde_json::from_slice(&output.stdout).map_err(|e|e.to_string())?;
    if report["protocol"]!=1||report["nonce"]!=args[1]||!valid_modules(&report["modules"]){return Err("后台启动检查答复无效".into());}
    let directory=scratch.join(format!("desktop-probe-{}",crate::dialogs::new_program_id()?));std::fs::create_dir(&directory).map_err(|e|e.to_string())?;
    let modules=report["modules"].clone();
    let probe=Arc::new(Mutex::new(Probe{browser:None,nonce:args[1].clone(),modules:modules.clone(),seen_snapshot:false,seen_modules:false,frame:false,surface:false,completed:false,previous:None,probe_size:None}));
    component_ui::run_probe(&directory.to_string_lossy(),probe.clone())?;
    if !probe.lock().map_err(|_|"启动检查状态无效")?.completed{return Err("桌面启动检查未完成".into());}
    let browser=probe.lock().map_err(|_|"启动检查状态无效")?.browser.ok_or("缺少浏览器进程身份")?;
    let result=json!({"browserProcessId":browser.0,"browserCreated":browser.1,"protocol":1,"nonce":args[1],"pid":std::process::id(),"executable":executable,"modules":modules,"uiReady":true});
    std::io::stdout().write_all(result.to_string().as_bytes()).map_err(|e|e.to_string())?;Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn startup_catalog_accepts_all_builtins_and_rejects_missing_or_duplicate_entries(){
        let mut modules:Value=serde_json::from_str(include_str!("../../../internal/catalog/builtin.json")).unwrap();
        assert!(valid_modules(&modules));
        let removed=modules.as_array_mut().unwrap().pop().unwrap();
        assert!(!valid_modules(&modules));
        let duplicate=modules[0].clone();modules.as_array_mut().unwrap().push(duplicate);
        assert!(!valid_modules(&modules));
        modules.as_array_mut().unwrap().pop();modules.as_array_mut().unwrap().push(removed);
        assert!(valid_modules(&modules));
    }
}
