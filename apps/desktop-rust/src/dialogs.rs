//! Shared native file selection. Dialog ownership preserves native keyboard,
//! cancellation and focus behavior; selection never starts or saves a program.
use crate::windows_files::wide;
use windows_sys::{core::GUID, Win32::{Foundation::HWND, System::Com::CoCreateGuid, UI::Controls::Dialogs::*}};

/// CSS preferences use RGB; Win32 COLORREF stores those channels in reverse order.
pub fn color_ref(value: &str) -> Result<u32,String> {
    if value.len()!=7 || !value.starts_with('#') || !value.as_bytes()[1..].iter().all(u8::is_ascii_hexdigit) {
        return Err("边框颜色无效，请重新读取设置。".into());
    }
    let rgb=u32::from_str_radix(&value[1..],16).map_err(|e|e.to_string())?;
    Ok((rgb&255)<<16 | (rgb&0xff00) | (rgb>>16)&255)
}

pub fn pick_color(owner:HWND,current:&str)->Result<Option<String>,String>{
    let mut custom=[0xffffffu32;16];
    let mut dialog=CHOOSECOLORW {lStructSize:std::mem::size_of::<CHOOSECOLORW>() as u32,
        hwndOwner:owner,rgbResult:color_ref(current)?,lpCustColors:custom.as_mut_ptr(),Flags:CC_RGBINIT|CC_FULLOPEN,..Default::default()};
    if unsafe{ChooseColorW(&mut dialog)}==0{
        let error=unsafe{CommDlgExtendedError()};
        return if error==0{Ok(None)}else{Err(format!("无法选择颜色：{error:#x}"))};
    }
    let color=dialog.rgbResult;
    Ok(Some(format!("#{:02x}{:02x}{:02x}",color&255,(color>>8)&255,(color>>16)&255)))
}

#[cfg(test)]
mod color_tests{
    #[test]
    fn release_links_reject_other_schemes_hosts_and_paths(){
        assert!(super::valid_release_url("https://github.com/example/app/releases/tag/v1.2.3"));
        for value in ["file:///C:/program.exe","https://github.com.evil.test/example/app/releases/tag/v1.2.3","https://github.com/example/app/releases/tag/v1.2.3?redirect=evil","https://github.com/example/app/issues/1","https://github.com/example/../releases/tag/v1.2.3"]{
            assert!(!super::valid_release_url(value),"{value}");
        }
    }
    #[test]
    fn css_channels_are_preserved_and_invalid_colors_are_rejected(){
        assert_eq!(super::color_ref("#123456").unwrap(),0x563412);
        assert_eq!(super::color_ref("#AaBBcC").unwrap(),0xccbbaa);
        for value in ["#12","123456","#ffGG00","#00000000","#红色"," #000000"]{assert!(super::color_ref(value).is_err());}
    }
}

/// Only the Host's validated diagnostics folder reaches this explicit UI action.
pub fn open_folder(owner: HWND, path: &str) -> Result<(), String> {
    use windows_sys::{core::w, Win32::UI::{Shell::ShellExecuteW, WindowsAndMessaging::SW_SHOWNORMAL}};
    if path.contains('\0') || !std::path::Path::new(path).is_absolute() { return Err("日志文件夹路径无效".into()); }
    let result = unsafe { ShellExecuteW(owner, w!("open"), wide(path).as_ptr(), std::ptr::null(), std::ptr::null(), SW_SHOWNORMAL) } as isize;
    if result <= 32 { return Err(format!("无法打开日志文件夹：{result}")); }
    Ok(())
}

pub(crate) fn valid_release_url(value:&str)->bool{
    let Some(path)=value.strip_prefix("https://github.com/")else{return false;};
    let parts:Vec<_>=path.split('/').collect();
    if parts.len()!=5 || parts[2]!="releases" || parts[3]!="tag"{return false;}
    let owner=parts[0].as_bytes();let repository=parts[1].as_bytes();let tag=parts[4].as_bytes();
    !owner.is_empty()&&owner.len()<=39&&owner.first().unwrap().is_ascii_alphanumeric()&&owner.last().unwrap().is_ascii_alphanumeric()
        &&owner.iter().all(|b|b.is_ascii_alphanumeric()||*b==b'-')
        &&!repository.is_empty()&&repository.len()<=100&&parts[1]!="."&&parts[1]!=".."
        &&repository.iter().all(|b|b.is_ascii_alphanumeric()||b"_.-".contains(b))
        &&!tag.is_empty()&&tag.iter().all(|b|b.is_ascii_alphanumeric()||b".-".contains(b))
}

