//! Shared Windows controls and a flat palette; all native pages use this host.
use std::ptr::{null, null_mut};
use windows_sys::{core::w, Win32::{Foundation::*, Graphics::{Gdi::*, Dwm::*}, System::{LibraryLoader::GetModuleHandleW, Registry::*}, UI::{Controls::*, Accessibility::*, HiDpi::*, WindowsAndMessaging::*}}};

pub use crate::windows_files::wide;
pub fn text(window: HWND) -> String {
    let size = unsafe { GetWindowTextLengthW(window) }.max(0) as usize;
    let mut bytes = vec![0; size + 1];
    let used = unsafe { GetWindowTextW(window, bytes.as_mut_ptr(), bytes.len() as i32) }.max(0) as usize;
    String::from_utf16_lossy(&bytes[..used])
}
pub fn set_text(window: HWND, text: &str) { unsafe { SetWindowTextW(window, wide(text).as_ptr()); } }
pub fn enable(window: HWND, enabled: bool) { unsafe { windows_sys::Win32::UI::Input::KeyboardAndMouse::EnableWindow(window, enabled as i32); } }
pub fn content_width(window:HWND,scale:f32)->i32{
    let mut client=RECT::default();unsafe{GetClientRect(window,&mut client);}
    let width=(client.right as f32/scale)as i32;
    width-SIDEBAR_WIDTH-64
}
pub const SIDEBAR_WIDTH:i32=208;
pub fn wrapped_height(window:HWND,font:HFONT,text:&str,width:i32,scale:f32)->i32{
    unsafe{
        let dc=GetDC(window);let old=SelectObject(dc,font);let mut rect=RECT{left:0,top:0,right:(width as f32*scale)as i32,bottom:0};
        DrawTextW(dc,wide(text).as_ptr(),-1,&mut rect,DT_CALCRECT|DT_WORDBREAK|DT_NOPREFIX);
        SelectObject(dc,old);ReleaseDC(window,dc);
        ((rect.bottom-rect.top)as f32/scale).ceil()as i32
    }
}

pub struct Palette { pub background: COLORREF, pub sidebar: COLORREF, pub foreground: COLORREF, pub selected: COLORREF, pub selected_foreground:COLORREF, pub muted: COLORREF, pub brush: HBRUSH, pub sidebar_brush:HBRUSH, pub font: HFONT, pub heading: HFONT, pub scale: f32, pub dark:bool }
impl Palette {
    pub fn new(window: HWND, theme: &str) -> Self {
        let dpi = unsafe { GetDpiForWindow(window) }.max(96);
        let mut light = 1u32; let mut size = 4u32;
        unsafe { RegGetValueW(HKEY_CURRENT_USER, w!("Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize"), w!("AppsUseLightTheme"), RRF_RT_REG_DWORD, null_mut(), (&mut light as *mut u32).cast(), &mut size); }
        let mut contrast: HIGHCONTRASTW = unsafe { std::mem::zeroed() };
        contrast.cbSize = std::mem::size_of::<HIGHCONTRASTW>() as u32;
        unsafe { SystemParametersInfoW(SPI_GETHIGHCONTRAST, contrast.cbSize, (&mut contrast as *mut HIGHCONTRASTW).cast(), 0); }
        let high_contrast = contrast.dwFlags & HCF_HIGHCONTRASTON != 0;
        let dark = !high_contrast && (theme == "dark" || theme == "system" && light == 0);
        let (background, sidebar, foreground, selected, muted) = unsafe {
            if high_contrast { (GetSysColor(COLOR_WINDOW), GetSysColor(COLOR_BTNFACE), GetSysColor(COLOR_WINDOWTEXT), GetSysColor(COLOR_HIGHLIGHT), GetSysColor(COLOR_GRAYTEXT)) }
            else if dark { (0x181818, 0x202020, 0xf5f5f5, 0x383838, 0xb0b0b0) }
            else { (0xffffff, 0xfafafa, 0x202020, 0xe7e7e7, 0x606060) }
        };
        let native_dark = dark as i32;
        unsafe { DwmSetWindowAttribute(window, DWMWA_USE_IMMERSIVE_DARK_MODE as u32, (&native_dark as *const i32).cast(), 4); }
        let scale = dpi as f32 / 96.0;
        let make_font = |height: f32, weight| unsafe { CreateFontW(-(height * scale).round() as i32, 0, 0, 0, weight, 0, 0, 0, DEFAULT_CHARSET as u32, 0, 0, CLEARTYPE_QUALITY as u32, 0, w!("Segoe UI")) };
        Self { background, sidebar, foreground, selected, selected_foreground:if high_contrast{unsafe{GetSysColor(COLOR_HIGHLIGHTTEXT)}}else{foreground}, muted,
            brush: unsafe { CreateSolidBrush(background) },sidebar_brush:unsafe{CreateSolidBrush(sidebar)}, font: make_font(14.0, 400), heading: make_font(24.0, 600), scale, dark }
    }
    pub fn px(&self, value: i32) -> i32 { (value as f32 * self.scale).round() as i32 }
}

