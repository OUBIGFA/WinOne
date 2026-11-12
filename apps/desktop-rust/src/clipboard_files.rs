//! Read native file clipboard data for the file-transfer page.
use windows_sys::Win32::{Foundation::HWND,System::DataExchange::{OpenClipboard,CloseClipboard}};
// Explorer may expose only CF_HDROP to WebView2, with no DOM File.path.
// Read names on an explicit paste only; do not consume, clear or free clipboard
// memory. Actual file classification/import remains in the receiving Host.
pub(crate) fn read_clipboard_files(owner:HWND)->Result<Vec<String>,String>{
    use windows_sys::Win32::{System::DataExchange::{GetClipboardData,IsClipboardFormatAvailable},UI::Shell::DragQueryFileW};
    unsafe{
        if IsClipboardFormatAvailable(15)==0{return Ok(Vec::new());}
        if OpenClipboard(owner)==0{return Err("剪贴板正被其他程序使用，请重试".into());}
        let result=(||{
            let drop=GetClipboardData(15);
            if drop.is_null(){return Err(std::io::Error::last_os_error().to_string());}
            let count=DragQueryFileW(drop, u32::MAX, std::ptr::null_mut(), 0);
            if count>16{return Err("一次请选择 1 至 16 个附件".into());}
            let mut paths=Vec::new();
            for index in 0..count{
                let length=DragQueryFileW(drop,index,std::ptr::null_mut(),0);
                if length==0||length>32767{return Err("剪贴板文件位置无效".into());}
                let mut path=vec![0u16;length as usize+1];
                if DragQueryFileW(drop,index,path.as_mut_ptr(),length+1)!=length{return Err("剪贴板文件读取不完整".into());}
                paths.push(String::from_utf16(&path[..length as usize]).map_err(|_|"剪贴板文件位置无效".to_string())?);
            }
            Ok(paths)
        })();
        CloseClipboard();result
    }
}
