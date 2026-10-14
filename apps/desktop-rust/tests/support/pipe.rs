//! A bounded, private pipe peer for exercising the real Rust wire transport.
use serde_json::{Value, json};
use std::{io::{self, Read, Write}, ptr::null, sync::{Arc, atomic::{AtomicBool, AtomicU64, Ordering}}, thread::JoinHandle};
use windows_sys::Win32::{Foundation::*, Storage::FileSystem::*, System::{IO::*, Pipes::*, Threading::*}};
use winone_desktop::{protocol::{read_frame, write_frame}, transport::Client};

struct Handle(HANDLE);
impl Drop for Handle { fn drop(&mut self) { unsafe { CloseHandle(self.0); } } }
struct Peer(Handle);
impl Peer {
    fn transfer(&self, operation: impl FnOnce(*mut OVERLAPPED, *mut u32) -> i32) -> io::Result<u32> {
        let event = Handle(unsafe { CreateEventW(null(), 1, 0, null()) });
        if event.0.is_null() { return Err(io::Error::last_os_error()); }
        let mut overlapped: OVERLAPPED = unsafe { std::mem::zeroed() };
        overlapped.hEvent = event.0;
        let mut count = 0;
        if operation(&mut overlapped, &mut count) != 0 { return Ok(count); }
        let error = io::Error::last_os_error();
        if error.raw_os_error() == Some(ERROR_PIPE_CONNECTED as i32) { return Ok(0); }
        if error.raw_os_error() != Some(ERROR_IO_PENDING as i32) { return Err(error); }
        if unsafe { WaitForSingleObject(event.0, 5000) } != WAIT_OBJECT_0 {
            // A broken client must fail the test, never leave a blocking server.
            unsafe { CancelIoEx(self.0.0, &overlapped); GetOverlappedResult(self.0.0, &overlapped, &mut count, 1); }
            return Err(io::Error::new(io::ErrorKind::TimedOut, "test pipe peer timed out"));
        }
        if unsafe { GetOverlappedResult(self.0.0, &overlapped, &mut count, 0) } == 0 { return Err(io::Error::last_os_error()); }
        Ok(count)
    }
}
impl Read for Peer {
    fn read(&mut self, buffer: &mut [u8]) -> io::Result<usize> {
        self.transfer(|overlapped, count| unsafe { ReadFile(self.0.0, buffer.as_mut_ptr(), buffer.len() as u32, count, overlapped) }).map(|n| n as usize)
    }
}
impl Write for Peer {
    fn write(&mut self, buffer: &[u8]) -> io::Result<usize> {
        self.transfer(|overlapped, count| unsafe { WriteFile(self.0.0, buffer.as_ptr(), buffer.len() as u32, count, overlapped) }).map(|n| n as usize)
    }
    fn flush(&mut self) -> io::Result<()> { Ok(()) }
}

/// Each entry is one Host result; None disconnects after receiving the request.
/// Returned requests let callers verify admission count, method, and ticket.
pub fn serve(responses: Vec<Option<Value>>) -> (Client, JoinHandle<Vec<Value>>) {
    static SEQUENCE: AtomicU64 = AtomicU64::new(1);
    let name = format!(r"\\.\pipe\WinOne-rust-peer-{}-{}", std::process::id(), SEQUENCE.fetch_add(1, Ordering::Relaxed));
    let server_name: Vec<u16> = name.encode_utf16().chain(Some(0)).collect();
    let (ready, initialized) = std::sync::mpsc::sync_channel(0);
    let thread = std::thread::spawn(move || {
        let handle = unsafe { CreateNamedPipeW(server_name.as_ptr(), PIPE_ACCESS_DUPLEX | FILE_FLAG_OVERLAPPED | FILE_FLAG_FIRST_PIPE_INSTANCE,
            PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS, 1, 8192, 8192, 0, null()) };
        assert_ne!(handle, INVALID_HANDLE_VALUE, "{}", io::Error::last_os_error());
        let mut peer = Peer(Handle(handle));
        ready.send(()).unwrap();
        let mut requests = Vec::new();
        for response in responses {
            peer.transfer(|overlapped, _| unsafe { ConnectNamedPipe(peer.0.0, overlapped) }).unwrap();
            let request: Value = serde_json::from_slice(&read_frame(&mut peer).unwrap()).unwrap();
            let id = request["id"].clone();
            requests.push(request);
            let Some(response) = response else { break; };
            write_frame(&mut peer, &serde_json::to_vec(&json!({"id":id,"result":response})).unwrap()).unwrap();
            // Wait for the client to consume the response and close. Disconnecting
            // immediately after WriteFile could discard buffered response bytes.
            let error = peer.read(&mut [0]).unwrap_err();
            assert!(matches!(error.raw_os_error(), Some(code) if code == ERROR_BROKEN_PIPE as i32 || code == ERROR_NO_DATA as i32));
            assert_ne!(unsafe { DisconnectNamedPipe(peer.0.0) }, 0);
        }
        requests
    });
    initialized.recv_timeout(std::time::Duration::from_secs(5)).unwrap();
    (Client::new(&name, Arc::new(AtomicBool::new(false))).unwrap(), thread)
}
