//! Bounded local named-pipe transport.
use crate::protocol::{RpcError, decode_response, read_frame, write_frame};
use serde_json::{Value, json};
use std::io::{self, Read, Write};
use std::ptr::{null, null_mut};
use std::sync::{Arc, Mutex, atomic::{AtomicBool, AtomicU64, Ordering}};
use std::time::{Duration, Instant};
use windows_sys::Win32::{Foundation::*, Storage::FileSystem::*, System::{IO::*, Pipes::*, Threading::*}};

struct Handle(HANDLE);
impl Drop for Handle { fn drop(&mut self) { unsafe { CloseHandle(self.0); } } }

/// The actual pipe server, held by process handle so PID reuse cannot change
/// the lifetime this UI observes. Kernel process handles are thread-safe.
pub struct ServerProcess { handle: HANDLE, pid: u32 }
unsafe impl Send for ServerProcess {}
unsafe impl Sync for ServerProcess {}
impl Drop for ServerProcess { fn drop(&mut self) { unsafe { CloseHandle(self.handle); } } }
impl ServerProcess {
    pub fn id(&self)->u32 { self.pid }
    pub fn exit_code(&self)->Result<Option<u32>,RpcError>{
        match unsafe{WaitForSingleObject(self.handle,0)}{
            WAIT_TIMEOUT=>Ok(None),WAIT_OBJECT_0=>{
                let mut code=0;if unsafe{GetExitCodeProcess(self.handle,&mut code)}==0{return Err(io::Error::last_os_error().into());}Ok(Some(code))
            },_=>Err(io::Error::last_os_error().into()),
        }
    }
    /// An exit code means the server exited; None means the wait was cancelled.
    pub fn wait_for_exit(&self, stop: HANDLE) -> Result<Option<u32>,RpcError> {
        let handles=[self.handle,stop];
        match unsafe { WaitForMultipleObjects(2,handles.as_ptr(),0,INFINITE) } {
            WAIT_OBJECT_0=>self.exit_code(), value if value==WAIT_OBJECT_0+1=>Ok(None),
            _=>Err(io::Error::last_os_error().into()),
        }
    }
}

