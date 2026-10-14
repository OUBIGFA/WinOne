//! Settings-window identity is shared with the existing Host and installer.
//! This module owns only window presence and single-instance notification; it
//! neither starts engines nor grants administrator rights.
use crate::{icon::sha256_hex, windows_files::wide};
use serde_json::{Value, json};
use std::{path::Path, ptr::{null, null_mut}, time::{Duration, Instant}};
use windows_sys::Win32::{Foundation::*, Security::*, System::{DataExchange::COPYDATASTRUCT, LibraryLoader::GetModuleHandleW, Threading::*}, UI::WindowsAndMessaging::*};

pub const WAKE: u32 = WM_APP + 8;
const COPY_INSTANCE: usize = 1542;
const MAX_NOTIFICATION: usize = 65536;

struct Handle(HANDLE);
impl Drop for Handle { fn drop(&mut self) { unsafe { CloseHandle(self.0); } } }

pub struct Instance {
    _mutex: Handle,
    _presence: Handle,
    notification: HWND,
    class: Vec<u16>,
    title: Vec<u16>,
}

/// Match residency.windowName and windowinstance.FindPID, including Wails'
/// message-only endpoint. Keeping the wire identity avoids a separate Host UI.
pub fn names(pipe: &str) -> (String, String, String, String) {
    let hash = sha256_hex(pipe.as_bytes());
    let base = format!("wails-app-winone-desktop-{hash}");
    (format!("{base}sim"), format!("{base}-sic"), format!("{base}-siw"), format!("Local\\WinOne-Desktop-{hash}"))
}

impl Instance {
    /// None means an existing, verified window accepted the notification.
    /// A starting or unresponsive instance is an error, never permission to
    /// create a second settings window for the same Host.
    pub fn claim(pipe: &str, open_settings: bool) -> Result<Option<Self>, String> {
        let (mutex_name, class, title, presence) = names(pipe);
        let mutex = unsafe { CreateMutexW(null(), 0, wide(&mutex_name).as_ptr()) };
        if mutex.is_null() { return Err(std::io::Error::last_os_error().to_string()); }
        let already_exists = unsafe { GetLastError() } == ERROR_ALREADY_EXISTS;
        let mutex = Handle(mutex);
        let class = wide(&class); let title = wide(&title);
        if already_exists {
            let deadline = Instant::now() + Duration::from_secs(5);
            loop {
                let window = unsafe { FindWindowExW(HWND_MESSAGE, null_mut(), class.as_ptr(), title.as_ptr()) };
                if !window.is_null() { notify(window, open_settings)?; return Ok(None); }
                if Instant::now() >= deadline { return Err("设置窗口尚未就绪，请稍后重试。".into()); }
                std::thread::sleep(Duration::from_millis(25));
            }
        }
        let event = unsafe { CreateEventW(null(), 1, 1, wide(&presence).as_ptr()) };
        if event.is_null() { return Err(std::io::Error::last_os_error().to_string()); }
        Ok(Some(Self { _mutex:mutex, _presence:Handle(event), notification:null_mut(), class, title }))
    }

    pub fn attach(&mut self, target: HWND) -> Result<(), String> {
        let module = unsafe { GetModuleHandleW(null()) };
        let registration = WNDCLASSW { lpfnWndProc:Some(notification_proc), hInstance:module, lpszClassName:self.class.as_ptr(), ..Default::default() };
        if unsafe { RegisterClassW(&registration) } == 0 { return Err(std::io::Error::last_os_error().to_string()); }
        self.notification = unsafe { CreateWindowExW(0, self.class.as_ptr(), self.title.as_ptr(), 0, 0,0,0,0,
            HWND_MESSAGE, null_mut(), module, target.cast()) };
        if self.notification.is_null() { return Err(std::io::Error::last_os_error().to_string()); }
        Ok(())
    }
}

impl Drop for Instance {
    fn drop(&mut self) {
        unsafe {
            if !self.notification.is_null() { DestroyWindow(self.notification); }
            UnregisterClassW(self.class.as_ptr(), GetModuleHandleW(null()));
        }
    }
}

