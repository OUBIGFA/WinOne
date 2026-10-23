//! Explorer's OLE drag is blocked by UIPI when WinOne is elevated. Keep OLE
//! normally; elevated windows use the Shell's WM_DROPFILES compatibility path.
//! Only file-transfer messages are admitted, never general IPC or commands.
use std::{cell::Cell,ptr::null_mut};
use windows_sys::{core::w,Win32::{Foundation::*,Graphics::Gdi::*,Security::*,System::{LibraryLoader::GetModuleHandleW,Threading::*},UI::{Shell::*,WindowsAndMessaging::*}}};
use windows_sys::Win32::UI::{Accessibility::*,Input::KeyboardAndMouse::GetAsyncKeyState};

thread_local!{static DROP_TARGET:Cell<HWND>=const{Cell::new(null_mut())};}
pub struct CaptureHook(HWINEVENTHOOK);
impl Drop for CaptureHook{fn drop(&mut self){DROP_TARGET.with(|target|target.set(null_mut()));unsafe{UnhookWinEvent(self.0);}}}

// Out-of-context events run on this window's UI thread. They report external
// mouse capture without a polling timer or a low-level input-blocking hook.
unsafe extern "system" fn capture_event(_:HWINEVENTHOOK,event:u32,source:HWND,_:i32,_:i32,_:u32,_:u32){
    DROP_TARGET.with(|target|{
        let child=target.get();if child.is_null(){return;}
        let root=unsafe{GetParent(child)};
        let external=unsafe{GetAncestor(source,GA_ROOTOWNER)}!=root;
        if event==EVENT_SYSTEM_CAPTURESTART&&external&&unsafe{GetAsyncKeyState(1)}<0&&unsafe{IsWindowVisible(root)}!=0&&unsafe{IsIconic(root)}==0{
            unsafe{ShowWindow(child,SW_SHOWNOACTIVATE);}
        }else if event==EVENT_SYSTEM_CAPTUREEND{unsafe{ShowWindow(child,SW_HIDE);}}
    });
}

pub fn elevated()->Result<bool,String>{
    let mut token=null_mut();
    if unsafe{OpenProcessToken(GetCurrentProcess(),TOKEN_QUERY,&mut token)}==0{return Err(std::io::Error::last_os_error().to_string());}
    let mut value=TOKEN_ELEVATION::default();let mut size=0;
    let ok=unsafe{GetTokenInformation(token,TokenElevation,(&mut value as *mut TOKEN_ELEVATION).cast(),std::mem::size_of::<TOKEN_ELEVATION>()as u32,&mut size)};
    let error=std::io::Error::last_os_error();unsafe{CloseHandle(token);}
    if ok==0{return Err(error.to_string());}Ok(value.TokenIsElevated!=0)
}

pub fn prepare(window:HWND)->Result<CaptureHook,String>{
    unsafe extern "system" fn drop_target(child:HWND,message:u32,w:WPARAM,l:LPARAM)->LRESULT{
        // Parent destruction precedes the stack-owned hook's Drop. Do not let
        // a queued capture event act on a destroyed (or reused) child handle.
        if message==WM_NCDESTROY{DROP_TARGET.with(|target|{if target.get()==child{target.set(null_mut());}});}
        // It is present only during a drag from another window. HTTRANSPARENT
        // cannot pass ordinary clicks across Chromium's process boundary.
        if message==WM_NCHITTEST{return HTCLIENT as LRESULT;}
        if message==WM_ERASEBKGND{return 1;}
        if message==WM_PAINT{let mut paint=PAINTSTRUCT::default();unsafe{BeginPaint(child,&mut paint);EndPaint(child,&paint);}return 0;}
        if message==WM_DROPFILES{
            let result=unsafe{SendMessageW(GetParent(child),WM_DROPFILES,w,child as LPARAM)};
            unsafe{ShowWindow(child,SW_HIDE);}return result;
        }
        unsafe{DefWindowProcW(child,message,w,l)}
    }
    let module=unsafe{GetModuleHandleW(std::ptr::null())};
    let class=WNDCLASSW{lpfnWndProc:Some(drop_target),hInstance:module,lpszClassName:w!("WinOne.FileDropTarget"),..Default::default()};
    if unsafe{RegisterClassW(&class)}==0&&unsafe{GetLastError()}!=ERROR_CLASS_ALREADY_EXISTS{return Err(std::io::Error::last_os_error().to_string());}
    let child=unsafe{CreateWindowExW(WS_EX_TRANSPARENT|WS_EX_NOACTIVATE,class.lpszClassName,w!(""),WS_CHILD,0,0,1,1,window,null_mut(),module,std::ptr::null())};
    if child.is_null(){return Err(std::io::Error::last_os_error().to_string());}
    for message in [WM_DROPFILES,0x0049]{
        if unsafe{ChangeWindowMessageFilterEx(child,message,MSGFLT_ALLOW,null_mut())}==0{return Err(format!("无法启用资源管理器文件拖入：{}",std::io::Error::last_os_error()));}
    }
    unsafe{DragAcceptFiles(child,1);}resize(window)?;
    let hook=unsafe{SetWinEventHook(EVENT_SYSTEM_CAPTURESTART,EVENT_SYSTEM_CAPTUREEND,null_mut(),Some(capture_event),0,0,WINEVENT_OUTOFCONTEXT)};
    if hook.is_null(){return Err(std::io::Error::last_os_error().to_string());}
    DROP_TARGET.with(|target|target.set(child));
    Ok(CaptureHook(hook))
}