pub struct Client { pipe: Vec<u16>, cancel: Arc<AtomicBool>, sequence: AtomicU64, server: Mutex<Option<Arc<ServerProcess>>> }
impl Client {
    pub fn connected_server(&self)->Option<Arc<ServerProcess>> { self.server.lock().ok()?.clone() }
    /// A background refresh must not resurrect a session after normal Quit.
    /// Startup retries and recovery from a failed Host keep their prior path.
    pub fn may_start_server(&self)->Result<bool,RpcError>{
        match self.connected_server(){None=>Ok(true),Some(server)=>Ok(server.exit_code()?.is_some_and(|code|code!=0))}
    }
    fn bind_server(&self, pipe:HANDLE)->Result<(),RpcError> {
        let mut pid=0;
        if unsafe{GetNamedPipeServerProcessId(pipe,&mut pid)}==0{return Err(io::Error::last_os_error().into());}
        let mut current=self.server.lock().map_err(|_|RpcError::new("connection","后台进程状态不可用"))?;
        if let Some(server)=current.as_ref(){
            let exit=server.exit_code()?;
            if exit.is_none() && server.pid==pid { return Ok(()); }
            if !exit.is_some_and(|code|code!=0) {
                return Err(RpcError::new("connection","后台已退出，请重新打开 WinOne"));
            }
        }
        {
            let handle=unsafe{OpenProcess(PROCESS_SYNCHRONIZE|PROCESS_QUERY_LIMITED_INFORMATION,0,pid)};
            if handle.is_null(){return Err(io::Error::last_os_error().into());}
            *current=Some(Arc::new(ServerProcess{handle,pid}));
        }
        Ok(())
    }
    /// Startup coordination must not launch a Host after the UI closes.
    pub fn cancellation_requested(&self) -> bool { self.cancel.load(Ordering::Acquire) }
    /// Submit an operation once and only poll its returned ticket afterwards.
    /// A timeout reports uncertainty; it never resubmits the launch/action.
    pub fn call_job(&self, namespace: &str, method: &str, params: Value) -> Result<Value, RpcError> {
        self.wait_job(namespace,method,params,Some(Duration::from_secs(300)))
    }
    /// A user may keep an editor open indefinitely. Individual pipe requests
    /// remain bounded, but the editing session has no artificial time limit.
    pub fn call_interactive_job(&self, namespace: &str, method: &str, params: Value) -> Result<Value, RpcError> {
        self.wait_job(namespace,method,params,None)
    }
    fn wait_job(&self, namespace: &str, method: &str, params: Value, limit: Option<Duration>) -> Result<Value, RpcError> {
        use crate::protocol::{module_job, ModuleJob};
        let deadline = limit.map(|limit|Instant::now()+limit);
        let mut state = module_job(None, &self.call(&format!("{namespace}.{method}"), params)?)?;
        loop {
            match state {
                ModuleJob::Completed(value) => return Ok(value),
                ModuleJob::Cancelled => return Err(RpcError::new("cancelled","操作已取消")),
                ModuleJob::Running(ticket) => {
                    for _ in 0..5 {
                        if let Some(deadline)=deadline { self.check(deadline)?; }
                        else if self.cancellation_requested() { return Err(RpcError::new("cancelled","操作已取消")); }
                        std::thread::sleep(Duration::from_millis(50));
                    }
                    state = module_job(Some(&ticket), &self.call(&format!("{namespace}.jobs.get"), json!({"id":ticket}))?)?;
                }
            }
        }
    }
    pub fn new(pipe: &str, cancel: Arc<AtomicBool>) -> Result<Self, RpcError> {
        if !pipe.starts_with(r"\\.\pipe\WinOne-") || pipe.contains('\0') || pipe.len() > 256 {
            return Err(RpcError::new("invalid", "仅支持本机 WinOne 管道"));
        }
        Ok(Self { pipe: pipe.encode_utf16().chain(Some(0)).collect(), cancel, sequence: AtomicU64::new(1), server:Mutex::new(None) })
    }
    pub fn call(&self, method: &str, params: Value) -> Result<Value, RpcError> {
        let id = format!("rust-{}-{}", std::process::id(), self.sequence.fetch_add(1, Ordering::Relaxed));
        let timeout = if method=="chat.export" { 150 } else if method.starts_with("chat.") || method=="capture.ai.usage.export.save" { 30 } else if method.starts_with("capture.") || method.starts_with("always-on-top.") { 15 } else { 5 };
        self.request(&id, method, params, Instant::now() + Duration::from_secs(timeout))
    }
    pub fn call_completing(&self, method: &str, params: Value) -> Result<Value, RpcError> {
        let id = format!("rust-{}-{}", std::process::id(), self.sequence.fetch_add(1, Ordering::Relaxed));
        // Batch model probes retain Magpie's 90-second request budget. Other
        // desktop actions keep their existing short response window.
        let budget=if method=="capture.ai.models.test" {100}else{30};
        let deadline = Instant::now() + Duration::from_secs(budget);
        // Export has a bounded 20-second producer before its atomic write.
        // Keep ordinary requests responsive without timing out a valid export.
        // Account reads serialize against token refresh/login in the Host and
        // have a 25-second total budget, including their wait for the profile.
        let first_wait=if method=="capture.ai.models.test" {100}else if method=="capture.ai.usage.export.save" || matches!(method,"capture.ai.magpie.catalog"|"capture.ai.accounts.catalog"|"capture.ai.accounts.list"|"capture.ai.accounts.models"|"capture.ai.accounts.quota"|"capture.ai.accounts.checkin"){30}else{15};
        let first = self.request(&id, method, params, deadline.min(Instant::now() + Duration::from_secs(first_wait)))?;
        let Some(ticket) = first.get("configurationJob").and_then(Value::as_str) else { return Ok(first); };
        loop {
            self.check(deadline)?;
            let job = self.request(&format!("{id}-configuration"), "configuration.jobs.get", json!({"id":ticket}), deadline)?;
            if job["id"].as_str() != Some(ticket) { return Err(RpcError::new("protocol", "配置任务标识不匹配")); }
            match job["state"].as_str() {
                Some("running") => {
                    // Poll only the admitted job, never resubmit the original save.
                    for _ in 0..3 { self.check(deadline)?; std::thread::sleep(Duration::from_millis(50)); }
                }
                Some(state @ ("completed" | "failed")) => {
                    let response = job.get("response").ok_or_else(|| RpcError::new("protocol", "配置任务缺少结果"))?;
                    if (state == "failed") != response.get("error").is_some() {
                        return Err(RpcError::new("protocol", "配置任务状态与结果不一致"));
                    }
                    return decode_response(&id, &serde_json::to_vec(response)?);
                }
                _ => return Err(RpcError::new("protocol", "无法确认配置任务状态")),
            }
        }
    }
    fn check(&self, deadline: Instant) -> Result<(), RpcError> {
        if self.cancel.load(Ordering::Acquire) { return Err(RpcError::new("cancelled", "操作已取消")); }
        if Instant::now() >= deadline { return Err(RpcError::new("timeout", "后台响应超时，请检查后重试")); }
        Ok(())
    }
    fn request(&self, id: &str, method: &str, params: Value, deadline: Instant) -> Result<Value, RpcError> {
        self.check(deadline)?;
        // Identification-level SQOS prevents a local pipe server from impersonating this UI.
        let file = loop {
            let raw = unsafe { CreateFileW(self.pipe.as_ptr(), GENERIC_READ | GENERIC_WRITE, 0, null(), OPEN_EXISTING,
                FILE_FLAG_OVERLAPPED | SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION, null_mut()) };
            if raw != INVALID_HANDLE_VALUE { break Handle(raw); }
            let error = io::Error::last_os_error();
            if error.raw_os_error() != Some(ERROR_PIPE_BUSY as i32) { return Err(error.into()); }
            self.check(deadline)?;
            unsafe { WaitNamedPipeW(self.pipe.as_ptr(), 50); }
        };
        self.bind_server(file.0)?;
        let mut stream = Pipe { file, deadline, cancel: &self.cancel };
        write_frame(&mut stream, &serde_json::to_vec(&json!({"version":1,"id":id,"method":method,"params":params}))?)?;
        let bytes = read_frame(&mut stream)?;
        decode_response(id, &bytes)
    }
}