fn token_user(process: HANDLE) -> Result<(Handle, Vec<u64>), String> {
    let mut token = null_mut();
    if unsafe { OpenProcessToken(process, TOKEN_QUERY, &mut token) } == 0 { return Err(std::io::Error::last_os_error().to_string()); }
    let token = Handle(token);
    let mut needed = 0;
    unsafe { GetTokenInformation(token.0, TokenUser, null_mut(), 0, &mut needed); }
    if needed == 0 || needed > 65536 { return Err("无法核实窗口账户。".into()); }
    // TOKEN_USER contains pointers and must be naturally aligned.
    let mut data = vec![0u64; (needed as usize).div_ceil(8)];
    if unsafe { GetTokenInformation(token.0, TokenUser, data.as_mut_ptr().cast(), needed, &mut needed) } == 0 {
        return Err(std::io::Error::last_os_error().to_string());
    }
    Ok((token, data))
}

fn verified_process(window: HWND) -> Result<Handle, String> {
    let mut pid = 0;
    unsafe { GetWindowThreadProcessId(window, &mut pid); }
    let process = Handle(unsafe { OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION | PROCESS_SYNCHRONIZE, 0, pid) });
    if process.0.is_null() { return Err(std::io::Error::last_os_error().to_string()); }
    let mut path = vec![0u16;32768]; let mut size = path.len() as u32;
    if unsafe { QueryFullProcessImageNameW(process.0, 0, path.as_mut_ptr(), &mut size) } == 0 { return Err(std::io::Error::last_os_error().to_string()); }
    let actual = String::from_utf16(&path[..size as usize]).map_err(|e| e.to_string())?;
    let expected = std::env::current_exe().map_err(|e| e.to_string())?;
    let actual = Path::new(&actual).canonicalize().map_err(|e| e.to_string())?;
    let expected = expected.canonicalize().map_err(|e| e.to_string())?;
    if !actual.to_string_lossy().eq_ignore_ascii_case(&expected.to_string_lossy()) { return Err("此配置已由另一安装目录的窗口打开。".into()); }
    let (_target_token, target) = token_user(process.0)?;
    let (_current_token, current) = token_user(unsafe { GetCurrentProcess() })?;
    let same_user = unsafe { EqualSid((*(target.as_ptr().cast::<TOKEN_USER>())).User.Sid, (*(current.as_ptr().cast::<TOKEN_USER>())).User.Sid) };
    if same_user == 0 { return Err("窗口属于其他 Windows 账户。".into()); }
    if unsafe { WaitForSingleObject(process.0, 0) } != WAIT_TIMEOUT { return Err("设置窗口已经退出，请重试。".into()); }
    let mut current_pid = 0; unsafe { GetWindowThreadProcessId(window, &mut current_pid); }
    if current_pid != pid { return Err("设置窗口已变化，请重试。".into()); }
    Ok(process)
}

fn notify(window: HWND, open_settings: bool) -> Result<(), String> {
    let _process = verified_process(window)?;
    let mut pid = 0; unsafe { GetWindowThreadProcessId(window, &mut pid); AllowSetForegroundWindow(pid); }
    let args: Vec<&str> = if open_settings { vec!["--open-settings"] } else { vec![] };
    let payload = wide(&json!({"Args":args,"WorkingDirectory":""}).to_string());
    let message = COPYDATASTRUCT { dwData:COPY_INSTANCE, cbData:(payload.len()*2) as u32, lpData:payload.as_ptr().cast_mut().cast() };
    let mut result = 0;
    if unsafe { SendMessageTimeoutW(window, WM_COPYDATA, 0, &message as *const _ as isize, SMTO_ABORTIFHUNG | SMTO_BLOCK, 2000, &mut result) } == 0 {
        return Err("已有设置窗口未响应，请稍后重试。".into());
    }
    if result != 1 { return Err("已有窗口未接受打开请求。".into()); }
    Ok(())
}

