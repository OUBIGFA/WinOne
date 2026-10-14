//! On-demand browser guests, isolated from the settings document and its IPC.
use serde::{Deserialize,Serialize};
use serde_json::{Value,json};
use std::{collections::HashMap,path::PathBuf,rc::Rc};
use windows::core::Interface;
use windows_sys::Win32::{Foundation::HWND,UI::WindowsAndMessaging::{GetClientRect,SetWindowPos,HWND_TOP,SWP_NOMOVE,SWP_NOSIZE,SWP_NOACTIVATE},UI::Input::KeyboardAndMouse::GetKeyState};
use webview2_com::{NavigationCompletedEventHandler,HistoryChangedEventHandler,SourceChangedEventHandler,ProcessFailedEventHandler,AcceleratorKeyPressedEventHandler,FaviconChangedEventHandler,IsDocumentPlayingAudioChangedEventHandler};
use webview2_com::Microsoft::Web::WebView2::Win32::*;
use wry::{WebView,WebViewBuilder,WebContext,WebViewExtWindows,Rect,dpi::{PhysicalPosition,PhysicalSize}};
use winone_desktop::protocol::RpcError;

fn failure(message:impl Into<String>)->RpcError{RpcError::new("browser",message.into())}
fn navigation_url(value:&str)->Result<String,RpcError>{
    if value=="about:blank"{return Ok(value.into());}
    if value.len()>32768||value.chars().any(char::is_control){return Err(failure("网页地址无效"));}
    let url=url::Url::parse(value).map_err(|_|failure("请输入完整的 HTTP 或 HTTPS 网页地址"))?;
    if !matches!(url.scheme(),"http"|"https")||url.host_str().is_none()||url.host_str()==Some("winone.localhost")||!url.username().is_empty()||url.password().is_some(){return Err(failure("此地址不能在内置浏览器打开"));}
    Ok(url.into())
}

#[derive(Clone,Copy,Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Bounds{pub x:f64,pub y:f64,pub width:f64,pub height:f64}
fn physical_bounds(value:Bounds,scale:f64)->Result<(i32,i32,u32,u32),RpcError>{
    if ![value.x,value.y,value.width,value.height,scale].iter().all(|v|v.is_finite())||!(0.25..=8.0).contains(&scale)||value.x<0.0||value.y<0.0||value.width<=0.0||value.height<=0.0||[value.x,value.y,value.width,value.height].iter().any(|v|v*scale>32768.0){return Err(failure("网页显示范围无效"));}
    Ok(((value.x*scale).round()as i32,(value.y*scale).round()as i32,(value.width*scale).round().max(1.0)as u32,(value.height*scale).round().max(1.0)as u32))
}

