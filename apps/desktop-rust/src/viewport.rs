//! Native clipping and scroll routing without replacing child-control behavior.
use crate::widgets::{Palette,wide};
use std::ptr::{null,null_mut};
use windows_sys::{core::w,Win32::{Foundation::*,Graphics::Gdi::*,System::LibraryLoader::GetModuleHandleW,UI::{WindowsAndMessaging::*,Controls::SetScrollInfo,Shell::{SetWindowSubclass,RemoveWindowSubclass,DefSubclassProc}}}};

pub const SCROLL:u32=WM_APP+6;
pub const WHEEL:u32=WM_APP+7;
pub const FOCUS:u32=WM_APP+9;
pub struct Viewport{pub window:HWND}
impl Viewport{
    pub fn new(parent:HWND,title:&str)->Result<Self,String>{
        let instance=unsafe{GetModuleHandleW(null())};let mut found=WNDCLASSW::default();
        if unsafe{GetClassInfoW(instance,w!("WinOne.ScrollViewport"),&mut found)}==0{
            let class=WNDCLASSW{lpfnWndProc:Some(viewport_proc),hInstance:instance,hCursor:unsafe{LoadCursorW(null_mut(),IDC_ARROW)},lpszClassName:w!("WinOne.ScrollViewport"),..Default::default()};
            if unsafe{RegisterClassW(&class)}==0{return Err(std::io::Error::last_os_error().to_string());}
        }
        let window=unsafe{CreateWindowExW(WS_EX_CONTROLPARENT,w!("WinOne.ScrollViewport"),wide(title).as_ptr(),
            WS_CHILD|WS_VISIBLE|WS_CLIPCHILDREN|WS_CLIPSIBLINGS|WS_VSCROLL,0,0,1,1,parent,null_mut(),instance,null())};
        if window.is_null(){return Err(std::io::Error::last_os_error().to_string());}
        Ok(Self{window})
    }
    pub fn place(&self,palette:&Palette,rect:RECT){unsafe{SetWindowPos(self.window,null_mut(),palette.px(rect.left),palette.px(rect.top),palette.px(rect.right-rect.left),palette.px(rect.bottom-rect.top),SWP_NOZORDER|SWP_NOACTIVATE);}}
    pub fn range(&self,height:i32,page:i32,position:i32){
        let info=SCROLLINFO{cbSize:std::mem::size_of::<SCROLLINFO>()as u32,fMask:SIF_RANGE|SIF_PAGE|SIF_POS,nMin:0,nMax:height.max(1)-1,nPage:page.max(1)as u32,nPos:position,..Default::default()};
        unsafe{SetScrollInfo(self.window,SB_VERT,&info,1);}
    }
}
/// Keep the standard control's focus/input behavior; notify the owner only so
/// keyboard navigation can reveal a child clipped by this viewport.
pub fn observe_focus(window:HWND)->Result<(),String>{
    if unsafe{SetWindowSubclass(window,Some(child_proc),1,0)}==0{return Err("无法注册详情焦点处理".into());}Ok(())
}
unsafe extern "system" fn child_proc(window:HWND,message:u32,w:WPARAM,l:LPARAM,id:usize,_data:usize)->LRESULT{
    if message==WM_SETFOCUS{unsafe{SendMessageW(GetParent(GetParent(window)),FOCUS,0,window as isize);}}
    if message==WM_NCDESTROY{unsafe{RemoveWindowSubclass(window,Some(child_proc),id);}}
    unsafe{DefSubclassProc(window,message,w,l)}
}
impl Drop for Viewport{fn drop(&mut self){unsafe{DestroyWindow(self.window);}}}
unsafe extern "system" fn viewport_proc(window:HWND,message:u32,w:WPARAM,l:LPARAM)->LRESULT{
    let parent=unsafe{GetParent(window)};
    match message{
        WM_COMMAND|WM_NOTIFY|WM_DRAWITEM|WM_MEASUREITEM|WM_CTLCOLORSTATIC|WM_CTLCOLOREDIT|WM_CTLCOLORBTN|WM_CTLCOLORLISTBOX=>unsafe{SendMessageW(parent,message,w,l)},
        WM_VSCROLL=>unsafe{SendMessageW(parent,SCROLL,w,window as isize)},
        WM_MOUSEWHEEL=>unsafe{SendMessageW(parent,WHEEL,w,window as isize)},
        WM_ERASEBKGND|WM_PRINTCLIENT=>unsafe{
            let brush=SendMessageW(parent,WM_CTLCOLORSTATIC,w,window as isize)as HBRUSH;
            let mut rect=RECT::default();GetClientRect(window,&mut rect);
            FillRect(w as HDC,&rect,if brush.is_null(){GetSysColorBrush(COLOR_WINDOW)}else{brush});1
        },
        _=>unsafe{DefWindowProcW(window,message,w,l)},
    }
}