/// The user-facing launcher still owns foreground permission after delegating
/// to an authorized administrator task. Pass that permission to the actual
/// window before this launcher exits, including when its HWND is still starting.
pub fn activate_redirected(pipe: &str) -> Result<(), String> {
    let (_, class, title, _) = names(pipe);
    let (class,title)=(wide(&class),wide(&title));
    let deadline=Instant::now()+Duration::from_secs(8);
    loop {
        let window=unsafe{FindWindowExW(HWND_MESSAGE,null_mut(),class.as_ptr(),title.as_ptr())};
        if !window.is_null(){
            let _process=verified_process(window)?;
            let mut pid=0;
            unsafe{
                GetWindowThreadProcessId(window,&mut pid);
                AllowSetForegroundWindow(pid);
                // The elevated launcher already sent its trusted open request.
                // Do not send WM_COPYDATA from a lower-integrity launcher (UIPI
                // would reject it). Activate its verified top-level HWND instead.
                let target=GetWindowLongPtrW(window,GWLP_USERDATA) as HWND;
                let mut target_pid=0;GetWindowThreadProcessId(target,&mut target_pid);
                if target_pid==pid && IsWindowVisible(target)!=0 {
                    if IsIconic(target)!=0{ShowWindow(target,SW_RESTORE);}
                    SetForegroundWindow(target);
                }
            }
            return Ok(());
        }
        if Instant::now()>=deadline{return Err("管理员窗口尚未就绪，请再次打开 WinOne。".into());}
        std::thread::sleep(Duration::from_millis(25));
    }
}

fn wake_request(value: &Value) -> Option<bool> {
    let args = value["Args"].as_array()?;
    let mut settings = false;
    let mut index = 0;
    while index < args.len() {
        match args[index].as_str()? {
            "--open-settings" => settings = true,
            "--standard-session" => {},
            "--pipe" | "--data-dir" => { index += 1; args.get(index)?.as_str()?; }
            _ => return None,
        }
        index += 1;
    }
    Some(settings)
}

unsafe extern "system" fn notification_proc(window: HWND, message: u32, wparam: WPARAM, lparam: LPARAM) -> LRESULT {
    if message == WM_NCCREATE {
        let create = unsafe { &*(lparam as *const CREATESTRUCTW) };
        unsafe { SetWindowLongPtrW(window, GWLP_USERDATA, create.lpCreateParams as isize); }
    }
    if message == WM_COPYDATA && lparam != 0 {
        let data = unsafe { &*(lparam as *const COPYDATASTRUCT) };
        // Wails' sender declares one extra byte; read complete UTF-16 units
        // within the declared bound and require an in-bound NUL terminator.
        if data.dwData != COPY_INSTANCE || data.lpData.is_null() || data.cbData < 2 || data.cbData as usize > MAX_NOTIFICATION { return 0; }
        let units = unsafe { std::slice::from_raw_parts(data.lpData.cast::<u16>(), data.cbData as usize/2) };
        let Some(length) = units.iter().position(|u| *u == 0) else { return 0; };
        let request = String::from_utf16(&units[..length]).ok().and_then(|s| serde_json::from_str::<Value>(&s).ok()).and_then(|v| wake_request(&v));
        if let Some(settings) = request {
            let target = unsafe { GetWindowLongPtrW(window, GWLP_USERDATA) } as HWND;
            return unsafe { PostMessageW(target, WAKE, settings as usize, 0) } as isize;
        }
        return 0;
    }
    unsafe { DefWindowProcW(window, message, wparam, lparam) }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn notification_requests_reject_feature_commands_and_broken_arguments() {
        assert_eq!(wake_request(&json!({"Args":["--open-settings","--pipe","local","--data-dir","profile"]})), Some(true));
        assert_eq!(wake_request(&json!({"Args":[]})), Some(false));
        for args in [json!(["--pipe"]), json!(["--capture-draft","abc"]), json!([false]), json!(["--unknown"])] {
            assert_eq!(wake_request(&json!({"Args":args})), None);
        }
    }
}
