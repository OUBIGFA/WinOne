#![cfg_attr(windows, windows_subsystem = "windows")]
#[cfg(all(windows,feature="component-ui"))] mod component_file_drop;
#[cfg(all(windows,feature="component-ui"))] mod component_ui;
#[cfg(all(windows,feature="component-ui"))] mod component_browser;
#[cfg(all(windows,feature="component-ui"))] mod browser_capture;
#[cfg(all(windows,feature="component-ui"))] mod component_bridge;
#[cfg(all(windows,feature="component-ui"))] mod component_assets;
#[cfg(all(windows,feature="component-ui"))] mod component_frame;
#[cfg(all(windows,feature="component-ui"))] mod component_placement;
#[cfg(all(windows,feature="component-ui"))] mod component_probe;
#[cfg(all(windows,feature="component-ui"))] mod component_program_info;
#[cfg(all(windows,feature="component-ui"))] mod component_capture;
#[cfg(all(windows,feature="component-ui"))] mod usage_export;
#[cfg(all(windows,feature="component-ui"))] mod chat_export;
#[cfg(all(windows,feature="component-ui"))] mod component_capture_assets;
#[cfg(all(windows,any(not(feature="component-ui"),test)))] mod native;
#[cfg(all(windows,any(not(feature="component-ui"),test)))] mod widgets;
#[cfg(all(windows,any(not(feature="component-ui"),test)))] mod viewport;
#[cfg(all(windows,any(not(feature="component-ui"),test)))] mod worker;
#[cfg(windows)] mod dialogs;
#[cfg(windows)] mod icon;
#[cfg(windows)] mod lifecycle;
#[cfg(windows)] mod diagnostics;
#[cfg(windows)] mod capture_history;
#[cfg(windows)] mod windows_files;
#[cfg(windows)] mod startup;
#[cfg(windows)] mod permissions;
#[cfg(windows)] mod profile_transfer;
#[cfg(windows)] mod process_exit;

#[cfg(windows)]
fn main() {
    startup::trace("process-entry");
    let args:Vec<String>=std::env::args().skip(1).collect();
    #[cfg(feature="component-ui")]
    if component_probe::requested(&args){if let Err(error)=component_probe::run(&args){eprintln!("{error}");process_exit::after_cleanup(1);}process_exit::after_cleanup(0);}
    if let Some(result)=startup::helper(&args) {
        if let Err(error)=result { eprintln!("{error}"); process_exit::after_cleanup(1); }
        process_exit::after_cleanup(0);
    }
    if let Err(error)=run(&args) { report(&error); process_exit::after_cleanup(1); }
    process_exit::after_cleanup(0);
}
#[cfg(windows)]
fn run(args: &[String]) -> Result<(),String> {
    let default_pipe=startup::default_pipe()?;
    let local=std::path::PathBuf::from(std::env::var_os("LOCALAPPDATA").ok_or("无法确定本机配置目录。")?);
    let options=startup::Options::parse(args,&default_pipe,&local)?;
    let executable=std::env::current_exe().map_err(|e|e.to_string())?;
    // Match the existing desktop's maintenance admission: reject and exit
    // before UI startup. A modal error would keep the executable locked and
    // block the installer until someone dismissed it.
    let _lease=match startup::RuntimeSession::acquire(std::path::Path::new(&options.directory),executable.parent().ok_or("无法确定程序目录。")?,&local){
        Ok(lease)=>lease,
        Err(error)=>{eprintln!("{error}");process_exit::after_cleanup(1);}
    };
    let plan=match startup::authorization(&options,&default_pipe,&local)? {
        startup::Decision::Continue(plan)=>plan,
        // An accepted or uncertain elevated handoff must not be shadowed by
        // this launch claiming the standard window's singleton identity.
        startup::Decision::Redirected(warning)=>{
            if !warning.is_empty(){return Err(warning);}
            enqueue_directory_links(&options,&Default::default())?;
            // A delegated screenshot has a separate lifecycle. Only a settings
            // open transfers foreground rights to the existing settings HWND.
            return if options.capture_draft.is_none(){lifecycle::activate_redirected(&options.pipe)}else{Ok(())};
        },
    };
    startup::trace("authorization-ready");
    enqueue_directory_links(&options,&plan)?;
    if let Some(id)=&options.capture_draft {
        let client=winone_desktop::transport::Client::new(&options.pipe,std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false))).map_err(|e|e.to_string())?;
        let snapshot=startup::ensure_host(&client,&options.pipe,&options.directory,&plan).map_err(|e|e.to_string())?;
        // The worker owns the independent native capture window. This entry
        // waits for it without constructing a settings window or UI runtime.
        return match client.call_interactive_job("capture","draft.resume",serde_json::json!({"expectedId":id,"language":snapshot["locale"].as_str().unwrap_or("zh-CN")})) {
            Ok(_)=>Ok(()), Err(error) if error.code=="cancelled"=>Ok(()), Err(error)=>Err(error.to_string()),
        };
    }
    #[cfg(feature="component-ui")]
    return component_ui::run(&options.pipe,&options.directory,options.open_settings,plan);
    #[cfg(not(feature="component-ui"))]
    native::run(&options.pipe,&options.directory,options.open_settings,plan)
}
#[cfg(windows)]
fn enqueue_directory_links(options:&startup::Options,plan:&startup::Plan)->Result<(),String>{
    if !options.directory_links&&!options.interconnect{return Ok(());}
    let client=winone_desktop::transport::Client::new(&options.pipe,std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false))).map_err(|e|e.to_string())?;
    startup::ensure_host(&client,&options.pipe,&options.directory,plan).map_err(|e|e.to_string())?;
    client.call(if options.interconnect {"interconnect.open"} else {"directory-links.open"},if options.interconnect {serde_json::json!({"paths":options.link_sources})} else {serde_json::json!({"paths":options.link_sources,"options":options.link_options})}).map_err(|e|e.to_string())?;
    Ok(())
}
#[cfg(windows)]
fn report(error: &str) {
    use windows_sys::{core::w, Win32::UI::WindowsAndMessaging::*};
    unsafe { MessageBoxW(std::ptr::null_mut(), windows_files::wide(error).as_ptr(), w!("WinOne"), MB_OK | MB_ICONERROR); }
}
#[cfg(not(windows))]
fn main() { eprintln!("WinOne 桌面界面仅支持 Windows。"); }