/// Only a checked release returned for the current update request reaches here.
pub fn open_update_release(owner:HWND,address:&str)->Result<(),String>{
    use windows_sys::{core::w,Win32::UI::{Shell::ShellExecuteW,WindowsAndMessaging::SW_SHOWNORMAL}};
    if !valid_release_url(address){return Err("更新发布页无效".into());}
    let result=unsafe{ShellExecuteW(owner,w!("open"),wide(address).as_ptr(),std::ptr::null(),std::ptr::null(),SW_SHOWNORMAL)} as isize;
    if result<=32{return Err(format!("无法打开发布页：{result}"));}Ok(())
}

pub fn new_program_id() -> Result<String,String> {
    let mut guid = GUID::from_u128(0);
    let result = unsafe { CoCreateGuid(&mut guid) };
    if result < 0 { return Err(format!("无法生成程序标识：{result:#x}")); }
    Ok(format!("{:08x}-{:04x}-{:04x}-{:02x}{:02x}-{:02x}{:02x}{:02x}{:02x}{:02x}{:02x}",
        guid.data1,guid.data2,guid.data3,guid.data4[0],guid.data4[1],guid.data4[2],guid.data4[3],guid.data4[4],guid.data4[5],guid.data4[6],guid.data4[7]))
}

pub fn pick_programs(owner: HWND, title: &str, multiple: bool) -> Result<Vec<String>,String> {
    pick_programs_labeled(owner,title,multiple,"Programs and scripts")
}
pub fn pick_programs_labeled(owner: HWND, title: &str, multiple: bool,label:&str) -> Result<Vec<String>,String> {
    let title = wide(title);
    let filter = wide(&format!("{label}\0*.exe;*.bat;*.cmd;*.ps1;*.py;*.pyw\0All files\0*.*\0"));
    let mut buffer = vec![0u16;65536];
    let mut dialog: OPENFILENAMEW = unsafe { std::mem::zeroed() };
    dialog.lStructSize = std::mem::size_of::<OPENFILENAMEW>() as u32;
    dialog.hwndOwner = owner; dialog.lpstrTitle = title.as_ptr(); dialog.lpstrFilter = filter.as_ptr();
    dialog.lpstrFile = buffer.as_mut_ptr(); dialog.nMaxFile = buffer.len() as u32;
    dialog.Flags = OFN_EXPLORER | OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST | OFN_NOCHANGEDIR | OFN_DONTADDTORECENT
        | if multiple { OFN_ALLOWMULTISELECT } else { 0 };
    if unsafe { GetOpenFileNameW(&mut dialog) } == 0 {
        let error = unsafe { CommDlgExtendedError() };
        return if error == 0 { Ok(Vec::new()) } else { Err(format!("无法选择程序：{error:#x}")) };
    }
    let parts = buffer.split(|c| *c == 0).take_while(|s| !s.is_empty()).map(String::from_utf16)
        .collect::<Result<Vec<_>,_>>().map_err(|e| e.to_string())?;
    if parts.len() <= 1 { return Ok(parts); }
    Ok(parts[1..].iter().map(|name| std::path::Path::new(&parts[0]).join(name).to_string_lossy().into_owned()).collect())
}

/// Choose a .winone configuration file to import. Cancellation returns None.
pub fn pick_profile(owner: HWND, title: &str) -> Result<Option<String>,String> {
    pick_profile_labeled(owner,title,"WinOne configuration")
}
pub fn pick_profile_labeled(owner: HWND, title: &str,label:&str) -> Result<Option<String>,String> {
    pick_single_file(owner,title,&format!("{label}\0*.winone\0All files\0*.*\0"))
}

