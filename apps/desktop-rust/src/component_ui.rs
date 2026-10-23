//! Rust window/IPC host for the existing component UI. No controls are drawn here.
use crate::{component_assets,component_bridge::{self,Request,Reply},windows_files::wide};
use raw_window_handle::{HasWindowHandle,WindowHandle,RawWindowHandle,Win32WindowHandle,HandleError};
use serde_json::{Value,json};
use std::collections::HashSet;
use std::{cell::{Cell,RefCell},num::NonZeroIsize,path::PathBuf,ptr::{null,null_mut},sync::{Arc,Mutex,atomic::{AtomicBool,Ordering},mpsc},thread::JoinHandle};
use winone_desktop::{transport::Client,protocol::RpcError};
use windows_sys::{core::w,Win32::{Foundation::*,System::LibraryLoader::GetModuleHandleW,UI::{WindowsAndMessaging::*,HiDpi::*,Input::KeyboardAndMouse::{ReleaseCapture,SetFocus}}}};
use wry::{WebView,WebViewBuilder,WebViewExtWindows,WebContext,Rect,dpi::{PhysicalPosition,PhysicalSize}};
use windows_sys::Win32::Graphics::Gdi::{MonitorFromWindow,MonitorFromRect,GetMonitorInfoW,MONITORINFO,MONITOR_DEFAULTTONEAREST};

// Wry only calls CoInitializeEx. RegisterDragDrop additionally requires OLE
// initialization on this STA, otherwise Windows returns E_OUTOFMEMORY and Wry
// leaves every target unregistered. Hold the guard until all WebViews are gone.
struct OleApartment(std::marker::PhantomData<std::rc::Rc<()>>);
impl OleApartment {
    fn initialize()->Result<Self,String>{
        let status=unsafe{windows_sys::Win32::System::Ole::OleInitialize(null())};
        if status<0{return Err(format!("无法初始化 Windows 文件拖放：OleInitialize 返回 0x{:08X}",status as u32));}
        Ok(Self(std::marker::PhantomData))
    }
}
impl Drop for OleApartment {
    fn drop(&mut self){unsafe{windows_sys::Win32::System::Ole::OleUninitialize();}}
}

fn monitor_info(window:HWND)->Option<MONITORINFO>{
    let mut info=MONITORINFO{cbSize:std::mem::size_of::<MONITORINFO>()as u32,..Default::default()};
    (unsafe{GetMonitorInfoW(MonitorFromWindow(window,MONITOR_DEFAULTTONEAREST),&mut info)}!=0).then_some(info)
}

fn window_icon(module:HMODULE)->HICON{
    unsafe extern "system" fn found(module:HMODULE,_kind:*const u16,name:*const u16,context:isize)->i32{
        unsafe{*(context as *mut HICON)=LoadIconW(module,name);}0
    }
    let mut icon=std::ptr::null_mut();
    unsafe{windows_sys::Win32::System::LibraryLoader::EnumResourceNamesW(module,14 as *const u16,Some(found),(&mut icon as *mut HICON)as isize);}
    icon
}

const DISPATCH:u32=WM_APP+20;
const HOST_EXITED:u32=WM_APP+21;
// Sent WM_SIZE/WM_WINDOWPOSCHANGED callbacks can run inside GetMessage without
// making it return. Queue one wake so deferred layout cannot wait for a later
// click, Host reply or screenshot to release the outer message loop.
fn request_view_sync(window:HWND,requested:&Cell<bool>){
    if !requested.replace(true)&&unsafe{PostMessageW(window,DISPATCH,0,0)}==0{
        eprintln!("desktop view wake: {}",std::io::Error::last_os_error());
    }
}
// COM can deliver WM_CLOSE while the view's RefCell is already borrowed. Keep
// that request outside the view and drain it once the outer dispatch returns.
struct WindowState { value:RefCell<State>, close_requested:Cell<Option<bool>>, view_sync_requested:Cell<bool> }
impl std::ops::Deref for WindowState { type Target=RefCell<State>;fn deref(&self)->&Self::Target{&self.value} }
impl WindowState {
    fn drain_close(&self){
        if let Ok(mut state)=self.value.try_borrow_mut(){
            if let Some(exit)=self.close_requested.get(){if state.webview.is_some(){self.close_requested.set(None);state.exit_requested|=exit;state.close();}}
            // ShowWindow synchronously re-enters the native procedure while
            // State is borrowed. Keep visibility/layout intent outside that
            // borrow, just like close intent, and apply it before the next wait.
            if self.view_sync_requested.replace(false){state.sync_view();}
        }
    }
}

#[derive(Debug,PartialEq)]
enum CloseAction { Guard, Hide, Exit }
// Every close source uses the same activity guard. Full application exit is
// sticky through its confirmation so accepting it can never merely hide UI.
fn close_action(keep:bool,exit:bool,dirty:bool,busy:bool)->CloseAction {
    if dirty||busy {CloseAction::Guard} else if keep&&!exit {CloseAction::Hide} else {CloseAction::Exit}
}
// A hidden window is still preparing its first frame (or running the bounded
// read-only startup probe). Keep that controller active until initialization;
// only a user-hidden/minimized initialized window should suspend composition.
fn view_is_active(frame_initialized:bool,window_visible:bool,minimized:bool)->bool {
    !minimized&&(!frame_initialized||window_visible)
}
struct HostWatch { stop:HANDLE, task:Option<JoinHandle<()>>, server_pid:u32 }
struct BrowserLifetime(HANDLE);
impl Drop for BrowserLifetime{fn drop(&mut self){unsafe{CloseHandle(self.0);}}}
impl BrowserLifetime{
    fn finish(&self){
        // Keep WebContext/its environment alive while Close is delivered. COM
        // callbacks need the STA pump; releasing it immediately can leave the
        // runtime waiting for the embedder to disappear instead of closing.
        let until=std::time::Instant::now()+std::time::Duration::from_secs(4);
        loop{
            let left=until.saturating_duration_since(std::time::Instant::now()).as_millis()as u32;
            match unsafe{MsgWaitForMultipleObjects(1,&self.0,0,left,QS_ALLINPUT)}{
                WAIT_OBJECT_0=>return,
                value if value==WAIT_OBJECT_0+1=>unsafe{let mut msg=MSG::default();while PeekMessageW(&mut msg,null_mut(),0,0,PM_REMOVE)!=0{if msg.message!=WM_QUIT{TranslateMessage(&msg);DispatchMessageW(&msg);}}},
                _=>{eprintln!("WebView2 shutdown remains pending after controller close");return;},
            }
            if std::time::Instant::now()>=until{return;}
        }
    }
}

