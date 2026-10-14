//! Short-lived Go consent helper. Host calls stay in this Rust process so the
//! restart owner is the real settings process. No elevated command is supplied by UI data.
use serde_json::{Value,json};
use std::{io::{Read,Write},os::windows::io::AsRawHandle,process::{Command,Stdio},sync::Arc,time::{Duration,Instant}};
use winone_desktop::{protocol::{RpcError,read_frame,write_frame},transport::Client};
use windows_sys::Win32::{Foundation::HWND,System::IO::CancelSynchronousIo,UI::WindowsAndMessaging::*};

pub const GUARD_MESSAGE:u32=WM_APP+5;
#[derive(Clone,Copy,Debug,PartialEq,Eq)]
pub enum Action{Preference(bool),Restart}

struct HelperProcess(std::process::Child);
impl std::ops::Deref for HelperProcess{type Target=std::process::Child;fn deref(&self)->&Self::Target{&self.0}}
impl std::ops::DerefMut for HelperProcess{fn deref_mut(&mut self)->&mut Self::Target{&mut self.0}}
impl Drop for HelperProcess{
    fn drop(&mut self){
        // Thread creation and process inspection can also fail. Those paths
        // still own this helper and must not leave it waiting on abandoned input.
        if !matches!(self.0.try_wait(),Ok(Some(_))){
            if let Err(error)=self.0.kill(){eprintln!("permission helper cleanup: {error}");}
            else{let _=self.0.wait();}
        }
    }
}

fn protocol_error()->RpcError{RpcError::new("protocol","无法确认权限操作结果，请重新读取设置。")}
fn token(value:&Value)->Option<&str>{value.as_str().filter(|s|s.len()==32&&s.bytes().all(|c|c.is_ascii_hexdigit()))}
fn guard(window:HWND)->bool{
    let deadline=Instant::now()+Duration::from_secs(1);
    loop{
        let mut result=0;
        if (unsafe{SendMessageTimeoutW(window,GUARD_MESSAGE,0,0,SMTO_ABORTIFHUNG|SMTO_BLOCK,1000,&mut result)})==0{return false;}
        if result!=2{return result==1;}
        // A sent message can reenter control creation. Busy is not a negative
        // edit decision: ask again once that bounded UI dispatch can unwind.
        if Instant::now()>=deadline{return false;}
        std::thread::yield_now();
    }
}

pub fn run(client:Arc<Client>,window:HWND,pipe:&str,directory:&str,action:Action)->Result<Value,RpcError>{
    let mut command=crate::startup::command().map_err(|e|RpcError::new("permission",e))?;
    command.arg("--desktop-control");
    let mut request=json!({"version":1,"operation":if matches!(action,Action::Restart){"restart"}else{"preference"},
        "pipe":pipe,"dataDirectory":directory,"ownerPID":std::process::id()});
    if let Action::Preference(enabled)=action{request["enabled"]=json!(enabled);}
    run_child(command,client,window,request,action,Duration::from_secs(300))
}

fn run_child(mut command:Command,client:Arc<Client>,window:HWND,request:Value,action:Action,limit:Duration)->Result<Value,RpcError>{
    if client.cancellation_requested(){return Err(RpcError::new("cancelled","权限操作已取消"));}
    command.stdin(Stdio::piped()).stdout(Stdio::piped()).stderr(Stdio::null());
    let mut child=HelperProcess(command.spawn()?);
    let mut input=child.stdin.take().expect("piped helper input");
    let mut output=child.stdout.take().expect("piped helper output");
    let caller=client.clone();let owner=window as usize;
    let io=std::thread::Builder::new().name("permission-io".into()).spawn(move||{
        write_frame(&mut input,&serde_json::to_vec(&request)?)?;
        exchange(&mut output,&mut input,action,|method,params|{
            if method=="settings.save"{caller.call_completing(method,params)}else{caller.call(method,params)}
        },||guard(owner as HWND),||caller.cancellation_requested())
    })?;
    let deadline=Instant::now()+limit;
    let mut stopped=None;
    while !io.is_finished(){
        if stopped.is_none() && (client.cancellation_requested() || Instant::now()>=deadline){
            stopped=Some(if client.cancellation_requested(){RpcError::new("cancelled","权限操作已取消")}else{RpcError::new("timeout","权限操作超时，请检查当前设置。")});
            if let Err(error)=child.kill(){if !matches!(child.try_wait(),Ok(Some(_))){stopped=Some(RpcError::new("permission",format!("无法结束权限辅助进程：{error}")));}}
        }
        if stopped.is_some(){
            // Interrupt a blocked anonymous-pipe read/write even if a descendant
            // retained a pipe handle. Only this owned helper I/O thread is affected.
            unsafe{CancelSynchronousIo(io.as_raw_handle() as _);}
        }
        std::thread::sleep(Duration::from_millis(20));
    }
    let result=io.join().unwrap_or_else(|_|Err(RpcError::new("permission","权限通信线程意外退出")));
    let exit_deadline=Instant::now()+Duration::from_secs(3);
    let status=loop{
        if let Some(status)=child.try_wait()?{break status;}
        if Instant::now()>=exit_deadline{child.kill()?;let _=child.wait();return Err(RpcError::new("permission","权限辅助进程未正常退出。"));}
        std::thread::sleep(Duration::from_millis(20));
    };
    if let Some(error)=stopped{return Err(error);}
    if !status.success(){return result.and_then(|_|Err(RpcError::new("permission",format!("权限辅助进程退出：{status}"))));}
    result
}