pub fn apply_control_theme(window:HWND,class:&str,palette:&Palette)->Result<(),String>{
    if matches!(class,"BUTTON"|"COMBOBOX"|"EDIT"|"LISTBOX"){
        let theme=if !palette.dark{null()}else if class=="COMBOBOX"{w!("DarkMode_CFD")}else{w!("DarkMode_Explorer")};
        let result=unsafe{SetWindowTheme(window,theme,null())};
        if result<0{return Err(format!("无法应用控件主题：{result:#x}"));}
    }
    Ok(())
}
impl Drop for Palette {
    fn drop(&mut self) { unsafe { DeleteObject(self.brush);DeleteObject(self.sidebar_brush); DeleteObject(self.font); DeleteObject(self.heading); } }
}

/// The system Rich Edit control lays out bounded logs much faster than EDIT.
/// Keep one library reference per UI host, after its controls in drop order.
pub struct RichEdit(HMODULE);
impl RichEdit {
    pub fn load() -> Result<Self, String> {
        use windows_sys::Win32::System::LibraryLoader::{LoadLibraryExW, LOAD_LIBRARY_SEARCH_SYSTEM32};
        let library = unsafe { LoadLibraryExW(w!("Msftedit.dll"), null_mut(), LOAD_LIBRARY_SEARCH_SYSTEM32) };
        if library.is_null() { return Err(format!("无法加载日志控件：{}", std::io::Error::last_os_error())); }
        Ok(Self(library))
    }
    pub fn configure_log(&self, window: HWND, palette: &Palette) -> Result<(), String> {
        // SDK Richedit.h: CHARFORMATW and these messages are not exposed by
        // windows-sys. Plain-text mode prevents formatting or embedded content.
        #[repr(C)]
        #[derive(Default)]
        struct CharFormat { size: u32, mask: u32, effects: u32, height: i32, offset: i32, color: COLORREF, charset: u8, pitch: u8, face: [u16; 32] }
        const EM_SETTEXTMODE: u32 = WM_USER + 89;
        const EM_SETBKGNDCOLOR: u32 = WM_USER + 67;
        const EM_SETCHARFORMAT: u32 = WM_USER + 68;
        const TM_PLAINTEXT: usize = 1;
        let format = CharFormat { size: std::mem::size_of::<CharFormat>() as u32, mask: 0x40000000, color: palette.foreground, ..Default::default() };
        unsafe {
            if SendMessageW(window, EM_SETTEXTMODE, TM_PLAINTEXT, 0) != 0 { return Err("无法初始化纯文本日志控件".into()); }
            SendMessageW(window, EM_SETBKGNDCOLOR, 0, palette.background as isize);
            if SendMessageW(window, EM_SETCHARFORMAT, 0, (&format as *const CharFormat) as isize) == 0 { return Err("无法应用日志文字颜色".into()); }
        }
        Ok(())
    }
}
impl Drop for RichEdit { fn drop(&mut self) { unsafe { FreeLibrary(self.0); } } }