#[cfg(test)]
mod background_tests {
    use super::*;
    #[test]
    fn closing_preserves_activity_and_distinguishes_application_exit() {
        assert_eq!(close_action(true,false,false,false),CloseAction::Hide);
        assert_eq!(close_action(false,false,false,false),CloseAction::Exit);
        assert_eq!(close_action(true,true,false,false),CloseAction::Exit);
        for keep in [true,false] { for exit in [true,false] {
            assert_eq!(close_action(keep,exit,true,false),CloseAction::Guard);
            assert_eq!(close_action(keep,exit,false,true),CloseAction::Guard);
        }}
    }
    #[test]
    fn native_view_sync_posts_one_wake_until_the_pending_batch_is_drained() {
        std::thread::spawn(||unsafe{
            // A hidden, runner-owned window needs no WebView or user input.
            // Its queue must wake even if a sent resize is the final message.
            let window=CreateWindowExW(0,w!("STATIC"),w!("WinOne view-sync test"),0,0,0,1,1,null_mut(),null_mut(),GetModuleHandleW(null()),null());
            assert!(!window.is_null());
            struct OwnedWindow(HWND);
            impl Drop for OwnedWindow {fn drop(&mut self){unsafe{DestroyWindow(self.0);}}}
            let _window=OwnedWindow(window);
            let requested=Cell::new(false);let mut message=MSG::default();
            request_view_sync(window,&requested);request_view_sync(window,&requested);
            assert!(requested.get());
            assert_ne!(PeekMessageW(&mut message,window,DISPATCH,DISPATCH,PM_REMOVE),0,"a sent resize must wake the outer message loop");
            assert_eq!(PeekMessageW(&mut message,window,DISPATCH,DISPATCH,PM_REMOVE),0,"resize messages should coalesce");
            requested.set(false);request_view_sync(window,&requested);
            assert_ne!(PeekMessageW(&mut message,window,DISPATCH,DISPATCH,PM_REMOVE),0,"the next resize must not lose its wake");
        }).join().unwrap();
    }
    #[test]
    fn first_frame_and_hidden_probe_stay_active_but_retained_windows_suspend() {
        assert!(view_is_active(false,false,false));
        assert!(view_is_active(false,true,false));
        assert!(view_is_active(true,true,false));
        assert!(!view_is_active(true,false,false));
        for initialized in [true,false] {for visible in [true,false] {
            assert!(!view_is_active(initialized,visible,true));
        }}
    }
}
impl HostWatch {
    fn start(server:Arc<winone_desktop::transport::ServerProcess>,window:HWND,cancel:Arc<AtomicBool>)->Result<Self,RpcError>{
        use windows_sys::Win32::System::Threading::*;
        let stop=unsafe{CreateEventW(null(),1,0,null())};
        if stop.is_null(){return Err(std::io::Error::last_os_error().into());}
        let (signal,target,server_pid)=(stop as usize,window as usize,server.id());
        let task=std::thread::Builder::new().name("host-lifetime".into()).spawn(move||{
            match server.wait_for_exit(signal as HANDLE){
                Ok(Some(0)) if !cancel.load(Ordering::Acquire)=>unsafe{
                    let mut pid=0;GetWindowThreadProcessId(target as HWND,&mut pid);
                    if pid==std::process::id(){PostMessageW(target as HWND,HOST_EXITED,0,0);}
                },
                // Keep the local diagnostics and unsaved draft after a crash.
                Ok(Some(code)) if code!=0=>eprintln!("Host exited unexpectedly: {code}"),
                Err(error)=>eprintln!("Host lifetime observation: {error}"),_=>{},
            }
        });
        match task{Ok(task)=>Ok(Self{stop,task:Some(task),server_pid}),Err(error)=>{unsafe{CloseHandle(stop);}Err(error.into())}}
    }
}
impl Drop for HostWatch {fn drop(&mut self){
    unsafe{windows_sys::Win32::System::Threading::SetEvent(self.stop);}
    if let Some(task)=self.task.take(){let _=task.join();}
    unsafe{CloseHandle(self.stop);}
}}
enum Message{Request(Request),Reply(String,Reply),FilesDropped(Vec<String>,(i32,i32)),Browser(crate::component_browser::BrowserEvent)}
#[derive(Default)]
struct HostReady{initialized:bool,error:Option<RpcError>}
pub(crate) struct Window(pub(crate) HWND);
impl HasWindowHandle for Window{
    fn window_handle(&self)->Result<WindowHandle<'_>,HandleError>{
        let handle=Win32WindowHandle::new(NonZeroIsize::new(self.0 as isize).ok_or(HandleError::Unavailable)?);
        Ok(unsafe{WindowHandle::borrow_raw(RawWindowHandle::Win32(handle))})
    }
}
struct State{
    host_watch:Option<HostWatch>,
    window:HWND,webview:Option<WebView>,view_visible:Option<bool>,client:Arc<Client>,cancel:Arc<AtomicBool>,
    incoming:mpsc::Receiver<Message>,outgoing:mpsc::Sender<Message>,jobs:Vec<(bool,JoinHandle<()>)>,
    ready:Arc<Mutex<HostReady>>,plan:crate::startup::Plan,pipe:String,directory:String,
    dirty:bool,busy:bool,close_pending:bool,navigation:u64,update_id:Option<String>,
    keep_ui_running:bool,exit_requested:bool,
    mutations:HashSet<u64>,
    file_drop_enabled:Arc<AtomicBool>,
    locale:String,
    theme:String,
    frame_initialized:bool,
    initial_show:i32,placement:Option<crate::component_placement::Store>,window_warning:Option<String>,
    probe:Option<Arc<Mutex<crate::component_probe::Probe>>>,
    browsers:Option<crate::component_browser::BrowserManager>,
}
impl State{
    fn sync_view(&mut self){
        let Some(view)=&self.webview else{return;};
        let visible=view_is_active(self.frame_initialized,unsafe{IsWindowVisible(self.window)!=0},unsafe{IsIconic(self.window)!=0});
        if visible{
            let mut rect=RECT::default();
            if unsafe{GetClientRect(self.window,&mut rect)}!=0&&rect.right>0&&rect.bottom>0{
                if let Err(error)=view.set_bounds(Rect{position:PhysicalPosition::new(0,0).into(),size:PhysicalSize::new(rect.right as u32,rect.bottom as u32).into()}){eprintln!("webview layout: {error}");}
            }
        }
        // Keep the last real viewport while hidden/minimized, instead of
        // collapsing it to 1×1. Explicit controller visibility lets WebView2
        // resume its compositor immediately without navigating/rebuilding UI.
        if self.view_visible!=Some(visible){
            match view.set_visible(visible){Ok(())=>self.view_visible=Some(visible),Err(error)=>eprintln!("webview visibility: {error}"),}
        }
        if visible{if let Err(error)=crate::component_file_drop::resize(self.window){eprintln!("file drop layout: {error}");}}
    }
    fn tr<'a>(&self,zh:&'a str,en:&'a str)->&'a str{component_bridge::text(&self.locale,zh,en)}
    fn reply(&self,reply:Reply){if let Some(view)=&self.webview{
        let script=format!("window.__winoneReply({})",serde_json::to_string(&reply).unwrap());
        if let Err(error)=view.evaluate_script(&script){eprintln!("desktop reply: {error}");}
    }}
    fn event(&self,name:&str){if let Some(view)=&self.webview{
        if let Err(error)=view.evaluate_script(&format!("window.__winoneEvent({})",json!(name))){eprintln!("desktop event: {error}");}
    }}
    fn close(&mut self){
        match close_action(self.keep_ui_running,self.exit_requested,self.dirty,self.busy||!self.mutations.is_empty()) {
            CloseAction::Guard=>{self.close_pending=true;reveal(self.window);self.event("winone:settings-close");},
            CloseAction::Hide=>{self.save_placement();unsafe{ShowWindow(self.window,SW_HIDE);}},
            CloseAction::Exit=>self.finish_close(),
        }
    }
    fn finish_close(&mut self){
        self.save_placement();self.cancel.store(true,Ordering::Release);self.host_watch.take();
        // Wry closes its WebView2 controller and detaches the parent subclass
        // in Drop. Do that while the parent HWND still exists, not after its
        // child window has already been destroyed by DestroyWindow(parent).
        if let Some(browser)=&mut self.browsers{browser.clear();}
        self.webview.take();
        unsafe{DestroyWindow(self.window);}
    }
    fn save_placement(&mut self){
        if !self.frame_initialized{return;}
        if let Some(store)=&mut self.placement{match store.save(self.window){
            Ok(())=>self.window_warning=None,
            Err(error)=>{let warning=format!("无法保存窗口位置：{error}");eprintln!("{warning}");self.window_warning=Some(warning);}
        }}
    }
    fn frame(&self)->Value{json!({"maximized":unsafe{IsZoomed(self.window)}!=0,"warning":self.window_warning.as_deref().unwrap_or("")})}
    fn ui_request(&mut self,request:&Request)->Option<Result<Value,RpcError>>{
        if let Some(probe)=&self.probe{match probe.lock(){Ok(mut probe)=>if let Some(result)=probe.request(self.window,request){return Some(result);},Err(_)=>return Some(Err(RpcError::new("probe","启动检查状态无效"))),}}
        let result=(||{match request.method.as_str(){
            "ChatBrowser"=>{
                let payload:Value=serde_json::from_str(request.text(1)?)?;
                let result=self.browsers.as_mut().ok_or_else(||RpcError::new("browser","网页宿主尚未就绪"))?.request(self.window,request.text(0)?,payload)?;
                Ok(json!(result.to_string()))
            },
            "InitializeWindow"=>{if !self.frame_initialized{self.frame_initialized=true;unsafe{
                ShowWindow(self.window,self.initial_show);activate(self.window);
                // Match the original host's activation, then focus WebView
                // after the current state borrow releases its native callback.
                PostMessageW(self.window,WM_SIZE,0,0);PostMessageW(self.window,WM_SETFOCUS,0,0);
            }}Ok(self.frame())},
            "WindowFrameState"=>Ok(self.frame()),
            "WindowCommand"=>{match request.text(0)?{
                "close"=>self.close(),"minimize"=>{unsafe{ShowWindow(self.window,SW_MINIMIZE);}},
                "toggle-maximize"=>{unsafe{ShowWindow(self.window,if IsZoomed(self.window)!=0{SW_RESTORE}else{SW_MAXIMIZE});}},
                _=>return Err(RpcError::new("invalid","不支持的窗口操作")),
            }
                // ShowWindow sends WM_SIZE synchronously while this request
                // owns State. Re-layout after that borrow is released so the
                // original UI receives maximize/restore viewport changes.
                if request.text(0)?!="close"{unsafe{PostMessageW(self.window,WM_SIZE,0,0);}}
                Ok(Value::Null)},
            "BeginWindowDrag"=>{unsafe{ReleaseCapture();PostMessageW(self.window,WM_NCLBUTTONDOWN,HTCAPTION as usize,0);}Ok(Value::Null)},
            "BeginWindowResize"=>{
                let hit=match request.text(0)?{"n-resize"=>HTTOP,"ne-resize"=>HTTOPRIGHT,"e-resize"=>HTRIGHT,"se-resize"=>HTBOTTOMRIGHT,"s-resize"=>HTBOTTOM,"sw-resize"=>HTBOTTOMLEFT,"w-resize"=>HTLEFT,"nw-resize"=>HTTOPLEFT,_=>return Err(RpcError::new("invalid","窗口边缘无效"))};
                unsafe{ReleaseCapture();PostMessageW(self.window,WM_NCLBUTTONDOWN,hit as usize,0);}Ok(Value::Null)
            },
            "SetSettingsActivity"=>{self.dirty=request.boolean(0)?;self.busy=request.boolean(1)?;Ok(Value::Null)},
            "SettingsClosePending"=>Ok(json!(self.close_pending)),
            "CancelSettingsClose"=>{self.close_pending=false;self.exit_requested=false;Ok(Value::Null)},
            "ApproveSettingsClose"=>{
                if self.busy||!self.mutations.is_empty(){return Err(RpcError::new("busy","请等待当前操作完成"));}
                // A retained document would otherwise keep the form the user
                // explicitly discarded. Reset form components, not the IPC
                // document: in-flight read replies retain their unique IDs.
                if self.dirty&&self.keep_ui_running&&!self.exit_requested{self.event("winone:settings-discarded");}
                self.dirty=false;self.close_pending=false;self.close();Ok(Value::Null)
            },
            "SettingsNavigation"=>Ok(json!(self.navigation)),
            "SetFileDropTarget"=>{self.file_drop_enabled.store(request.boolean(0)?,Ordering::Release);Ok(Value::Null)},
            "StartupProbeNonce"=>{crate::startup::trace("ui-ready");Ok(json!(""))},
            "Reveal"|"KeepSettingsWindow"=>{reveal(self.window);Ok(Value::Null)},
            "PickProgram"|"PickPrograms"=>{
                let paths=crate::dialogs::pick_programs_labeled(self.window,self.tr("选择程序或脚本",if request.method=="PickPrograms"{"Choose programs or scripts"}else{"Choose a program or script"}),request.method=="PickPrograms",self.tr("程序与脚本","Programs and scripts")).map_err(|e|RpcError::new("dialog",e))?;
                Ok(if request.method=="PickProgram"{json!(paths.first().cloned().unwrap_or_default())}else{json!(paths)})
            },
            "PickDirectory"=>Ok(json!(crate::dialogs::pick_folder(self.window,self.tr("选择文件夹","Choose a folder")).map_err(|e|RpcError::new("dialog",e))?.unwrap_or_default())),
            "PickChatFiles"=>Ok(json!(crate::dialogs::pick_chat_files(self.window,self.tr("添加附件","Add attachments")).map_err(|e|RpcError::new("dialog",e))?)),
            "PickFile"=>Ok(json!(crate::dialogs::pick_file(self.window,self.tr("选择文件","Choose a file")).map_err(|e|RpcError::new("dialog",e))?.unwrap_or_default())),
            "PickDirectories"=>Ok(json!(crate::dialogs::pick_directories(self.window,self.tr("选择文件夹","Choose folders")).map_err(|e|RpcError::new("dialog",e))?)),
            "PickProfileLocation"=>match request.text(0)?{
                "file"=>Ok(json!(crate::dialogs::pick_file(self.window,self.tr("选择文件","Choose a file")).map_err(|e|RpcError::new("dialog",e))?.unwrap_or_default())),
                "directory"=>Ok(json!(crate::dialogs::pick_folder(self.window,self.tr("选择文件夹","Choose a folder")).map_err(|e|RpcError::new("dialog",e))?.unwrap_or_default())),
                "program"=>Ok(json!(crate::dialogs::pick_programs_labeled(self.window,self.tr("选择程序或脚本","Choose a program or script"),false,self.tr("程序与脚本","Programs and scripts")).map_err(|e|RpcError::new("dialog",e))?.first().cloned().unwrap_or_default())),
                _=>Err(RpcError::new("invalid","不支持的路径类型")),
            },
            "PickProfile"=>Ok(json!(crate::dialogs::pick_profile_labeled(self.window,self.tr("选择配置文件","Choose a configuration file"),self.tr("WinOne 配置","WinOne configuration")).map_err(|e|RpcError::new("dialog",e))?.unwrap_or_default())),
            "PickModulePackage"=>Ok(json!(crate::dialogs::pick_module_package(self.window,self.tr("选择模块包","Choose a module package"),self.tr("WinOne 模块包","WinOne module package")).map_err(|e|RpcError::new("dialog",e))?.unwrap_or_default())),
            _=>return Err(RpcError::new("delegate","")),
        }})();
        if result.as_ref().is_err_and(|e|e.code=="delegate"){None}else{Some(result)}
    }
    fn dispatch(&mut self){
        // WebView2 can invoke initialization scripts before its builder returns.
        // Keep those messages until the view can receive their replies.
        if self.webview.is_none(){return;}
        if let Some(server)=self.client.connected_server(){if self.host_watch.as_ref().is_none_or(|watch|watch.server_pid!=server.id()){
            self.host_watch.take();
            match HostWatch::start(server,self.window,self.cancel.clone()){
                Ok(watch)=>self.host_watch=Some(watch),
                Err(error)=>{crate::report(&error.to_string());self.exit_requested=true;self.close();return;},
            }
        }}
        self.jobs.retain(|(_,job)|!job.is_finished());
        while let Ok(message)=self.incoming.try_recv(){match message{
            Message::Browser(event)=>{
                if let Some(detail)=self.browsers.as_mut().and_then(|browser|browser.event(event)){if let Some(view)=&self.webview{
                    if let Err(error)=view.evaluate_script(&format!("window.dispatchEvent(new CustomEvent('winone:chat-browser',{{detail:{detail}}}))")){eprintln!("browser event: {error}");}
                }}
            },
            Message::FilesDropped(paths,position)=>{
                if let Some(view)=&self.webview{
                    // Wry supplies physical coordinates relative to the WebView.
                    // DOM hit-testing uses CSS pixels, including per-monitor DPI.
                    let script=format!("(()=>{{const paths={},point={},scale=window.devicePixelRatio||1,position={{x:point[0]/scale,y:point[1]/scale}},element=document.elementFromPoint(position.x,position.y);if(element&&!element.dispatchEvent(new CustomEvent('winone:paths-drop',{{bubbles:true,cancelable:true,detail:{{paths,position}}}})))return;if(document.querySelector('[role=dialog]'))return;const name=document.querySelector('[data-directory-links-root]')?'winone:directory-links-drop':document.querySelector('.chat-workspace')?'winone:chat-files-drop':null;if(name)window.dispatchEvent(new CustomEvent(name,{{detail:{{paths,position}}}}));}})()",json!(paths),json!(position));
                    if let Err(error)=view.evaluate_script(&script){eprintln!("native file drop: {error}");}
                }
            },
            Message::Reply(method,mut reply)=>{
                self.mutations.remove(&reply.id);
                if method=="Snapshot"{if let Some(value)=&reply.result{self.locale=value["locale"].as_str().unwrap_or("zh-CN").into();}}
                if matches!(method.as_str(),"Snapshot"|"SaveSettings"){if let Some(value)=&reply.result{
                    let settings=if method=="Snapshot"{&value["settings"]}else{value};
                    self.keep_ui_running=settings["startup"]["keepUIRunning"].as_bool().unwrap_or(true);
                    self.theme=settings["theme"].as_str().unwrap_or("system").into();
                    if let Err(error)=crate::component_frame::theme(self.window,&self.theme){eprintln!("{error}");}
                }}
                if method=="Snapshot"&&!self.plan.warning.is_empty(){if let Some(value)=reply.result.as_mut(){
                    if value["administrator"]!=true&&value["settings"]["startup"]["preferAdministrator"]==true{
                        value["runtimeWarning"]=json!(format!("{}\n{}",value["runtimeWarning"].as_str().unwrap_or(""),self.plan.warning).trim());
                    }
                }}
                let closing=method=="RestartAsAdministrator"&&reply.result==Some(json!(true));self.reply(reply);if closing{self.finish_close();}
            },
            Message::Request(mut request)=>{
                if request.method=="Capture"&&matches!(request.args.first().and_then(Value::as_str),Some("capture.start"|"capture.unified"|"scroll.retake"|"edit.last")){
                    unsafe{ShowWindow(self.window,SW_HIDE);}
                    let result=unsafe{windows_sys::Win32::Graphics::Dwm::DwmFlush()};
                    if result<0||unsafe{IsWindowVisible(self.window)}!=0{unsafe{ShowWindow(self.window,SW_SHOW);}self.reply(Reply::new(request.id,Err(RpcError::new("capture","窗口画面尚未准备好，请重试"))));continue;}
                }
                if request.method=="CheckForUpdates"{self.update_id=request.args.first().and_then(Value::as_str).map(str::to_owned);}
                if request.method=="CancelUpdateCheck"&&request.args.first().and_then(Value::as_str)==self.update_id.as_deref(){self.update_id=None;}
                if request.method=="OpenUpdateRelease"{
                    let Some(id)=&self.update_id else{self.reply(Reply::new(request.id,Err(RpcError::new("update","请先检查更新"))));continue;};request.args=vec![json!(id)];
                }
                if let Some(result)=self.ui_request(&request){self.reply(Reply::new(request.id,result));continue;}
                // Replies may have completed additional jobs while this same
                // dispatch batch admits queued reads. Count only live work;
                // already-finished handles must not produce a spurious busy.
                self.jobs.retain(|(_,job)|!job.is_finished());
                if self.jobs.len()>=16{self.reply(Reply::new(request.id,Err(RpcError::new("busy","当前请求较多，请稍后重试"))));continue;}
                let mutating=request.mutates();if mutating{self.mutations.insert(request.id);}
                let (client,ready,plan,directory,pipe,target,outgoing)=(self.client.clone(),self.ready.clone(),self.plan.clone(),self.directory.clone(),self.pipe.clone(),self.window as usize,self.outgoing.clone());
                let locale=self.locale.clone();
                let job=std::thread::spawn(move||{
                    let result=(||{
                        if !matches!(request.method.as_str(),"ReadModuleLogs"|"OpenModuleLogFolder"|"ProgramInfo"){
                            let mut initialized=ready.lock().map_err(|_|RpcError::new("startup","后台初始化失败"))?;
                            // Preserve a failed concurrent startup for the first real UI
                            // request. Later explicit reads retain the existing retry path.
                            if let Some(error)=initialized.error.take(){return Err(error);}
                            if !initialized.initialized||request.method=="Snapshot"{
                                let snapshot=crate::startup::ensure_host(&client,&pipe,&directory,&plan)?;initialized.initialized=true;
                                // Readiness already returns the real current snapshot. Reuse
                                // it for this request instead of immediately reading it again.
                                if request.method=="Snapshot"{return Ok(snapshot);}
                            }
                        }
                        component_bridge::execute(&request,client.clone(),&directory,&pipe,target as HWND,&locale)
                    })();
                    if client.cancellation_requested(){return;}
                    if outgoing.send(Message::Reply(request.method,Reply::new(request.id,result))).is_ok(){unsafe{PostMessageW(target as HWND,DISPATCH,0,0);}}
                });self.jobs.push((mutating,job));
            }
        }}
    }
}
impl Drop for State{fn drop(&mut self){
    self.cancel.store(true,Ordering::Release);
    self.host_watch.take();
    let mut owner=0;
    let owns_window=unsafe{GetWindowThreadProcessId(self.window,&mut owner)}==unsafe{windows_sys::Win32::System::Threading::GetCurrentThreadId()}&&owner==std::process::id();
    // Clear the callback pointer before releasing COM objects or a window:
    // their destructors may re-enter the procedure while State is dropping.
    if owns_window{unsafe{SetWindowLongPtrW(self.window,GWLP_USERDATA,0);}}
    for (mutating,job) in self.jobs.drain(..){
        // Shell metadata on an unavailable network path is not cancellable.
        // Read-only threads cannot post to a closed HWND (see dispatch), and
        // process exit reclaims them. Admitted writes still finish their guard.
        if mutating||job.is_finished(){let _=job.join();}
    }
    if let Some(browser)=&mut self.browsers{browser.clear();}
    self.webview.take();
    if owns_window{unsafe{DestroyWindow(self.window);}}
}}