fn exchange(reader:&mut impl Read,writer:&mut impl Write,action:Action,
    mut call:impl FnMut(&str,Value)->Result<Value,RpcError>,guard:impl Fn()->bool,cancelled:impl Fn()->bool)->Result<Value,RpcError>{
    let mut saved=None;let mut save_attempted=false;
    let mut snapshot=Value::Null;let mut prepared=None;let mut committed=false;let mut acknowledged=None;
    for sequence in 1..=8192u64{
        if cancelled(){return Err(RpcError::new("cancelled","权限操作已取消"));}
        let message:Value=serde_json::from_slice(&read_frame(reader)?)?;
        if message["version"]!=1 || message["sequence"].as_u64()!=Some(sequence){return Err(protocol_error());}
        let result=match message["kind"].as_str(){
            Some("guard")=>Ok(json!(guard())),
            Some("call")=>{
                let method=message["method"].as_str().ok_or_else(protocol_error)?;
                let allowed=method=="snapshot" || match action{
                    Action::Preference(_)=>method=="settings.save" && !save_attempted,
                    Action::Restart=>matches!(method,"application.restart.prepare"|"application.restart.status"|"application.restart.commit"|"application.restart.cancel"),
                };
                if !allowed{return Err(protocol_error());}
                if method=="settings.save"{save_attempted=true;}
                if method=="application.restart.prepare" && prepared.is_some() || method=="application.restart.commit" && committed{return Err(protocol_error());}
                let result=call(method,message["params"].clone());
                if let Ok(value)=&result{
                    if method=="snapshot"{snapshot=value.clone();}
                    if method=="settings.save"{saved=Some(value.clone());}
                    if method=="application.restart.prepare"{prepared=token(&value["token"]).map(str::to_owned);}
                    if method=="application.restart.commit" && value["state"]=="committed"{committed=true;}
                    if matches!(method,"application.restart.commit"|"application.restart.status") && value["state"]=="committed" && value["helperAcknowledged"]==true
                        && value["helperPid"].as_u64().is_some_and(|pid|pid>0) && token(&value["token"])==prepared.as_deref(){acknowledged=prepared.clone();}
                }
                result
            },
            Some("done")=>{
                if message.get("error").is_some()==message.get("result").is_some(){return Err(protocol_error());}
                if let Some(error)=message.get("error"){return Err(serde_json::from_value(error.clone())?);}
                let result=message["result"].clone();
                match (action,result["closing"].as_bool()){
                    (Action::Preference(enabled),Some(false)) if saved.as_ref()==Some(&result["settings"]) && result["settings"]["startup"]["preferAdministrator"]==enabled=>{},
                    (Action::Restart,Some(true)) if committed && acknowledged.is_some() && token(&result["token"])==acknowledged.as_deref()=>{},
                    (Action::Restart,Some(false)) if snapshot["administrator"]==true && snapshot["settings"]==result["settings"]=>{},
                    _=>return Err(protocol_error()),
                }
                return Ok(result);
            },
            _=>return Err(protocol_error()),
        };
        let response=match result{
            Ok(value)=>json!({"version":1,"sequence":sequence,"result":value}),
            Err(error)=>json!({"version":1,"sequence":sequence,"error":error}),
        };
        write_frame(writer,&serde_json::to_vec(&response)?)?;
    }
    Err(protocol_error())
}