pub fn pick_module_package(owner:HWND,title:&str,label:&str)->Result<Option<String>,String>{
    pick_single_file(owner,title,&format!("{label}\0*.winone-module;*.winone-runtime;*.zip\0All files\0*.*\0"))
}

fn pick_single_file(owner:HWND,title:&str,filter:&str)->Result<Option<String>,String>{
    let title = wide(title);
    let filter = wide(filter);
    let mut buffer = vec![0u16;32768];
    let mut dialog: OPENFILENAMEW = unsafe { std::mem::zeroed() };
    dialog.lStructSize = std::mem::size_of::<OPENFILENAMEW>() as u32;
    dialog.hwndOwner = owner; dialog.lpstrTitle = title.as_ptr(); dialog.lpstrFilter = filter.as_ptr();
    dialog.lpstrFile = buffer.as_mut_ptr(); dialog.nMaxFile = buffer.len() as u32;
    dialog.Flags = OFN_EXPLORER | OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST | OFN_NOCHANGEDIR | OFN_HIDEREADONLY | OFN_DONTADDTORECENT;
    if unsafe { GetOpenFileNameW(&mut dialog) } == 0 {
        let error = unsafe { CommDlgExtendedError() };
        return if error == 0 { Ok(None) } else { Err(format!("无法选择配置文件：{error:#x}")) };
    }
    Ok(Some(read_dialog_path(&buffer)))
}

/// Choose where to save an exported profile. Cancellation returns None and the
/// suggested extension is appended when the name carries none.
pub fn save_profile(owner: HWND, title: &str) -> Result<Option<String>,String> {
    save_profile_labeled(owner,title,"WinOne configuration")
}
pub fn save_profile_labeled(owner: HWND, title: &str,label:&str) -> Result<Option<String>,String> {
    save_file(owner,title,&format!("{label}\0*.winone\0All files\0*.*\0"),"winone","WinOne.winone")
}

/// Choose the path before the worker validates the saved-capture identity.
pub fn save_png(owner: HWND, title: &str, suggested: &str) -> Result<Option<String>,String> {
    save_png_labeled(owner,title,suggested,"PNG image")
}
pub fn save_png_labeled(owner: HWND,title:&str,suggested:&str,label:&str)->Result<Option<String>,String>{
    save_file(owner,title,&format!("{label}\0*.png\0"),"png",suggested)
}

/// The native save dialog adds .csv before asking about an existing file.
pub fn save_csv_labeled(owner:HWND,title:&str,suggested:&str,label:&str)->Result<Option<String>,String>{
    save_file(owner,title,&format!("{label}\0*.csv\0"),"csv",suggested)
}

/// Apply .cshare before Windows asks for overwrite confirmation.
pub fn save_cshare_labeled(owner:HWND,title:&str,suggested:&str,label:&str)->Result<Option<String>,String>{
    save_file(owner,title,&format!("{label}\0*.cshare\0"),"cshare",suggested)
}

pub(crate) fn save_file(owner: HWND, title: &str, filter: &str, extension: &str, suggested: &str) -> Result<Option<String>,String> {
    let title = wide(title);
    let filter = wide(filter);
    if suggested.contains('\0') { return Err("文件名无效".into()); }
    let name = wide(suggested);
    let extension = wide(extension);
    let mut buffer = vec![0u16;1024];
    if name.len()>buffer.len() { return Err("文件名过长".into()); }
    buffer[..name.len()-1].copy_from_slice(&name[..name.len()-1]);
    let mut dialog: OPENFILENAMEW = unsafe { std::mem::zeroed() };
    dialog.lStructSize = std::mem::size_of::<OPENFILENAMEW>() as u32;
    dialog.hwndOwner = owner; dialog.lpstrTitle = title.as_ptr(); dialog.lpstrFilter = filter.as_ptr();
    dialog.lpstrFile = buffer.as_mut_ptr(); dialog.nMaxFile = buffer.len() as u32;
    // Let the dialog apply the extension before its overwrite confirmation;
    // appending afterwards could overwrite a file the user was never shown.
    dialog.lpstrDefExt = extension.as_ptr();
    dialog.Flags = OFN_EXPLORER | OFN_PATHMUSTEXIST | OFN_OVERWRITEPROMPT | OFN_NOCHANGEDIR | OFN_HIDEREADONLY | OFN_DONTADDTORECENT;
    if unsafe { GetSaveFileNameW(&mut dialog) } == 0 {
        let error = unsafe { CommDlgExtendedError() };
        return if error == 0 { Ok(None) } else { Err(format!("无法选择保存位置：{error:#x}")) };
    }
    Ok(Some(read_dialog_path(&buffer)))
}