unsafe extern "system" fn procedure(window:HWND,message:u32,w:WPARAM,l:LPARAM)->LRESULT{
    if message==WM_NCCREATE{unsafe{SetWindowLongPtrW(window,GWLP_USERDATA,(*(l as *const CREATESTRUCTW)).lpCreateParams as isize);}}
    // The project titlebar component owns the caption; Windows still owns the
    // frame, resizing and window lifecycle. No substitute titlebar is drawn.
    if message==WM_NCCALCSIZE{
        if w!=0&&l!=0&&unsafe{IsZoomed(window)}!=0{
            // Match the original Wails frameless policy: maximized windows
            // have invisible resize borders outside the monitor. Keep their
            // actual client surface inside the proposed monitor's work area.
            let rect=unsafe{&mut *(l as *mut RECT)};
            let mut monitor=MONITORINFO{cbSize:std::mem::size_of::<MONITORINFO>()as u32,..Default::default()};
            if unsafe{GetMonitorInfoW(MonitorFromRect(rect,MONITOR_DEFAULTTONEAREST),&mut monitor)}!=0{*rect=monitor.rcWork;}
        }
        return 0;
    }
    // Activation may be nested inside ShowWindow while State is borrowed.
    // Restoring the existing system frame must not depend on that borrow.
    if message==WM_ACTIVATE||message==WM_DWMCOMPOSITIONCHANGED{
        if let Err(error)=crate::component_frame::decorations(window){eprintln!("{error}");}
    }
    // DestroyWindow can re-enter while a UI request owns State. Quitting the
    // message loop must not depend on reacquiring that borrow.
    if message==WM_DESTROY{unsafe{PostQuitMessage(0);}return 0;}
    // ShowWindow/SetWindowPlacement request these constraints synchronously,
    // including while State is borrowed. They depend only on the HWND.
    if message==WM_GETMINMAXINFO&&l!=0{
        let scale=unsafe{GetDpiForWindow(window)}.max(96)as i32;let info=unsafe{&mut *(l as *mut MINMAXINFO)};
        info.ptMinTrackSize=POINT{x:800*scale/96,y:560*scale/96};
        if let Some(monitor)=monitor_info(window){
            let width=monitor.rcWork.right-monitor.rcWork.left;let height=monitor.rcWork.bottom-monitor.rcWork.top;
            info.ptMinTrackSize.x=info.ptMinTrackSize.x.min(width);info.ptMinTrackSize.y=info.ptMinTrackSize.y.min(height);
            info.ptMaxPosition=POINT{x:monitor.rcWork.left-monitor.rcMonitor.left,y:monitor.rcWork.top-monitor.rcMonitor.top};info.ptMaxSize=POINT{x:width,y:height};
        }
        return 0;
    }
    let pointer=unsafe{GetWindowLongPtrW(window,GWLP_USERDATA)}as *const WindowState;
    if !pointer.is_null()&&matches!(message,WM_SHOWWINDOW|WM_SIZE|WM_WINDOWPOSCHANGED){request_view_sync(window,&unsafe{&*pointer}.view_sync_requested);}
    if !pointer.is_null() && (message==WM_CLOSE || message==HOST_EXITED){
        let pending=&unsafe{&*pointer}.close_requested;
        // Host sends WM_CLOSE(wParam=1) for tray/maintenance exit. Zero is a
        // user close (including Alt+F4); older window hosts ignore this marker.
        pending.set(Some(pending.get().unwrap_or(false)||message==HOST_EXITED||w==1));
    }
    if !pointer.is_null(){if let Ok(mut state)=unsafe{&*pointer}.try_borrow_mut(){match message{
        DISPATCH=>{state.dispatch();return 0;},
        WM_CLOSE|HOST_EXITED=>{if state.webview.is_some(){state.exit_requested|=unsafe{&*pointer}.close_requested.take().unwrap_or(false);state.close();}return 0;},
        WM_DROPFILES=>{
            let source=if l!=0&&unsafe{IsChild(window,l as HWND)}!=0{l as HWND}else{window};
            let received=crate::component_file_drop::receive(source,w as _);
            match received{
                Ok((paths,position)) if state.file_drop_enabled.load(Ordering::Acquire)=>{let _=state.outgoing.send(Message::FilesDropped(paths,position));unsafe{PostMessageW(window,DISPATCH,0,0);}},
                Ok(_)=>{},Err(error)=>crate::report(&error),
            }
            return 0;
        },
        WM_EXITSIZEMOVE=>{state.save_placement();return 0;},
        WM_SETTINGCHANGE|WM_THEMECHANGED=>{if let Err(error)=crate::component_frame::theme(window,&state.theme){eprintln!("{error}");}},
        WM_TIMER if w==crate::component_probe::TIMER&&state.probe.is_some()=>{state.finish_close();return 0;},
        crate::permissions::GUARD_MESSAGE=>return (!state.dirty&&!state.close_pending)as isize,
        crate::lifecycle::WAKE=>{if state.frame_initialized{reveal(window);}if w!=0{state.navigation+=1;state.event("winone:open-settings");}return 0;},
        WM_SIZE=>{return 0;}, // drained after re-entrant ShowWindow/layout returns
        WM_SETFOCUS=>{if let Some(view)=&state.webview{let _=view.focus();}return 0;},
        WM_DPICHANGED=>{let rect=unsafe{&*(l as *const RECT)};unsafe{SetWindowPos(window,null_mut(),rect.left,rect.top,rect.right-rect.left,rect.bottom-rect.top,SWP_NOZORDER|SWP_NOACTIVATE);PostMessageW(window,WM_SIZE,0,0);}return 0;},
        _=>{},
    }}else if message==crate::permissions::GUARD_MESSAGE{return 2;}else if message==WM_CLOSE||message==HOST_EXITED{return 0;}}
    unsafe{DefWindowProcW(window,message,w,l)}
}