#[cfg(test)]
mod tests{
    use super::*;
    fn frames(messages:Vec<Value>)->std::io::Cursor<Vec<u8>>{
        let mut result=Vec::new();for (index,mut message) in messages.into_iter().enumerate(){message["version"]=json!(1);message["sequence"]=json!(index+1);write_frame(&mut result,&serde_json::to_vec(&message).unwrap()).unwrap();}std::io::Cursor::new(result)
    }
    #[test]
    fn cancelled_permission_does_not_start_a_helper(){
        use std::sync::atomic::AtomicBool;
        let client=Arc::new(Client::new(r"\\.\pipe\WinOne-permission-cancel-test",Arc::new(AtomicBool::new(true))).unwrap());
        let error=run_child(Command::new("missing-permission-helper.exe"),client,std::ptr::null_mut(),json!({}),Action::Restart,Duration::from_secs(1)).unwrap_err();
        assert_eq!(error.code,"cancelled");
    }
    #[test]
    fn permission_relay_preserves_confirmed_settings_and_rejects_unverified_closure(){
        let settings=json!({"revision":8,"startup":{"preferAdministrator":true},"unknown":"kept"});
        let mut input=frames(vec![json!({"kind":"guard"}),json!({"kind":"call","method":"settings.save","params":settings}),json!({"kind":"done","result":{"closing":false,"settings":settings}})]);
        let result=exchange(&mut input,&mut Vec::new(),Action::Preference(true),|method,value|{assert_eq!(method,"settings.save");Ok(value)},||true,||false).unwrap();
        assert_eq!(result["settings"],settings);
        for message in [json!({"kind":"done","result":{"closing":true,"token":"a".repeat(32)}}),json!({"kind":"call","method":"modules.setEnabled","params":{}})]{
            assert!(exchange(&mut frames(vec![message]),&mut Vec::new(),Action::Restart,|_,_|panic!("unauthorized relay"),||true,||false).is_err());
        }
    }
    #[test]
    fn stalled_permission_helper_is_terminated_and_io_is_joined(){
        use std::{os::windows::process::CommandExt,sync::atomic::AtomicBool};
        use windows_sys::Win32::System::Threading::CREATE_NO_WINDOW;
        let executable=std::path::PathBuf::from(std::env::var_os("SystemRoot").unwrap()).join("System32/WindowsPowerShell/v1.0/powershell.exe");
        let mut command=Command::new(executable);command.creation_flags(CREATE_NO_WINDOW).args(["-NoProfile","-NonInteractive","-Command","Start-Sleep -Seconds 30"]);
        let client=Arc::new(Client::new(r"\\.\pipe\WinOne-permission-timeout-test",Arc::new(AtomicBool::new(false))).unwrap());
        let started=Instant::now();
        let error=run_child(command,client,std::ptr::null_mut(),json!({"test":true}),Action::Restart,Duration::from_millis(100)).unwrap_err();
        assert_eq!(error.code,"timeout");assert!(started.elapsed()<Duration::from_secs(3));
    }
    #[test]
    fn restart_result_requires_the_same_committed_and_acknowledged_token(){
        for acknowledge in [false,true]{
            let id="a".repeat(32);
            let mut messages=vec![json!({"kind":"call","method":"application.restart.prepare"}),json!({"kind":"call","method":"application.restart.commit"})];
            if acknowledge{messages.push(json!({"kind":"call","method":"application.restart.status"}));}
            messages.push(json!({"kind":"done","result":{"closing":true,"token":id}}));
            let result=exchange(&mut frames(messages),&mut Vec::new(),Action::Restart,|method,_|{
                Ok(json!({"token":id,"state":if method.ends_with("prepare"){"prepared"}else{"committed"},"helperPid":9,"helperAcknowledged":method.ends_with("status")}))
            },||true,||false);
            assert_eq!(result.is_ok(),acknowledge);
        }
    }
    #[test]
    fn failed_preference_save_is_never_retried_by_the_helper(){
        let mut calls=0;
        let result=exchange(&mut frames(vec![json!({"kind":"call","method":"settings.save"}),json!({"kind":"call","method":"settings.save"})]),&mut Vec::new(),Action::Preference(true),|_,_|{
            calls+=1;Err(RpcError::new("conflict","settings changed"))
        },||true,||false);
        assert!(result.is_err());assert_eq!(calls,1);
    }
    #[test]
    #[ignore="requires a current WinOne.Host.exe from build/test-desktop-rust.ps1"]
    fn real_host_helper_rejects_isolated_profiles_without_authorizing(){
        use std::{os::windows::process::CommandExt,sync::atomic::AtomicBool};
        use windows_sys::Win32::System::Threading::CREATE_NO_WINDOW;
        let mut command=Command::new(std::env::var_os("WINONE_RUST_PERMISSION_HELPER").unwrap());command.creation_flags(CREATE_NO_WINDOW).arg("--desktop-control");
        let pipe=r"\\.\pipe\WinOne-permission-isolated-test";
        let client=Arc::new(Client::new(pipe,Arc::new(AtomicBool::new(false))).unwrap());
        let error=run_child(command,client,std::ptr::null_mut(),json!({"version":1,"operation":"preference","enabled":true,"ownerPID":std::process::id(),"pipe":pipe,
            "dataDirectory":std::env::temp_dir().join("WinOne-permission-isolated").to_string_lossy()}),Action::Preference(true),Duration::from_secs(5)).unwrap_err();
        assert_eq!(error.code,"permission");assert!(error.message.contains("独立测试配置"),"{error}");
    }
}
