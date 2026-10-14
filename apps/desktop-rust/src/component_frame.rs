//! Match the existing Wails 2.14 frameless-with-decorations window policy.
//! Windows draws the border/corners; the existing UI still owns its titlebar.
use windows_sys::{core::w,Win32::{Foundation::HWND,Graphics::Dwm::*,System::Registry::*,UI::{Controls::MARGINS,Accessibility::{HIGHCONTRASTW,HCF_HIGHCONTRASTON},WindowsAndMessaging::*}}};

pub fn decorations(window:HWND)->Result<(),String>{
    // Wails windows/win32/window.go extends one physical pixel on every edge.
    // Returning zero from WM_NCCALCSIZE alone removes these decorations.
    let margins=MARGINS{cxLeftWidth:1,cxRightWidth:1,cyTopHeight:1,cyBottomHeight:1};
    let result=unsafe{DwmExtendFrameIntoClientArea(window,&margins)};
    if result<0{return Err(format!("无法恢复窗口边框：{result:#x}"));}Ok(())
}
pub fn theme(window:HWND,preference:&str)->Result<(),String>{
    let mut contrast:HIGHCONTRASTW=unsafe{std::mem::zeroed()};contrast.cbSize=std::mem::size_of::<HIGHCONTRASTW>()as u32;
    unsafe{SystemParametersInfoW(SPI_GETHIGHCONTRAST,contrast.cbSize,(&mut contrast as *mut HIGHCONTRASTW).cast(),0);}
    if contrast.dwFlags&HCF_HIGHCONTRASTON!=0{return Ok(());}
    let mut light=1u32;let mut size=4u32;
    unsafe{RegGetValueW(HKEY_CURRENT_USER,w!("Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize"),w!("AppsUseLightTheme"),RRF_RT_REG_DWORD,std::ptr::null_mut(),(&mut light as *mut u32).cast(),&mut size);}
    let dark=(preference=="dark"||preference=="system"&&light==0)as i32;
    let result=unsafe{DwmSetWindowAttribute(window,DWMWA_USE_IMMERSIVE_DARK_MODE as u32,(&dark as *const i32).cast(),4)};
    if result<0{return Err(format!("无法设置窗口主题：{result:#x}"));}Ok(())
}