struct Pipe<'a> { file: Handle, deadline: Instant, cancel: &'a AtomicBool }
impl Pipe<'_> {
    fn transfer(&self, bytes: *mut u8, length: usize, write: bool) -> io::Result<usize> {
        if self.cancel.load(Ordering::Acquire) { return Err(io::Error::new(io::ErrorKind::ConnectionAborted, "操作已取消")); }
        let event = Handle(unsafe { CreateEventW(null(), 1, 0, null()) });
        if event.0.is_null() { return Err(io::Error::last_os_error()); }
        let mut overlapped: OVERLAPPED = unsafe { std::mem::zeroed() };
        overlapped.hEvent = event.0;
        let mut transferred = 0;
        let ok = unsafe {
            if write { WriteFile(self.file.0, bytes, length as u32, &mut transferred, &mut overlapped) }
            else { ReadFile(self.file.0, bytes, length as u32, &mut transferred, &mut overlapped) }
        };
        if ok != 0 { return Ok(transferred as usize); }
        let error = io::Error::last_os_error();
        if error.raw_os_error() != Some(ERROR_IO_PENDING as i32) { return Err(error); }
        loop {
            if self.cancel.load(Ordering::Acquire) || Instant::now() >= self.deadline {
                // Drain cancellation before freeing the OVERLAPPED and caller buffer.
                unsafe { CancelIoEx(self.file.0, &overlapped); GetOverlappedResult(self.file.0, &overlapped, &mut transferred, 1); }
                return Err(io::Error::new(if self.cancel.load(Ordering::Acquire) { io::ErrorKind::ConnectionAborted } else { io::ErrorKind::TimedOut }, "后台请求已取消或超时"));
            }
            match unsafe { WaitForSingleObject(event.0, 50) } {
                WAIT_OBJECT_0 => {
                    if unsafe { GetOverlappedResult(self.file.0, &overlapped, &mut transferred, 0) } == 0 { return Err(io::Error::last_os_error()); }
                    return Ok(transferred as usize);
                }
                WAIT_TIMEOUT => continue,
                _ => {
                    let error = io::Error::last_os_error();
                    unsafe { CancelIoEx(self.file.0, &overlapped); GetOverlappedResult(self.file.0, &overlapped, &mut transferred, 1); }
                    return Err(error);
                }
            }
        }
    }
}
impl Read for Pipe<'_> { fn read(&mut self, bytes: &mut [u8]) -> io::Result<usize> { self.transfer(bytes.as_mut_ptr(), bytes.len(), false) } }
impl Write for Pipe<'_> {
    fn write(&mut self, bytes: &[u8]) -> io::Result<usize> { self.transfer(bytes.as_ptr() as *mut u8, bytes.len(), true) }
    fn flush(&mut self) -> io::Result<()> { Ok(()) }
}