#[derive(Clone, Copy)]
pub enum Area { Sidebar, Bottom, Content, ProgramList, ProgramDetail, ProgramFooter(usize), Tabs { index: usize, count: usize } }
pub struct Control { pub window: HWND, pub id: usize, pub area: Area, pub x: i32, pub y: i32, pub width: i32, pub height: i32, pub body:bool }
impl Control {
    pub fn new(parent: HWND, class: &str, title: &str, id: usize, style: u32, font: HFONT, area: Area, rect: [i32; 4]) -> Result<Self, String> {
        let window = unsafe { CreateWindowExW(0, wide(class).as_ptr(), wide(title).as_ptr(), WS_CHILD | WS_VISIBLE | style, 0, 0, 1, 1, parent, id as HMENU, GetModuleHandleW(null()), null()) };
        if window.is_null() { return Err(format!("无法创建界面控件：{}", std::io::Error::last_os_error())); }
        unsafe { SendMessageW(window, WM_SETFONT, font as usize, 0); }
        Ok(Self { window, id, area, x: rect[0], y: rect[1], width: rect[2], height: rect[3],body:false })
    }
    pub fn button(parent: HWND, title: &str, id: usize, palette: &Palette, area: Area, rect: [i32; 4]) -> Result<Self, String> {
        Self::new(parent, "BUTTON", title, id, WS_TABSTOP | BS_OWNERDRAW as u32, palette.font, area, rect)
    }
    pub fn place(&self, palette: &Palette, client: RECT, scroll: i32) {
        let logical_width = (client.right as f32 / palette.scale) as i32;
        let logical_height = (client.bottom as f32 / palette.scale) as i32;
        let sidebar = SIDEBAR_WIDTH;
        let (origin, y, available) = match self.area {
            Area::Sidebar => (12, self.y, sidebar - 24),
            Area::Bottom => (12, logical_height - self.height - 16, sidebar - 24),
            Area::Content => (sidebar + 32, self.y - scroll, logical_width - sidebar - 64),
            Area::ProgramList|Area::ProgramFooter(_) => (sidebar + 32, self.y - scroll, (logical_width - sidebar - 88) / 2),
            Area::ProgramDetail => (sidebar + 56 + (logical_width - sidebar - 88) / 2, self.y - scroll, (logical_width - sidebar - 88) / 2),
            Area::Tabs { index, count } => {
                let width = (logical_width - sidebar - 64).max(1) / count.max(1) as i32;
                (sidebar + 32 + index as i32 * width, self.y - scroll, width)
            }
        };
        let width = if self.width == 0 { available - self.x } else { self.width.min(available - self.x) }.max(1);
        unsafe { SetWindowPos(self.window, null_mut(), palette.px(origin + self.x), palette.px(y), palette.px(width), palette.px(self.height), SWP_NOZORDER | SWP_NOACTIVATE); }
    }
    pub fn place_local(&self,palette:&Palette,width:i32,scroll:i32){
        let available=(width-self.x).max(1);let width=if self.width==0{available}else{self.width.min(available)};
        unsafe{SetWindowPos(self.window,null_mut(),palette.px(self.x),palette.px(self.y-scroll),palette.px(width),palette.px(self.height),SWP_NOZORDER|SWP_NOACTIVATE);}
    }
    pub fn place_rect(&self,palette:&Palette,rect:RECT){
        unsafe{SetWindowPos(self.window,null_mut(),palette.px(rect.left),palette.px(rect.top),palette.px(rect.right-rect.left),palette.px(rect.bottom-rect.top),SWP_NOZORDER|SWP_NOACTIVATE);}
    }
}
impl Drop for Control { fn drop(&mut self) { unsafe { DestroyWindow(self.window); } } }

pub fn draw_button(item: &DRAWITEMSTRUCT, palette: &Palette, selected: bool, sidebar:bool) {
    unsafe {
        let selected=selected||item.itemState&ODS_SELECTED!=0;
        let background = if selected { palette.selected } else if sidebar {palette.sidebar} else { palette.background };
        let brush = CreateSolidBrush(background); FillRect(item.hDC, &item.rcItem, brush); DeleteObject(brush);
        SetBkMode(item.hDC, TRANSPARENT as i32);
        SetTextColor(item.hDC, if item.itemState & ODS_DISABLED != 0 { palette.muted } else if selected {palette.selected_foreground} else { palette.foreground });
        let old_font = SelectObject(item.hDC, palette.font);
        let mut rect = item.rcItem; rect.left += palette.px(12); rect.right -= palette.px(8);
        let label = wide(&text(item.hwndItem));
        DrawTextW(item.hDC, label.as_ptr(), -1, &mut rect, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_END_ELLIPSIS);
        if item.itemState & ODS_FOCUS != 0 { DrawFocusRect(item.hDC, &item.rcItem); }
        SelectObject(item.hDC, old_font);
    }
}

pub fn draw_color_swatch(item:&DRAWITEMSTRUCT,palette:&Palette,color:COLORREF){
    let mut rect=item.rcItem;
    rect.left=(rect.right-palette.px(40)).max(rect.left);rect.right-=palette.px(10);
    rect.top+=palette.px(8);rect.bottom-=palette.px(8);
    unsafe{
        let fill=CreateSolidBrush(if item.itemState&ODS_DISABLED!=0{palette.muted}else{color});
        FillRect(item.hDC,&rect,fill);DeleteObject(fill);
        let border=CreateSolidBrush(palette.foreground);FrameRect(item.hDC,&rect,border);DeleteObject(border);
    }
}
