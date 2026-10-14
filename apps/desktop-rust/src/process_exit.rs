//! Final process boundary, after the application's saves and resource cleanup.
use std::io::Write;
use windows_sys::Win32::System::Threading::{GetCurrentProcess, TerminateProcess};

pub fn after_cleanup(code: u32) -> ! {
    // CRT ExitProcess calls every injected DLL's detach callback. A blocked
    // input-method dictionary mutex kept otherwise closed desktops alive in
    // the field. Finish our own RAII cleanup first, then let Windows reclaim
    // the process without executing unrelated DLL cleanup under loader lock.
    // https://learn.microsoft.com/windows/win32/api/processthreadsapi/nf-processthreadsapi-exitprocess
    let _ = std::io::stdout().flush();
    let _ = std::io::stderr().flush();
    if unsafe { TerminateProcess(GetCurrentProcess(), code) } == 0 {
        eprintln!("final process exit: {}", std::io::Error::last_os_error());
    }
    std::process::exit(code as i32)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn completed_child() {
        if std::env::var("WINONE_RUST_EXIT_CHILD").as_deref() != Ok("1") { return; }
        if let Some(path) = std::env::var_os("WINONE_EXIT_FIXTURE_DLL") {
            use std::os::windows::ffi::OsStrExt;
            let wide: Vec<u16> = path.encode_wide().chain(Some(0)).collect();
            assert!(!unsafe { windows_sys::Win32::System::LibraryLoader::LoadLibraryW(wide.as_ptr()) }.is_null());
        }
        struct Saved(std::path::PathBuf);
        impl Drop for Saved { fn drop(&mut self) { std::fs::write(&self.0, b"saved").unwrap(); } }
        { let _saved = Saved(std::env::var_os("WINONE_RUST_EXIT_MARKER").unwrap().into()); }
        after_cleanup(17)
    }
    #[test]
    fn completed_exit_preserves_cleanup_and_skips_third_party_detach() {
        let marker = std::env::temp_dir().join(format!("winone-exit-{}-{}.txt", std::process::id(), std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_nanos()));
        let started = std::time::Instant::now();
        let mut child = std::process::Command::new(std::env::current_exe().unwrap())
            .args(["--exact", "process_exit::tests::completed_child", "--nocapture"])
            .env("WINONE_RUST_EXIT_CHILD", "1").env("WINONE_RUST_EXIT_MARKER", &marker)
            .stdout(std::process::Stdio::piped()).spawn().unwrap();
        let status = loop {
            if let Some(status) = child.try_wait().unwrap() { break status; }
            if started.elapsed() > std::time::Duration::from_secs(5) { let _ = child.kill(); let _ = child.wait(); panic!("completed process did not exit"); }
            std::thread::sleep(std::time::Duration::from_millis(10));
        };
        assert_eq!(status.code(), Some(17));
        use std::io::Read;
        let mut output = String::new();child.stdout.take().unwrap().read_to_string(&mut output).unwrap();
        assert!(!output.contains("native-detach"), "completed process invoked third-party DLL detach");
        assert_eq!(std::fs::read(&marker).unwrap(), b"saved");
        std::fs::remove_file(marker).unwrap();
    }
}