#[derive(Clone,Serialize)]
#[serde(rename_all="camelCase")]
pub struct BrowserState{
    pub session_id:String,pub tab_id:String,pub url:String,pub title:String,pub loading:bool,
    pub can_go_back:bool,pub can_go_forward:bool,pub error:String,pub visible:bool,
    pub favicon:String,pub is_audible:bool,pub muted:bool,pub crashed:bool,
}
#[derive(Clone)]pub struct BrowserEvent{pub key:String,pub generation:u64,pub kind:String,pub value:Value}
type Notify=Rc<dyn Fn(BrowserEvent)>;
struct Tab{view:Option<WebView>,state:BrowserState,generation:u64}
pub struct BrowserManager{tabs:HashMap<String,Tab>,contexts:HashMap<String,WebContext>,directory:PathBuf,notify:Notify,sequence:u64}
fn identity(payload:&Value)->Result<(String,String,String),RpcError>{
    let id=|field|payload[field].as_str().filter(|s|!s.is_empty()&&s.len()<=128&&s.bytes().all(|c|c.is_ascii_alphanumeric()||b"_-".contains(&c)||(field=="tabId"&&c==b':'))).map(str::to_owned).ok_or_else(||failure("网页标签标识无效"));
    let session=id("sessionId")?;let tab=id("tabId")?;Ok((format!("{session}/{tab}"),session,tab))
}
impl BrowserManager{
    pub fn new(directory:&str,notify:impl Fn(BrowserEvent)+'static)->Self{Self{tabs:HashMap::new(),contexts:HashMap::new(),directory:PathBuf::from(directory).join("webview-browser"),notify:Rc::new(notify),sequence:0}}
    pub fn clear(&mut self){self.tabs.clear();self.contexts.clear();}
    pub fn hide_all(&mut self){for tab in self.tabs.values_mut(){if let Some(view)=&tab.view{if let Err(error)=view.set_visible(false){eprintln!("browser hide: {error}");}}tab.state.visible=false;}}
    pub fn event(&mut self,event:BrowserEvent)->Option<Value>{
        let tab=self.tabs.get_mut(&event.key)?;if tab.generation!=event.generation{return None;}
        match event.kind.as_str(){
            "title"=>tab.state.title=event.value.as_str().unwrap_or("").into(),
            "loading"=>{tab.state.loading=event.value.as_bool().unwrap_or(false);if tab.state.loading{tab.state.error.clear();tab.state.crashed=false;}},
            "error"=>{tab.state.error=event.value.as_str().unwrap_or("网页未能加载").into();tab.state.loading=false;},
            "crash"=>{tab.state.crashed=true;tab.state.loading=false;tab.state.error="网页进程已停止，请重新加载".into();tab.view.take();},
            "newTab"|"command"|"comment"|"operation"=>return Some(json!({"kind":event.kind,"sessionId":tab.state.session_id,"tabId":tab.state.tab_id,"value":event.value})),
            _=>{},
        }
        if let Err(error)=refresh(tab){tab.state.error=error.to_string();}
        Some(json!({"kind":"state","state":tab.state}))
    }
    pub fn request(&mut self,owner:HWND,method:&str,payload:Value)->Result<Value,RpcError>{
        if method=="closeSession"{
            let session=payload["sessionId"].as_str().ok_or_else(||failure("会话标识无效"))?;
            self.tabs.retain(|_,tab|tab.state.session_id!=session);self.contexts.remove(session);return Ok(Value::Null);
        }
        let(key,session,tab_id)=identity(&payload)?;
        if method=="close"{self.tabs.remove(&key);if !self.tabs.values().any(|tab|tab.state.session_id==session){self.contexts.remove(&session);}return Ok(Value::Null);}
        if method=="create"&&!self.tabs.contains_key(&key){
            if self.tabs.len()>=32{return Err(failure("请先关闭不需要的网页标签"));}
            let url=navigation_url(payload["url"].as_str().unwrap_or("about:blank"))?;
            self.sequence+=1;
            self.tabs.insert(key.clone(),Tab{view:None,generation:self.sequence,state:BrowserState{session_id:session.clone(),tab_id,url,title:String::new(),loading:false,can_go_back:false,can_go_forward:false,error:String::new(),visible:false,favicon:String::new(),is_audible:false,muted:false,crashed:false}});
            if let Err(error)=self.materialize(owner,&key){self.tabs.remove(&key);return Err(error);}
        }
        if method=="reload"&&self.tabs.get(&key).is_some_and(|tab|tab.view.is_none()){self.materialize(owner,&key)?;}
        if method=="sync"&&payload["visible"]==true&&self.tabs.get(&key).is_some_and(|tab|tab.view.is_none()){self.materialize(owner,&key)?;}
        let tab=self.tabs.get_mut(&key).ok_or_else(||failure("网页标签已关闭"))?;
        if method=="capture"{
            let operation=payload["operationId"].as_str().filter(|v|v.len()<=128&&!v.is_empty()).ok_or_else(||failure("截图操作标识无效"))?.to_owned();
            let path=if payload["save"]==true{Some(self.directory.parent().ok_or_else(||failure("截图目录无效"))?.join("chat-browser-comments").join(&session).join(format!("{}.png",crate::dialogs::new_program_id().map_err(failure)?)))}else{None};
            let notify=self.notify.clone();let generation=tab.generation;let token=operation.clone();let event_key=key.clone();
            crate::browser_capture::capture(tab.view.as_ref().ok_or_else(||failure("网页尚未就绪"))?,owner,path,Rc::new(move|result|notify(BrowserEvent{key:event_key.clone(),generation,kind:"operation".into(),value:match result{Ok(value)=>json!({"operationId":token,"result":value}),Err(error)=>json!({"operationId":token,"error":error})}}))).map_err(failure)?;
            return Ok(json!({"operationId":operation}));
        }
        if method=="comment"{
            let channel=payload["channel"].as_str().ok_or_else(||failure("页面评论命令无效"))?;
            if !matches!(channel,"browser-comment:enter-mode"|"browser-comment:exit-mode"|"browser-comment:cancel-pending"|"browser-comment:commit-pending"|"browser-comment:prepare-screenshot"|"browser-comment:design-preview"|"browser-comment:design-reset"){return Err(failure("页面评论命令无效"));}
            let view=tab.view.as_ref().ok_or_else(||failure("网页尚未就绪"))?;
            view.evaluate_script(&format!("window.__winoneBrowserComment?.({},{})",json!(channel),payload["payload"])).map_err(|e|failure(e.to_string()))?;
            return Ok(Value::Null);
        }
        if method=="external"{
            let address=navigation_url(payload["url"].as_str().ok_or_else(||failure("缺少网页地址"))?)?;
            if address=="about:blank"{return Err(failure("空白页不能在系统浏览器打开"));}
            unsafe{
                use windows_sys::{core::w,Win32::UI::{Shell::ShellExecuteW,WindowsAndMessaging::SW_SHOWNORMAL}};
                let target:Vec<u16>=address.encode_utf16().chain(Some(0)).collect();
                let result=ShellExecuteW(owner,w!("open"),target.as_ptr(),std::ptr::null(),std::ptr::null(),SW_SHOWNORMAL)as isize;
                if result<=32{return Err(failure(format!("无法打开网页：{result}")));}
            }
            return Ok(Value::Null);
        }
        if method=="sync"{
            let visible=payload["visible"].as_bool().ok_or_else(||failure("缺少网页可见状态"))?;
            if let Some(view)=&tab.view{
                if visible{
                    let bounds:Bounds=serde_json::from_value(payload["bounds"].clone())?;let(x,y,width,height)=physical_bounds(bounds,payload["scale"].as_f64().unwrap_or(1.0))?;
                    let mut client=windows_sys::Win32::Foundation::RECT::default();unsafe{GetClientRect(owner,&mut client);}
                    if x+width as i32>client.right+1||y+height as i32>client.bottom+1{return Err(failure("网页范围超出侧栏"));}
                    view.set_bounds(Rect{position:PhysicalPosition::new(x,y).into(),size:PhysicalSize::new(width,height).into()}).map_err(|e|failure(e.to_string()))?;
                    // A native child normally covers renderer popovers. Cut a
                    // hole for the host comment editor while keeping the page
                    // live for style previews and full-viewport screenshots.
                    unsafe{
                        use windows_sys::Win32::Graphics::Gdi::{CreateRectRgn,CombineRgn,DeleteObject,SetWindowRgn,RGN_DIFF};
                        let region=CreateRectRgn(0,0,width as i32,height as i32);
                        if region.is_null(){return Err(failure("无法设置网页显示区域"));}
                        if !payload["occlusion"].is_null(){
                            let clip:Bounds=serde_json::from_value(payload["occlusion"].clone())?;
                            let scale=payload["scale"].as_f64().unwrap_or(1.0);
                            if [clip.x,clip.y,clip.width,clip.height].iter().all(|v|v.is_finite())&&clip.width>0.0&&clip.height>0.0{
                                let cut=CreateRectRgn((clip.x*scale).floor().max(0.0)as i32,(clip.y*scale).floor().max(0.0)as i32,((clip.x+clip.width)*scale).ceil().min(width as f64)as i32,((clip.y+clip.height)*scale).ceil().min(height as f64)as i32);
                                CombineRgn(region,region,cut,RGN_DIFF);DeleteObject(cut);
                            }
                        }
                        if SetWindowRgn(view.hwnd().0 as HWND,region,1)==0{DeleteObject(region);return Err(failure("无法更新网页显示区域"));}
                    }
                    unsafe{SetWindowPos(view.hwnd().0 as HWND,HWND_TOP,0,0,0,0,SWP_NOSIZE|SWP_NOMOVE|SWP_NOACTIVATE);}
                }
                view.set_visible(visible).map_err(|e|failure(e.to_string()))?;
                view.set_memory_usage_level(if visible{wry::MemoryUsageLevel::Normal}else{wry::MemoryUsageLevel::Low}).map_err(|e|failure(e.to_string()))?;
            }
            tab.state.visible=visible;
        }else if !matches!(method,"create"|"state"){
            let view=tab.view.as_ref().ok_or_else(||failure("网页已停止，请重新加载"))?;
            let native=view.webview();
            match method{
                "navigate"=>{let url=navigation_url(payload["url"].as_str().ok_or_else(||failure("缺少网页地址"))?)?;view.load_url(&url).map_err(|e|failure(e.to_string()))?;tab.state.url=url;tab.state.loading=true;tab.state.error.clear();},
                "back"=>unsafe{native.GoBack()}.map_err(|e|failure(e.to_string()))?,
                "forward"=>unsafe{native.GoForward()}.map_err(|e|failure(e.to_string()))?,
                "reload"=>{unsafe{native.Reload()}.map_err(|e|failure(e.to_string()))?;tab.state.error.clear();tab.state.loading=true;},
                "stop"=>{unsafe{native.Stop()}.map_err(|e|failure(e.to_string()))?;tab.state.loading=false;},
                "mute"=>{let audio=native.cast::<ICoreWebView2_8>().map_err(|e|failure(e.to_string()))?;unsafe{audio.SetIsMuted(payload["muted"].as_bool().unwrap_or(true))}.map_err(|e|failure(e.to_string()))?;},
                "terminate"=>{tab.view.take();tab.state.crashed=true;tab.state.loading=false;tab.state.error="网页已停止，可重新加载恢复".into();},
                _=>return Err(failure("不支持的网页操作")),
            }
        }
        refresh(tab)?;Ok(serde_json::to_value(&tab.state)?)
    }
    fn materialize(&mut self,owner:HWND,key:&str)->Result<(),RpcError>{
        let tab=self.tabs.get_mut(key).ok_or_else(||failure("网页标签已关闭"))?;
        self.sequence+=1;tab.generation=self.sequence;let generation=tab.generation;
        let notify=self.notify.clone();let key=key.to_owned();
        let emit:Rc<dyn Fn(&str,Value)>=Rc::new(move|kind,value|notify(BrowserEvent{key:key.clone(),generation,kind:kind.into(),value}));
        let context=self.contexts.entry(tab.state.session_id.clone()).or_insert_with(||WebContext::new(Some(self.directory.join(&tab.state.session_id))));
        let title=emit.clone();let page=emit.clone();let nav=emit.clone();let popup=emit.clone();let comment=emit.clone();
        // The guest receives only Cindy's page selection overlay. Its return
        // channel carries untrusted evidence and command acknowledgements, never
        // a settings request, native method, custom protocol or host object.
        let view=WebViewBuilder::new_with_web_context(context).with_visible(false)
            .with_bounds(Rect{position:PhysicalPosition::new(0,0).into(),size:PhysicalSize::new(1,1).into()})
            .with_navigation_handler(move|url|match navigation_url(&url){Ok(_)=>true,Err(error)=>{nav("error",json!(error.to_string()));false}})
            .with_new_window_req_handler(move|url,_|{if let Ok(url)=navigation_url(&url){popup("newTab",json!(url));}wry::NewWindowResponse::Deny})
            .with_document_title_changed_handler(move|value|title("title",json!(value)))
            .with_on_page_load_handler(move|event,_|page("loading",json!(matches!(event,wry::PageLoadEvent::Started))))
            .with_initialization_script(include_str!("browser_comment_guest.js"))
            .with_ipc_handler(move|request|{if request.body().len()>128*1024{return;}if let Ok(value)=serde_json::from_str::<Value>(request.body()){if value["kind"]=="comment"&&value["channel"].as_str().is_some_and(|v|matches!(v,"browser-comment:element-selected"|"browser-comment:mode-exited"|"browser-comment:command-result")){comment("comment",value);}}})
            .with_url(&tab.state.url).build_as_child(&crate::component_ui::Window(owner)).map_err(|e|failure(e.to_string()))?;
        let native=view.webview();let mut token=0i64;
        unsafe{
            let settings=native.Settings().map_err(|e|failure(e.to_string()))?;
            settings.SetAreHostObjectsAllowed(false).map_err(|e|failure(e.to_string()))?;
            settings.SetIsWebMessageEnabled(true).map_err(|e|failure(e.to_string()))?;
            let event=emit.clone();native.add_HistoryChanged(&HistoryChangedEventHandler::create(Box::new(move|_,_|{event("state",Value::Null);Ok(())})),&mut token).map_err(|e|failure(e.to_string()))?;
            let event=emit.clone();native.add_SourceChanged(&SourceChangedEventHandler::create(Box::new(move|_,_|{event("state",Value::Null);Ok(())})),&mut token).map_err(|e|failure(e.to_string()))?;
            let event=emit.clone();native.add_NavigationCompleted(&NavigationCompletedEventHandler::create(Box::new(move|_,args|{if let Some(args)=args{let mut success=Default::default();args.IsSuccess(&mut success)?;if !success.as_bool(){let mut status=COREWEBVIEW2_WEB_ERROR_STATUS_UNKNOWN;args.WebErrorStatus(&mut status)?;if status!=COREWEBVIEW2_WEB_ERROR_STATUS_OPERATION_CANCELED{event("error",json!(format!("网页加载失败（{}）",status.0)));}}}event("loading",json!(false));Ok(())})),&mut token).map_err(|e|failure(e.to_string()))?;
            let event=emit.clone();native.add_ProcessFailed(&ProcessFailedEventHandler::create(Box::new(move|_,_|{event("crash",Value::Null);Ok(())})),&mut token).map_err(|e|failure(e.to_string()))?;
            if let Ok(favicon)=native.cast::<ICoreWebView2_15>(){let event=emit.clone();favicon.add_FaviconChanged(&FaviconChangedEventHandler::create(Box::new(move|_,_|{event("state",Value::Null);Ok(())})),&mut token).map_err(|e|failure(e.to_string()))?;}
            if let Ok(audio)=native.cast::<ICoreWebView2_8>(){let event=emit.clone();audio.add_IsDocumentPlayingAudioChanged(&IsDocumentPlayingAudioChangedEventHandler::create(Box::new(move|_,_|{event("state",Value::Null);Ok(())})),&mut token).map_err(|e|failure(e.to_string()))?;}
            let event=emit;view.controller().add_AcceleratorKeyPressed(&AcceleratorKeyPressedEventHandler::create(Box::new(move|_,args|{
                if let Some(args)=args{let mut kind=COREWEBVIEW2_KEY_EVENT_KIND_KEY_DOWN;let mut key=0;args.KeyEventKind(&mut kind)?;args.VirtualKey(&mut key)?;
                    if matches!(kind,COREWEBVIEW2_KEY_EVENT_KIND_KEY_DOWN|COREWEBVIEW2_KEY_EVENT_KIND_SYSTEM_KEY_DOWN){let ctrl=GetKeyState(0x11)<0;let alt=GetKeyState(0x12)<0;let command=match(key,ctrl,alt){(0x4c,true,false)=>Some("focusAddress"),(0x57,true,false)=>Some("close"),(0x52,true,false)|(0x74,false,false)=>Some("reload"),(0x25,false,true)=>Some("back"),(0x27,false,true)=>Some("forward"),_=>None};if let Some(command)=command{args.SetHandled(true)?;event("command",json!(command));}}
                }Ok(())
            })),&mut token).map_err(|e|failure(e.to_string()))?;
        }
        tab.view=Some(view);tab.state.crashed=false;tab.state.error.clear();tab.state.loading=true;refresh(tab)
    }
}
fn refresh(tab:&mut Tab)->Result<(),RpcError>{
    let Some(view)=&tab.view else{return Ok(());};let native=view.webview();
    let current=view.url().map_err(|e|failure(e.to_string()))?;if !current.is_empty()&&current!="about:blank"{tab.state.url=current;}
    unsafe{
        let(mut back,mut forward)=(Default::default(),Default::default());native.CanGoBack(&mut back).map_err(|e|failure(e.to_string()))?;native.CanGoForward(&mut forward).map_err(|e|failure(e.to_string()))?;tab.state.can_go_back=back.as_bool();tab.state.can_go_forward=forward.as_bool();
        if let Ok(favicon)=native.cast::<ICoreWebView2_15>(){let mut value=windows::core::PWSTR::null();if favicon.FaviconUri(&mut value).is_ok(){let icon=webview2_com::take_pwstr(value);if icon.len()<=32768&&(icon.is_empty()||navigation_url(&icon).is_ok()){tab.state.favicon=icon;}}}
        if let Ok(audio)=native.cast::<ICoreWebView2_8>(){let(mut audible,mut muted)=(Default::default(),Default::default());audio.IsDocumentPlayingAudio(&mut audible).map_err(|e|failure(e.to_string()))?;audio.IsMuted(&mut muted).map_err(|e|failure(e.to_string()))?;tab.state.is_audible=audible.as_bool();tab.state.muted=muted.as_bool();}
    }Ok(())
}

#[cfg(test)]mod tests{
    use super::*;
    #[test]fn persisted_panel_tab_identity_is_accepted_but_session_paths_are_rejected(){
        assert!(identity(&json!({"sessionId":"chat-one","tabId":"browser:1234"})).is_ok());
        assert!(identity(&json!({"sessionId":"../chat","tabId":"browser:1234"})).is_err());
    }
    #[test]fn navigation_keeps_web_and_loopback_urls_but_rejects_local_or_privileged_schemes(){
        for url in ["https://example.com/path?q=a#b","http://127.0.0.1:8080/preview.html","about:blank"]{assert_eq!(navigation_url(url).unwrap(),url);}
        for url in ["file:///C:/Users/private.txt","javascript:alert(1)","data:text/html,test","winone://localhost/index.html","http://winone.localhost/index.html","https://user:password@example.com",""]{assert!(navigation_url(url).is_err(),"{url}");}
    }
    #[test]fn physical_browser_bounds_follow_dpi_and_reject_invalid_geometry(){
        assert_eq!(physical_bounds(Bounds{x:10.0,y:20.0,width:200.0,height:300.0},1.5).unwrap(),(15,30,300,450));
        assert!(physical_bounds(Bounds{x:0.0,y:0.0,width:f64::NAN,height:300.0},1.0).is_err());
        assert!(physical_bounds(Bounds{x:0.0,y:0.0,width:100.0,height:300.0},0.0).is_err());
    }
}