pub fn run(pipe:&str,directory:&str,open_settings:bool,plan:crate::startup::Plan)->Result<(),String>{
    run_inner(pipe,directory,open_settings,plan,None)
}
fn reveal(window:HWND){unsafe{
    // Showing an existing window preserves its placement and maximized state.
    ShowWindow(window,if IsIconic(window)!=0{SW_RESTORE}else{SW_SHOW});
    activate(window);PostMessageW(window,WM_SIZE,0,0);PostMessageW(window,WM_SETFOCUS,0,0);
}}

// Only explicit opens reach this path. WebView startup can outlive the
// launcher's input queue; connect to the current foreground queue just for
// activation, then detach immediately. Window topmost state never changes.
fn activate(window:HWND){unsafe{
    if SetForegroundWindow(window)!=0{SetFocus(window);return;}
    let foreground=GetForegroundWindow();
    let thread=windows_sys::Win32::System::Threading::GetCurrentThreadId();
    let other=GetWindowThreadProcessId(foreground,null_mut());
    let attached=other!=0&&other!=thread&&IsHungAppWindow(foreground)==0
        &&windows_sys::Win32::System::Threading::AttachThreadInput(thread,other,1)!=0;
    BringWindowToTop(window);
    windows_sys::Win32::UI::Input::KeyboardAndMouse::SetActiveWindow(window);
    SetForegroundWindow(window);SetFocus(window);
    if attached{windows_sys::Win32::System::Threading::AttachThreadInput(thread,other,0);}
    if GetForegroundWindow()!=window{eprintln!("desktop activation was not accepted by Windows");}
}}
pub fn run_probe(directory:&str,probe:Arc<Mutex<crate::component_probe::Probe>>)->Result<(),String>{
    run_inner(r"\\.\pipe\WinOne-isolated-startup-probe",directory,false,Default::default(),Some(probe))
}
fn run_inner(pipe:&str,directory:&str,open_settings:bool,plan:crate::startup::Plan,probe:Option<Arc<Mutex<crate::component_probe::Probe>>>)->Result<(),String>{
    let _ole=OleApartment::initialize()?;
    crate::startup::trace("component-entry");
    let mut instance=if probe.is_some(){None}else{let Some(instance)=crate::lifecycle::Instance::claim(pipe,open_settings)?else{return Ok(());};Some(instance)};
    unsafe{SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);}
    let cancel=Arc::new(AtomicBool::new(false));let client=Arc::new(Client::new(pipe,cancel.clone()).map_err(|e|e.to_string())?);
    let (outgoing,incoming)=mpsc::channel();
    let checking=probe.is_some();
    let ready=Arc::new(Mutex::new(HostReady::default()));
    let mut jobs=Vec::new();
    if !checking{
        // This is part of the user's current window launch, not a resident or
        // prewarmed runtime. Start Host while WebView2 creates its environment.
        let (client,ready,pipe,directory,plan)=(client.clone(),ready.clone(),pipe.to_owned(),directory.to_owned(),plan.clone());
        jobs.push((false,std::thread::spawn(move||{
            if let Ok(mut state)=ready.lock(){
                if state.initialized{return;}
                match crate::startup::ensure_host(&client,&pipe,&directory,&plan){
                    Ok(_)=>{state.initialized=true;crate::startup::trace("host-ready");},
                    Err(error)=>state.error=Some(error),
                }
            }
        })));
    }
    let file_drop_enabled=Arc::new(AtomicBool::new(false));
    // An offline startup has no tray or shortcut owner. Adopt the background
    // preference only after a successful Host snapshot confirms this session.
    let state=Box::new(WindowState{value:RefCell::new(State{host_watch:None,window:null_mut(),webview:None,view_visible:None,client,cancel,incoming,outgoing:outgoing.clone(),jobs,ready,plan,pipe:pipe.into(),directory:directory.into(),dirty:false,busy:false,close_pending:false,keep_ui_running:false,exit_requested:false,navigation:open_settings as u64,update_id:None,mutations:HashSet::new(),file_drop_enabled:file_drop_enabled.clone(),probe:probe.clone(),locale:"zh-CN".into(),theme:"system".into(),frame_initialized:false,initial_show:SW_SHOWNORMAL,placement:None,window_warning:None,browsers:None}),close_requested:Cell::new(None),view_sync_requested:Cell::new(false)});
    let module=unsafe{GetModuleHandleW(null())};
    // The Host resolves this shared class before normal tray exit. Preserve
    // the existing identity so its close and unsaved-edit guards still apply.
    let class=WNDCLASSW{lpfnWndProc:Some(procedure),hInstance:module,hIcon:window_icon(module),hCursor:unsafe{LoadCursorW(null_mut(),IDC_ARROW)},lpszClassName:w!("WinOne.Desktop"),..Default::default()};
    if unsafe{RegisterClassW(&class)}==0{return Err(std::io::Error::last_os_error().to_string());}
    let window=unsafe{CreateWindowExW(WS_EX_CONTROLPARENT|WS_EX_APPWINDOW,class.lpszClassName,wide("WinOne").as_ptr(),WS_OVERLAPPEDWINDOW|WS_CLIPCHILDREN,CW_USEDEFAULT,CW_USEDEFAULT,1280,840,null_mut(),null_mut(),module,(&*state as *const WindowState).cast())};
    if window.is_null(){return Err(std::io::Error::last_os_error().to_string());}
    state.borrow_mut().window=window;if let Some(instance)=&mut instance{instance.attach(window)?;}
    crate::component_frame::decorations(window)?;
    crate::component_frame::theme(window,"system")?;
    if let Some(monitor)=monitor_info(window){
        let dpi=unsafe{GetDpiForWindow(window)}.max(96)as i32;
        let width=(1280*dpi/96).min(monitor.rcWork.right-monitor.rcWork.left);let height=(840*dpi/96).min(monitor.rcWork.bottom-monitor.rcWork.top);
        let left=monitor.rcWork.left+(monitor.rcWork.right-monitor.rcWork.left-width)/2;
        let top=monitor.rcWork.top+(monitor.rcWork.bottom-monitor.rcWork.top-height)/2;
        unsafe{SetWindowPos(window,null_mut(),left,top,width,height,SWP_NOZORDER|SWP_NOACTIVATE);}
    }
    if !checking{
        let mut placement=crate::component_placement::Store::new(directory);
        // Do not hold State while SetWindowPlacement sends native messages.
        let restored=placement.restore(window);
        let mut state=state.borrow_mut();
        match restored{Ok(Some(true))=>state.initial_show=SW_SHOWMAXIMIZED,Ok(_)=>{},Err(error)=>state.window_warning=Some(format!("无法恢复窗口位置：{error}"))}
        state.placement=Some(placement);
    }
    let wrapper=Window(window);let mut context=WebContext::new(Some(PathBuf::from(directory).join("webview-components")));
    crate::startup::trace("native-window-ready");
    let target=window as usize;
    let browser_outgoing=outgoing.clone();
    state.borrow_mut().browsers=Some(crate::component_browser::BrowserManager::new(directory,move|event|{if browser_outgoing.send(Message::Browser(event)).is_ok(){unsafe{PostMessageW(target as HWND,DISPATCH,0,0);}}}));
    let asset_profile=PathBuf::from(directory);
    let drop_outgoing=outgoing.clone();
    let view=WebViewBuilder::new_with_web_context(&mut context)
        .with_asynchronous_custom_protocol("winone".into(),move|_,request,responder|{
            if request.uri().path().starts_with("/capture-"){
                let path=request.uri().path().to_owned();let profile=asset_profile.clone();
                std::thread::spawn(move||responder.respond(crate::component_capture_assets::response(&profile,&path)));
            }else{responder.respond(component_assets::response(request));}
        })
        .with_initialization_script(include_str!("component_runtime.js"))
        .with_drag_drop_handler(move|event|{
            if !file_drop_enabled.load(Ordering::Acquire){return false;}
            if let wry::DragDropEvent::Drop{paths,position,..}=event{
                let paths=paths.into_iter().map(|path|path.to_string_lossy().into_owned()).collect();
                if drop_outgoing.send(Message::FilesDropped(paths,position)).is_ok(){unsafe{PostMessageW(target as HWND,DISPATCH,0,0);}}
            }
            true
        })
        .with_ipc_handler(move|request|{
            if !request.uri().to_string().starts_with("http://winone.localhost/"){return;}
            match Request::parse(request.body()){
                Ok(request)=>{if outgoing.send(Message::Request(request)).is_ok(){unsafe{PostMessageW(target as HWND,DISPATCH,0,0);}}},
                Err(error)=>eprintln!("desktop request: {error}"),
            }
        })
        .with_navigation_handler(|url|url.starts_with("http://winone.localhost/")||url.starts_with("winone://localhost/"))
        .with_new_window_req_handler(|_,_|wry::NewWindowResponse::Deny)
        .with_url("winone://localhost/index.html");
    #[cfg(feature="component-test-debug")]
    let view={
        use wry::WebViewBuilderExtWindows;
        // WebView2 150+ intentionally ignores environment overrides at high
        // integrity (WebView2Feedback #5640). Compile a fixed port into the
        // opt-in test binary; never forward runtime environment into the API.
        // Preserve Wry 0.57's default options for this settings WebView.
        let port=env!("WINONE_COMPONENT_TEST_DEBUG_PORT").parse::<u16>().map_err(|_|"测试调试端口无效")?;
        view.with_additional_browser_args(format!("--disable-features=msWebOOUI,msPdfOOUI,msSmartScreenProtection --autoplay-policy=no-user-gesture-required --remote-debugging-address=127.0.0.1 --remote-debugging-port={port}"))
    };
    let view=view.build_as_child(&wrapper).map_err(|e|e.to_string())?;
    let _file_drop=if crate::component_file_drop::elevated()?{Some(crate::component_file_drop::prepare(window)?)}else{None};
    crate::startup::trace("webview-created");
    let mut browser_pid=0;unsafe{view.webview().BrowserProcessId(&mut browser_pid)}.map_err(|e|e.to_string())?;
    let browser_handle=unsafe{windows_sys::Win32::System::Threading::OpenProcess(windows_sys::Win32::System::Threading::PROCESS_SYNCHRONIZE,0,browser_pid)};
    if browser_handle.is_null(){return Err(std::io::Error::last_os_error().to_string());}
    let browser_lifetime=BrowserLifetime(browser_handle);
    if let Some(probe)=&probe {
        // Report the SDK's browser identity to the parent checker. The browser
        // may need this UI process to exit before it releases its cache files.
        let mut pid=0;unsafe{view.webview().BrowserProcessId(&mut pid)}.map_err(|e|e.to_string())?;
        use windows_sys::Win32::System::Threading::*;
        let handle=unsafe{OpenProcess(PROCESS_SYNCHRONIZE|PROCESS_QUERY_LIMITED_INFORMATION,0,pid)};
        if handle.is_null(){return Err(std::io::Error::last_os_error().to_string());}
        let(mut created,mut exited,mut kernel,mut user)=(FILETIME::default(),FILETIME::default(),FILETIME::default(),FILETIME::default());
        let ok=unsafe{GetProcessTimes(handle,&mut created,&mut exited,&mut kernel,&mut user)};
        let error=std::io::Error::last_os_error();unsafe{CloseHandle(handle);}
        if ok==0{return Err(error.to_string());}
        probe.lock().map_err(|_|"启动检查状态无效")?.browser=Some((pid,(created.dwHighDateTime as u64)<<32|created.dwLowDateTime as u64));
    }
    state.borrow_mut().webview=Some(view);
    unsafe{SendMessageW(window,WM_SIZE,0,0);PostMessageW(window,DISPATCH,0,0);if checking{SetTimer(window,crate::component_probe::TIMER,25000,None);}}
    let mut message=MSG::default();
    state.drain_close();
    loop{let result=unsafe{GetMessageW(&mut message,null_mut(),0,0)};if result==0{break;}if result<0{return Err(std::io::Error::last_os_error().to_string());}unsafe{TranslateMessage(&message);DispatchMessageW(&message);}state.drain_close();}
    drop(state);drop(instance);browser_lifetime.finish();drop(context);
    unsafe{UnregisterClassW(class.lpszClassName,module);}
    Ok(())
}

