//! Product launch preserves the existing profile, installation locks and
//! remembered authorization. Windows authorization itself stays in the Host's
//! shared Go implementation; the UI never creates a second task scheduler path.
use crate::{icon::sha256_hex, windows_files::{wide,final_path}};
use serde_json::{Value, json};
use std::{fs::{File, OpenOptions}, io::Write, os::windows::{fs::OpenOptionsExt, io::AsRawHandle, process::CommandExt}, path::{Path, PathBuf}, process::{Command, Stdio}, time::{Duration, Instant}};
use winone_desktop::{protocol::RpcError, transport::Client};
use windows_sys::Win32::{Foundation::*, Security::*, Storage::FileSystem::*, System::{IO::OVERLAPPED, Pipes::WaitNamedPipeW, Threading::*}};

#[derive(Clone, Default)]
pub struct Plan { pub standard: bool, pub warning: String, pub error: Option<String> }
pub enum Decision { Continue(Plan), Redirected(String) }

pub struct Options { pub pipe: String, pub directory: String, pub open_settings: bool, pub standard: bool, pub capture_draft: Option<String>,pub directory_links:bool,pub interconnect:bool,pub link_sources:Vec<String>,pub link_options:serde_json::Map<String,Value> }
impl Options {
    pub fn parse(args: &[String], default_pipe: &str, local: &Path) -> Result<Self, String> {
        if !local.is_absolute() { return Err("无法确定本机配置目录。".into()); }
        let mut result = Self {pipe:default_pipe.into(),directory:local.join("WinOne").to_string_lossy().into_owned(),open_settings:false,standard:false,capture_draft:None,directory_links:false,interconnect:false,link_sources:vec![],link_options:serde_json::Map::new()};
        let mut seen = std::collections::HashSet::new();
        let mut index=0;
        while index<args.len() {
            let argument=&args[index];
            if argument!="--source"&&!seen.insert(argument) { return Err("启动参数重复。".into()); }
            match argument.as_str() {
                "--pipe" | "--data-dir" | "--capture-draft" | "--source" | "--link-type" | "--link-targets" => {
                    index+=1;
                    let value=args.get(index).filter(|s| !s.is_empty() && !s.starts_with("--")).ok_or("启动参数缺少值。")?;
                    if argument=="--pipe" { result.pipe=value.clone(); }
                    else if argument=="--capture-draft" { result.capture_draft=Some(value.clone()); }
                    else if argument=="--source" { result.link_sources.push(value.clone()); }
                    else if argument=="--link-type" {if !matches!(value.as_str(),"auto"|"hardlink"|"junction"|"symlink"){return Err("链接类型无效。".into());}result.link_options.insert("kind".into(),json!(value));}
                    else if argument=="--link-targets" {if !matches!(value.as_str(),"absolute"|"relative"){return Err("链接路径类型无效。".into());}result.link_options.insert("relative".into(),json!(value=="relative"));}
                    else { result.directory=value.clone(); }
                }
                "--open-settings" => result.open_settings=true,
                "--directory-links" => {result.directory_links=true;result.open_settings=true;},
                "--interconnect" => {result.interconnect=true;result.open_settings=true;},
                "--absolute-paths" => {result.link_options.insert("relative".into(),json!(false));},
                "--link-retain-script"|"--retain-script" => {result.link_options.insert("retainScript".into(),json!(true));},
                "--link-hide-success"|"--hide-success-dialog" => {result.link_options.insert("hideSuccess".into(),json!(true));},
                value if value.starts_with("--link-targets=") => {let value=&value[15..];if !matches!(value,"absolute"|"relative"){return Err("链接路径类型无效。".into());}result.link_options.insert("relative".into(),json!(value=="relative"));},
                "--standard-session" => result.standard=true,
                _ => return Err("不支持的启动参数。".into()),
            }
            index+=1;
        }
        if !result.pipe.starts_with(r"\\.\pipe\WinOne-") || result.pipe.contains('\0') || result.pipe.len()>256 || !Path::new(&result.directory).is_absolute() || result.directory.contains('\0') {
            return Err("启动路径或管道无效。".into());
        }
        if result.capture_draft.as_ref().is_some_and(|id| id.len()!=32 || !id.bytes().all(|b|b.is_ascii_digit()||(b'a'..=b'f').contains(&b))) || (result.open_settings && result.capture_draft.is_some()) {
            return Err("截图启动参数无效。".into());
        }
        if result.directory_links&&result.interconnect||(!result.directory_links&&!result.interconnect&&!result.link_sources.is_empty())||(!result.directory_links&&!result.link_options.is_empty())||result.link_sources.len()>200||result.link_sources.iter().any(|source|!Path::new(source).is_absolute()||source.contains('\0')){return Err("目录链接启动参数无效。".into());}
        Ok(result)
    }
}