fn read_dialog_path(buffer: &[u16]) -> String {
    let length = buffer.iter().position(|c| *c == 0).unwrap_or(buffer.len());
    String::from_utf16_lossy(&buffer[..length])
}

/// Choose a folder for a profile path binding. Cancellation returns None.
pub fn pick_folder(owner: HWND, title: &str) -> Result<Option<String>,String> {
    Ok(pick_native_paths(owner,title,true,false)?.into_iter().next())
}
pub fn pick_directories(owner:HWND,title:&str)->Result<Vec<String>,String>{pick_native_paths(owner,title,true,true)}
fn pick_native_paths(owner:HWND,title:&str,folders:bool,multiple:bool)->Result<Vec<String>,String>{
    use windows::{core::PCWSTR,Win32::{Foundation::HWND as ComHwnd,System::Com::{CoTaskMemFree,CoInitializeEx,CoUninitialize,CoCreateInstance,COINIT_APARTMENTTHREADED,CLSCTX_INPROC_SERVER},UI::Shell::{IFileOpenDialog,FileOpenDialog,FOS_PICKFOLDERS,FOS_ALLOWMULTISELECT,FOS_FORCEFILESYSTEM,FOS_PATHMUSTEXIST,FOS_FILEMUSTEXIST,FOS_NOCHANGEDIR,FOS_DONTADDTORECENT,SIGDN_FILESYSPATH}}};
    // IFileOpenDialog is the Explorer picker with navigation sidebar and a
    // path field. COM objects are released before the STA is uninitialized.
    unsafe{CoInitializeEx(None,COINIT_APARTMENTTHREADED).ok()}.map_err(|e|format!("无法打开文件夹选择器：{e}"))?;
    struct Apartment;
    impl Drop for Apartment { fn drop(&mut self) { unsafe { CoUninitialize(); } } }
    let _apartment = Apartment;
    let title = wide(title);
    let select=||->windows::core::Result<Vec<String>>{unsafe{
        let dialog:IFileOpenDialog=CoCreateInstance(&FileOpenDialog,None,CLSCTX_INPROC_SERVER)?;
        dialog.SetTitle(PCWSTR(title.as_ptr()))?;
        let mut options=dialog.GetOptions()?|FOS_FORCEFILESYSTEM|FOS_PATHMUSTEXIST|FOS_NOCHANGEDIR|FOS_DONTADDTORECENT;
        if folders{options|=FOS_PICKFOLDERS;}else{options|=FOS_FILEMUSTEXIST;}
        if multiple{options|=FOS_ALLOWMULTISELECT;}
        dialog.SetOptions(options)?;
        if let Err(error)=dialog.Show(Some(ComHwnd(owner))){
            if error.code().0==0x800704c7u32 as i32{return Ok(vec![]);}return Err(error);
        }
        let items=dialog.GetResults()?;let mut paths=vec![];
        for i in 0..items.GetCount()?{let item=items.GetItemAt(i)?;let selected=item.GetDisplayName(SIGDN_FILESYSPATH)?;let result=selected.to_string();CoTaskMemFree(Some(selected.0.cast()));paths.push(result?);}
        Ok(paths)
    }};
    select().map_err(|e|format!("无法选择文件夹：{e}"))
}

pub fn pick_chat_files(owner:HWND,title:&str)->Result<Vec<String>,String>{
    pick_native_paths(owner,title,false,true)
}
pub fn pick_file(owner:HWND,title:&str)->Result<Option<String>,String>{
    Ok(pick_native_paths(owner,title,false,false)?.into_iter().next())
}