pub fn resize(window:HWND)->Result<(),String>{
    let child=unsafe{FindWindowExW(window,null_mut(),w!("WinOne.FileDropTarget"),std::ptr::null())};
    if child.is_null(){return Ok(());}
    let mut rect=RECT::default();
    if unsafe{GetClientRect(window,&mut rect)}==0||unsafe{SetWindowPos(child,HWND_TOP,0,0,rect.right,rect.bottom,SWP_NOACTIVATE)}==0{return Err(std::io::Error::last_os_error().to_string());}
    Ok(())
}

pub fn receive(window:HWND,drop:HDROP)->Result<(Vec<String>,(i32,i32)),String>{
    struct DropHandle(HDROP);impl Drop for DropHandle{fn drop(&mut self){unsafe{DragFinish(self.0);}}}
    let _handle=DropHandle(drop);
    let count=unsafe{DragQueryFileW(drop,u32::MAX,null_mut(),0)};
    if count==0||count>1024{return Err("拖入文件数量无效".into());}
    let mut paths=Vec::with_capacity(count as usize);
    for index in 0..count{
        let length=unsafe{DragQueryFileW(drop,index,null_mut(),0)};
        if length==0||length>32767{return Err("拖入文件路径无效".into());}
        let mut buffer=vec![0u16;length as usize+1];
        if unsafe{DragQueryFileW(drop,index,buffer.as_mut_ptr(),length+1)}!=length{return Err("无法读取拖入的文件路径".into());}
        paths.push(String::from_utf16(&buffer[..length as usize]).map_err(|_|"拖入路径编码无效")?);
    }
    let mut point=POINT::default();
    let root=unsafe{GetAncestor(window,GA_ROOT)};
    // DragQueryPoint supplies client-relative coordinates. Its BOOL reports
    // hit classification, not whether the output point exists. The transparent
    // surface receives FALSE even for interior drops, so check actual bounds.
    unsafe{DragQueryPoint(drop,&mut point);}
    if root!=window{unsafe{MapWindowPoints(window,root,&mut point,1);}}
    let mut client=RECT::default();
    if unsafe{GetClientRect(root,&mut client)}==0{return Err(std::io::Error::last_os_error().to_string());}
    if point.x<0||point.y<0||point.x>=client.right||point.y>=client.bottom{
        return Err("请把文件拖入页面中的目标区域".into());
    }
    // The top-level client is also the Wry view's origin.
    // Keep physical pixels; the shared DOM dispatcher applies devicePixelRatio.
    Ok((paths,(point.x,point.y)))
}

#[cfg(test)]
mod tests{
    use super::*;
    use windows_sys::Win32::System::Memory::*;
    fn file_drop(x:i32,y:i32,non_client:i32)->HDROP{
        let name:Vec<u16>="C:\\拖入文件.txt".encode_utf16().chain([0,0]).collect();
        let size=std::mem::size_of::<DROPFILES>();
        unsafe{
            let handle=GlobalAlloc(GMEM_MOVEABLE|GMEM_ZEROINIT,size+name.len()*2);assert!(!handle.is_null());
            let data=GlobalLock(handle).cast::<u8>();assert!(!data.is_null());
            std::ptr::write(data.cast::<DROPFILES>(),DROPFILES{pFiles:size as u32,pt:POINT{x,y},fNC:non_client,fWide:1});
            std::ptr::copy_nonoverlapping(name.as_ptr().cast::<u8>(),data.add(size),name.len()*2);
            GlobalUnlock(handle);handle as HDROP
        }
    }
    #[test]
    fn transparent_surface_drop_keeps_client_coordinates_and_rejects_outside(){
        // Hidden native window: no desktop input, real Shell handle parsing.
        let window=unsafe{CreateWindowExW(0,w!("STATIC"),w!(""),WS_POPUP,1080,276,1280,840,null_mut(),null_mut(),null_mut(),std::ptr::null())};
        assert!(!window.is_null());
        struct Window(HWND);impl Drop for Window{fn drop(&mut self){unsafe{DestroyWindow(self.0);}}}
        let _window=Window(window);
        for non_client in [0,1]{
            let (paths,point)=receive(window,file_drop(365,494,non_client)).unwrap();
            assert_eq!(paths,["C:\\拖入文件.txt"]);assert_eq!(point,(365,494));
        }
        assert!(receive(window,file_drop(-1,494,1)).is_err());
        assert!(receive(window,file_drop(1280,494,0)).is_err());
    }
}