pub fn default_pipe() -> Result<String,String> {
    use windows_sys::Win32::{Security::Authorization::ConvertSidToStringSidW, System::RemoteDesktop::ProcessIdToSessionId};
    let mut token=std::ptr::null_mut();
    if unsafe { OpenProcessToken(GetCurrentProcess(),TOKEN_QUERY,&mut token) }==0 { return Err(std::io::Error::last_os_error().to_string()); }
    let result=(|| {
        let mut length=0;
        unsafe { GetTokenInformation(token,TokenUser,std::ptr::null_mut(),0,&mut length); }
        if length==0 || length>65536 { return Err("无法读取 Windows 账户。".into()); }
        let mut data=vec![0u64;(length as usize).div_ceil(8)];
        if unsafe { GetTokenInformation(token,TokenUser,data.as_mut_ptr().cast(),length,&mut length) }==0 { return Err(std::io::Error::last_os_error().to_string()); }
        let sid=unsafe { (*(data.as_ptr().cast::<TOKEN_USER>())).User.Sid };
        let mut text=std::ptr::null_mut();
        if unsafe { ConvertSidToStringSidW(sid,&mut text) }==0 { return Err(std::io::Error::last_os_error().to_string()); }
        let mut units=0;
        while unsafe { *text.add(units) }!=0 { units+=1; }
        let sid=String::from_utf16(unsafe { std::slice::from_raw_parts(text,units) }).map_err(|e|e.to_string());
        unsafe { LocalFree(text.cast()); }
        let mut session=0;
        if unsafe { ProcessIdToSessionId(GetCurrentProcessId(),&mut session) }==0 { return Err(std::io::Error::last_os_error().to_string()); }
        Ok(format!(r"\\.\pipe\WinOne-{}-{session}",sid?))
    })();
    unsafe { CloseHandle(token); }
    result
}
/// Opt-in startup diagnostics: phase names and elapsed time only, no settings
/// or paths. Keep the normal launch free of file I/O and background tracing.
pub fn trace(phase:&str) {
    static TRACE:std::sync::OnceLock<(Instant,bool)>=std::sync::OnceLock::new();
    let (start,enabled)=TRACE.get_or_init(||(Instant::now(),std::env::var_os("WINONE_DESKTOP_STARTUP_TRACE").is_some()));
    if *enabled{eprintln!("startup {:.3}ms {phase}",start.elapsed().as_secs_f64()*1000.0);}
}

/// Match maintenance.AcquireRuntimeForInstallation: installation first, profile
/// second, byte-zero shared locks, and no startup across a pending marker.
pub struct RuntimeSession { _installation: File, _profile: File }
impl RuntimeSession {
    pub fn acquire(profile: &Path, installation: &Path, local: &Path) -> Result<Self,String> {
        if !profile.is_absolute() || !installation.is_absolute() || !local.is_absolute() { return Err("维护锁路径无效。".into()); }
        let directory=OpenOptions::new().read(true).share_mode(FILE_SHARE_READ|FILE_SHARE_WRITE|FILE_SHARE_DELETE)
            .custom_flags(FILE_FLAG_BACKUP_SEMANTICS).open(installation).map_err(|e|e.to_string())?;
        // Go uses Unicode simple lowercase (one rune per rune) for this key.
        let identity: String=final_path(&directory).map_err(|e|e.to_string())?.chars().map(|c| c.to_lowercase().next().unwrap_or(c)).collect();
        let lock_directory=local.join("WinOne/installation-locks").join(sha256_hex(identity.as_bytes()));
        let installed=lock(&lock_directory)?;
        match std::fs::symlink_metadata(installation.join(".winone-pending")) {
            Err(error) if error.kind()==std::io::ErrorKind::NotFound => {},
            _ => return Err("安装或卸载尚未完成，暂时无法启动 WinOne。".into()),
        }
        let selected=lock(profile)?;
        Ok(Self {_installation:installed,_profile:selected})
    }
}
fn lock(directory: &Path) -> Result<File,String> {
    std::fs::create_dir_all(directory).map_err(|e|e.to_string())?;
    let file=OpenOptions::new().read(true).write(true).create(true).truncate(false).share_mode(FILE_SHARE_READ|FILE_SHARE_WRITE)
        .open(directory.join("maintenance.lock")).map_err(|e|e.to_string())?;
    let mut overlapped=OVERLAPPED::default();
    if unsafe { LockFileEx(file.as_raw_handle() as HANDLE,LOCKFILE_FAIL_IMMEDIATELY,0,1,0,&mut overlapped) }==0 {
        return Err(format!("WinOne 正在维护，请稍后重试：{}",std::io::Error::last_os_error()));
    }
    Ok(file)
}