#[cfg(test)]mod ole_tests{
    use super::OleApartment;
    use windows_sys::Win32::System::Com::{CoInitializeEx,CoUninitialize,COINIT_MULTITHREADED,COINIT_APARTMENTTHREADED};
    #[test]fn ole_successes_are_balanced_including_repeated_initialization(){
        std::thread::spawn(||{
            let first=OleApartment::initialize().expect("STA OLE initialization");
            let repeated=OleApartment::initialize().expect("repeated OLE initialization");
            drop(repeated);drop(first);
            // CoGetApartmentType can see another thread's implicit MTA. A
            // fresh STA initialization returns S_OK; leaked nesting returns
            // S_FALSE, independently of concurrent tests' COM apartments.
            let fresh=unsafe{CoInitializeEx(std::ptr::null(),COINIT_APARTMENTTHREADED as u32)};
            if fresh>=0{unsafe{CoUninitialize();}}
            assert_eq!(fresh,0,"the thread must return to its uninitialized COM state");
        }).join().unwrap();
    }
    #[test]fn ole_apartment_mismatch_is_an_explicit_error(){
        std::thread::spawn(||{
            assert!(unsafe{CoInitializeEx(std::ptr::null(),COINIT_MULTITHREADED as u32)}>=0);
            let error=OleApartment::initialize().err().expect("OLE must reject a preexisting MTA");
            unsafe{CoUninitialize();}
            assert!(error.contains("0x80010106"),"{error}");
        }).join().unwrap();
    }
}