fn companion() -> Result<PathBuf,String> {
    Ok(std::env::current_exe().map_err(|e|e.to_string())?.parent().ok_or("无法确定程序目录。")?.join("WinOne.Host.exe"))
}
pub(crate) fn command() -> Result<Command,String> {
    let mut command=Command::new(companion()?);
    command.creation_flags(CREATE_NO_WINDOW);
    Ok(command)
}
fn failure(output: &std::process::Output) -> String {
    let message=String::from_utf8_lossy(&output.stderr).trim().to_string();
    if message.is_empty() { format!("后台辅助进程退出：{}",output.status) } else { message }
}

/// Special task-scheduler/UAC entries are forwarded verbatim to the same fixed
/// sibling. The Host still validates mode, SID, elevation and handoff tokens.
pub fn helper(args: &[String]) -> Option<Result<(),String>> {
    if !args.first().is_some_and(|s| matches!(s.as_str(),"--authorized-start"|"--authorize-administrator"|"--revoke-administrator")) { return None; }
    Some((|| {
        let output=command()?.arg("--desktop-helper").args(args).output().map_err(|e|e.to_string())?;
        if !output.status.success() { return Err(failure(&output)); }
        Ok(())
    })())
}

/// Run before claiming the window identity so an elevated replacement never
/// accidentally wakes a standard-token window created by this same launch.
pub fn authorization(options: &Options, default_pipe: &str, local: &Path) -> Result<Decision,String> {
    let same_directory=options.directory.replace('/',"\\").trim_end_matches('\\').eq_ignore_ascii_case(&local.join("WinOne").to_string_lossy());
    if options.standard || options.pipe!=default_pipe || !same_directory {
        return Ok(Decision::Continue(Plan {standard:options.standard,..Default::default()}));
    }
    if !companion()?.try_exists().map_err(|e|e.to_string())? {
        return Ok(Decision::Continue(Plan {error:Some("缺少 WinOne.Host.exe，请修复安装。".into()),..Default::default()}));
    }
    let mut child=command()?.arg("--desktop-bootstrap").stdin(Stdio::piped()).stdout(Stdio::piped()).stderr(Stdio::piped()).spawn().map_err(|e|e.to_string())?;
    let payload=json!({"pipe":options.pipe,"dataDirectory":options.directory,"standardSession":options.standard,"draftId":options.capture_draft.as_deref().unwrap_or("")}).to_string();
    child.stdin.take().ok_or("无法传递启动信息。")?.write_all(payload.as_bytes()).map_err(|e|e.to_string())?;
    let output=child.wait_with_output().map_err(|e|e.to_string())?;
    if !output.status.success() { return Err(failure(&output)); }
    if output.stdout.len()>16384 { return Err("后台启动信息过长。".into()); }
    let value:Value=serde_json::from_slice(&output.stdout).map_err(|e|e.to_string())?;
    if value["version"]!=1 || !value["redirected"].is_boolean() || !value["standardSession"].is_boolean() || !value["warning"].is_string() { return Err("无法确认后台启动状态。".into()); }
    let warning=value["warning"].as_str().unwrap().to_string();
    if value["redirected"]==true {
        return Ok(Decision::Redirected(warning));
    }
    Ok(Decision::Continue(Plan {standard:value["standardSession"]==true,warning,error:None}))
}

pub fn ensure_host(client: &Client, pipe: &str, directory: &str, plan: &Plan) -> Result<Value,RpcError> {
    if let Some(error)=&plan.error { return Err(RpcError::new("startup",error)); }
    let initial_error=match client.call("snapshot",Value::Null){Ok(snapshot)=>return Ok(snapshot),Err(error)=>error};
    if client.cancellation_requested() { return Err(RpcError::new("cancelled","操作已取消")); }
    if !client.may_start_server()? { return Err(initial_error); }
    let available=unsafe { WaitNamedPipeW(wide(pipe).as_ptr(),100) };
    let error=unsafe { GetLastError() };
    if available!=0 || error!=ERROR_FILE_NOT_FOUND { return client.call("snapshot",Value::Null); }
    let mut command=command().map_err(|e|RpcError::new("startup",e))?;
    command.args(["--pipe",pipe,"--data-dir",directory]).stdin(Stdio::null()).stdout(Stdio::null()).stderr(Stdio::null());
    if plan.standard { command.arg("--standard-session"); }
    let mut child=command.spawn().map_err(|e|RpcError::new("startup",format!("无法启动后台：{e}")))?;
    let deadline=Instant::now()+Duration::from_secs(8);
    loop {
        match client.call("snapshot",Value::Null) {
            Ok(snapshot)=>return Ok(snapshot),
            Err(error)=> {
                if client.cancellation_requested() { return Err(error); }
                // Another legitimate launcher may win pipe creation. Always
                // probe the pipe before treating this child's exit as failure.
                if child.try_wait()?.is_some() || Instant::now()>=deadline { return Err(RpcError::new("startup",format!("后台未就绪，请查看诊断日志：{error}"))); }
                std::thread::sleep(Duration::from_millis(40));
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn launch_options_keep_profiles_and_reject_missing_duplicate_or_remote_values() {
        let args=vec!["--data-dir".into(),r"C:\Profiles\Other".into(),"--open-settings".into(),"--standard-session".into()];
        let value=Options::parse(&args,r"\\.\pipe\WinOne-test",Path::new(r"C:\Local")).unwrap();
        assert_eq!(value.directory,r"C:\Profiles\Other"); assert!(value.open_settings&&value.standard);
        for args in [vec!["--pipe"],vec!["--data-dir","relative"],vec!["--pipe",r"\\server\pipe\WinOne"],vec!["--open-settings","--open-settings"],vec!["--data-dir","--open-settings"]] {
            assert!(Options::parse(&args.into_iter().map(str::to_owned).collect::<Vec<_>>(),r"\\.\pipe\WinOne-test",Path::new(r"C:\Local")).is_err());
        }
    }
    #[test]
    fn a_cancelled_window_does_not_start_a_host() {
        let client=Client::new(r"\\.\pipe\WinOne-cancelled-start",std::sync::Arc::new(std::sync::atomic::AtomicBool::new(true))).unwrap();
        assert_eq!(ensure_host(&client,r"\\.\pipe\WinOne-cancelled-start",r"C:\Profiles\Test",&Plan::default()).unwrap_err().code,"cancelled");
    }
    #[test]
    fn explorer_link_prefill_preserves_paths_and_options_without_creating_links(){
        let args=["--directory-links","--source",r"C:\Documents\notes.txt","--link-type","hardlink","--link-targets=absolute","--retain-script"].map(str::to_owned);
        let value=Options::parse(&args,r"\\.\pipe\WinOne-test",Path::new(r"C:\Local")).unwrap();
        assert!(value.directory_links&&value.open_settings);assert_eq!(value.link_sources,vec![r"C:\Documents\notes.txt"]);assert_eq!(value.link_options["kind"],"hardlink");assert_eq!(value.link_options["relative"],false);assert_eq!(value.link_options["retainScript"],true);
        assert!(Options::parse(&["--source".into(),r"C:\file".into()],r"\\.\pipe\WinOne-test",Path::new(r"C:\Local")).is_err());
    }
    #[test]
    fn capture_entry_is_separate_and_requires_a_canonical_draft_id() {
        let id="a".repeat(32);
        let args=vec!["--capture-draft".into(),id.clone()];
        let options=Options::parse(&args,r"\\.\pipe\WinOne-test",Path::new(r"C:\Local")).unwrap();
        assert_eq!(options.capture_draft,Some(id));
        for args in [vec!["--capture-draft","../draft"],vec!["--capture-draft","AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"],vec!["--capture-draft","aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","--open-settings"]] {
            assert!(Options::parse(&args.into_iter().map(str::to_owned).collect::<Vec<_>>(),r"\\.\pipe\WinOne-test",Path::new(r"C:\Local")).is_err());
        }
    }
    #[test]
    #[ignore = "requires the Go maintenance fixture from build/test-desktop-rust.ps1"]
    fn real_maintenance_lock_probe() {
        use std::io::Read;
        let profile=PathBuf::from(std::env::var_os("WINONE_RUST_LOCK_PROFILE").unwrap());
        let installation=PathBuf::from(std::env::var_os("WINONE_RUST_LOCK_INSTALLATION").unwrap());
        let local=PathBuf::from(std::env::var_os("WINONE_RUST_LOCK_LOCAL").unwrap());
        let result=RuntimeSession::acquire(&profile,&installation,&local);
        if std::env::var("WINONE_RUST_LOCK_MODE").unwrap()=="blocked" {
            assert!(result.is_err(),"the Go lease or pending marker must block Rust startup");
        } else {
            let _session=result.unwrap();
            println!("LOCK_READY"); std::io::stdout().flush().unwrap();
            let mut release=[0]; std::io::stdin().read_exact(&mut release).unwrap();
        }
    }
}
