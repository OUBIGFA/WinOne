//! One cancellable request worker; the native UI never waits for pipe I/O.
use std::sync::{Arc, atomic::{AtomicBool, Ordering}, mpsc::{self, Receiver, SyncSender}};
use std::thread::JoinHandle;
use serde_json::Value;
use winone_desktop::{protocol::RpcError, transport::Client};
use windows_sys::Win32::{Foundation::HWND, UI::WindowsAndMessaging::{PostMessageW, WM_APP}};

pub const READY: u32 = WM_APP + 1;
#[derive(Clone)]
pub enum Task { Bootstrap, Load { page: String, generation: u64 }, Save { page: String, value: Value }, Launch { id: String },
    Capture {mode:String,language:String},
    CaptureSnapshot {generation:u64},
    CaptureAction {method:String,params:Value},
    CaptureFileName {generation:u64,template:String},
    Permission {action:crate::permissions::Action},
    UpdateInfo {generation:u64}, UpdateCheck {id:String}, UpdateStatus {id:String}, UpdateCancel {id:String}, UpdateRelease {id:String},
    Logs { page: String, generation: u64 }, LogFolder { page: String, generation: u64 },
    // Icon extraction reads file resources locally; it never reaches the Host
    // pipe, but shares the single worker so the UI thread never blocks on I/O.
    ProgramIcon { path: String },
    // The shortcut section owns its own transactions: recording pauses and
    // restores Host registrations, and its draft saves through the global
    // settings object rather than the module settings envelope.
    ShortcutRecord, ShortcutStop { token: String }, ShortcutStatus, ShortcutSnapshot, ShortcutSave { value: Value },
    // Module management reads through the control namespace; setEnabled and
    // recoverConfiguration commit configuration, so they complete their job.
    ModulesSnapshot { generation: u64 },
    ModulesSetEnabled { id: String, enabled: bool, revision: u64 },
    ModulesRecover,
    // Profile export and import. The preview is assembled from the Host's
    // chunked preview store and verified against the descriptor digest.
    ProfileExport { path: String, name: String },
    ProfilePreview { path: String, bindings: Value },
    ProfileApply { params: Value } }
pub struct Reply { pub task: Task, pub result: Result<Value, RpcError> }
pub struct Worker { pub requests: Option<SyncSender<Task>>, pub replies: Receiver<Reply>, cancel: Arc<AtomicBool>, join: Option<JoinHandle<()>> }
impl Worker {
    #[cfg(test)]
    pub fn test_channels() -> (Self, Receiver<Task>, mpsc::Sender<Reply>) {
        let (requests, incoming) = mpsc::sync_channel(4);
        let (outgoing, replies) = mpsc::channel();
        (Self { requests: Some(requests), replies, cancel: Arc::new(AtomicBool::new(false)), join: None }, incoming, outgoing)
    }
    pub fn start(pipe: &str, data_directory: &str, window: HWND, plan: crate::startup::Plan) -> Result<Self, RpcError> {
        let cancel = Arc::new(AtomicBool::new(false));
        let client = Arc::new(Client::new(pipe, cancel.clone())?);
        let pipe = pipe.to_string();
        let (requests, incoming) = mpsc::sync_channel(4);
        let (outgoing, replies) = mpsc::channel();
        let target = window as usize;
        let data_directory = data_directory.to_string();
        let icon_cache = std::path::Path::new(&data_directory).join("cache/desktop-rust/program-icons").to_string_lossy().into_owned();
        let stopping = cancel.clone();
        let join = std::thread::spawn(move || {
            while let Ok(task) = incoming.recv() {
                if stopping.load(Ordering::Acquire) { break; }
                let result = match &task {
                    Task::UpdateInfo{..}=>client.call("updates.info",Value::Null),
                    Task::UpdateCheck{id}=>client.call("updates.check",serde_json::json!({"id":id})),
                    Task::UpdateStatus{id}=>client.call("updates.status",serde_json::json!({"id":id})),
                    Task::UpdateCancel{id}=>client.call("updates.cancel",serde_json::json!({"id":id})),
                    Task::UpdateRelease{id}=>client.call("updates.release",serde_json::json!({"id":id})),
                    Task::Permission {action}=>crate::permissions::run(client.clone(),target as HWND,&pipe,&data_directory,*action),
                    Task::Capture {mode,language}=>client.call_interactive_job("capture","capture.unified",serde_json::json!({"mode":mode,"language":language})),
                    Task::CaptureSnapshot {..}=>match crate::capture_history::read(&data_directory) {
                        Err(error) if error.code=="legacy_capture"=>client.call("capture.snapshot",Value::Null),
                        result=>result,
                    },
                    Task::CaptureAction {method,params}=>client.call_interactive_job("capture",method,params.clone()),
                    Task::CaptureFileName {template,..}=>client.call("capture.filename.preview",serde_json::json!({"template":template})),
                    Task::ProgramIcon { path } => Ok(serde_json::json!({"path":path, "icon": crate::icon::load(&icon_cache, path)})),
                    Task::Logs { page, .. } => crate::diagnostics::read(&data_directory, page),
                    Task::LogFolder { page, .. } => crate::diagnostics::folder(&data_directory, page),
                    Task::Launch { id } => client.call_job("wintray", "launch", serde_json::json!({"id":id})),
                    Task::ShortcutRecord => client.call("shortcuts.record.start",
                        serde_json::json!({"processId": std::process::id()})),
                    Task::ShortcutStop { token } => client.call("shortcuts.record.stop",
                        serde_json::json!({"token":token})),
                    Task::ShortcutStatus => client.call("shortcuts.snapshot", Value::Null),
                    Task::ShortcutSnapshot => client.call("snapshot", Value::Null),
                    Task::ShortcutSave { value } => client.call_completing("settings.save", value.clone()),
                    Task::ModulesSnapshot { .. } => client.call("modules.snapshot", Value::Null),
                    Task::ModulesSetEnabled { id, enabled, revision } => client.call_completing("modules.setEnabled",
                        serde_json::json!({"id":id, "enabled":enabled, "revision":revision})),
                    Task::ModulesRecover => client.call_completing("modules.recoverConfiguration", Value::Null),
                    Task::ProfileExport { path, name } => client.call("profiles.export", serde_json::json!({"path":path, "name":name})),
                    Task::ProfilePreview { path, bindings } => receive_profile_preview(&client, path, bindings),
                    Task::ProfileApply { params } => client.call_completing("profiles.import.apply", params.clone()),
                    Task::Bootstrap => crate::startup::ensure_host(&client, &pipe, &data_directory, &plan).and_then(|mut snapshot| {
                        if !plan.warning.is_empty() {
                            let current=snapshot["runtimeWarning"].as_str().unwrap_or("");
                            snapshot["runtimeWarning"]=serde_json::json!(format!("{current}\n{}",plan.warning).trim());
                        }
                        client.call("modules.catalog", Value::Null).map(|catalog| serde_json::json!({"snapshot":snapshot,"catalog":catalog}))
                    }),
                    Task::Load { page, .. } => client.call(
                        &if page == "settings" { "snapshot".into() } else { format!("{page}.settings.get") }, Value::Null),
                    Task::Save { page, value } => client.call_completing(
                        &if page == "settings" { "settings.save".into() } else { format!("{page}.settings.save") }, value.clone()),
                };
                if stopping.load(Ordering::Acquire) || outgoing.send(Reply { task, result }).is_err() { break; }
                // The channel owns the response; no raw allocation is passed through LPARAM.
                unsafe { PostMessageW(target as HWND, READY, 0, 0); }
            }
        });
        Ok(Self { requests: Some(requests), replies, cancel, join: Some(join) })
    }
    pub fn submit(&self, task: Task) -> Result<(), RpcError> {
        self.requests.as_ref().ok_or_else(|| RpcError::new("closed", "界面已关闭"))?.try_send(task)
            .map_err(|_| RpcError::new("busy", "正在读取，请稍后重试"))
    }
}

use crate::profile_transfer::receive as receive_profile_preview;

#[cfg(test)]
#[path = "../tests/support/pipe.rs"]
mod test_pipe;

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    #[ignore = "requires the dedicated Host handler fixture from build/test-desktop-rust.ps1"]
    fn real_host_module_and_profile_roundtrip() {
        assert_eq!(crate::startup::default_pipe().unwrap(),std::env::var("WINONE_RUST_EXPECTED_PIPE").unwrap(),"Rust and Host must select the same Windows user/session pipe");
        let pipe = std::env::var("WINONE_RUST_CONFIGURATION_PIPE").unwrap();
        assert!(pipe.starts_with(r"\\.\pipe\WinOne-rust-configuration-"));
        let directory = std::env::var("WINONE_RUST_CONFIGURATION_DIRECTORY").unwrap();
        let client = Client::new(&pipe, Arc::new(AtomicBool::new(false))).unwrap();
        let logs = std::path::Path::new(&directory).join("logs");
        std::fs::create_dir_all(&logs).unwrap();
        let mut text = "日志\u{fffd}".as_bytes().to_vec();
        text.extend_from_slice(&[0xff,0xff,b'\n']);
        std::fs::write(logs.join("host.log"), text).unwrap();
        for module in ["host","capture","always-on-top","wintray"] {
            let remote = client.call("diagnostics.read", json!({"module":module})).unwrap();
            assert_eq!(crate::diagnostics::read(&directory,module).unwrap(), remote, "local diagnostics must retain the Host's text and empty-state contract");
        }
        let snapshot = client.call("modules.snapshot", Value::Null).unwrap();
        assert_eq!(snapshot["modules"].as_array().unwrap().len(), 3);
        let toggle = json!({"id":"capture","enabled":false,"revision":snapshot["revision"]});
        assert_eq!(client.call_completing("modules.setEnabled", toggle.clone()).unwrap()["state"], "completed");
        assert!(client.call_completing("modules.setEnabled", toggle).is_err(), "stale module revision must be rejected");
        let mut settings = client.call("snapshot", Value::Null).unwrap()["settings"].clone();
        settings["theme"] = json!("dark");
        client.call_completing("settings.save", settings).unwrap();
        let path = std::path::Path::new(&directory).join("portable.winone").to_string_lossy().into_owned();
        let exported = client.call("profiles.export", json!({"path":path,"name":"配置迁移"})).unwrap();
        assert_eq!(exported["path"], path);
        assert!(std::fs::metadata(&path).unwrap().len() > 0);
        let mut settings = client.call("snapshot", Value::Null).unwrap()["settings"].clone();
        settings["theme"] = json!("light");
        client.call_completing("settings.save", settings).unwrap();
        let bindings = json!({"capture.output":directory});
        let preview = receive_profile_preview(&client, &path, &bindings).unwrap();
        assert_eq!(preview["name"], "配置迁移");
        assert_eq!(preview["preferences"]["theme"], "dark");
        let request = json!({"path":path,"bindings":bindings,"sourceHash":preview["sourceHash"],
            "configurationRevision":preview["configurationRevision"],"moduleRevision":preview["moduleRevision"],"global":true,"modules":["capture"]});
        let result = client.call_completing("profiles.import.apply", request.clone()).unwrap();
        assert!(result["revision"].is_u64());
        assert!(client.call_completing("profiles.import.apply", request).is_err(), "reviewed import cannot be replayed");
        assert_eq!(client.call("snapshot", Value::Null).unwrap()["settings"]["theme"], "dark");
    }

    #[test]
    fn profile_chunks_reassemble_unicode_and_release_the_snapshot() {
        let bytes = "{\"name\":\"配置\"}".as_bytes();
        let token = "a".repeat(32);
        let (client, server) = test_pipe::serve(vec![
            Some(json!({"token":token,"size":bytes.len(),"sha256":crate::icon::sha256_hex(bytes)})),
            Some(json!({"token":token,"offset":0,"data":crate::icon::base64_encode(&bytes[..11])})),
            Some(json!({"token":token,"offset":11,"data":crate::icon::base64_encode(&bytes[11..])})),
            Some(json!({"released":true})),
        ]);
        assert_eq!(receive_profile_preview(&client, "sample.winone", &json!({})).unwrap(), json!({"name":"配置"}));
        let requests = server.join().unwrap();
        assert_eq!(requests[2]["params"]["offset"], 11);
        assert_eq!(requests[3]["method"], "profiles.import.preview.release");
    }

    #[test]
    fn invalid_profile_chunks_fail_without_losing_cleanup() {
        let bytes = b"{}";
        let token = "b".repeat(32);
        for (offset, digest) in [(1, crate::icon::sha256_hex(bytes)), (0, "0".repeat(64))] {
            let (client, server) = test_pipe::serve(vec![
                Some(json!({"token":token,"size":bytes.len(),"sha256":digest})),
                Some(json!({"token":token,"offset":offset,"data":crate::icon::base64_encode(bytes)})),
                Some(json!({"released":true})),
            ]);
            assert_eq!(receive_profile_preview(&client, "sample.winone", &json!({})).unwrap_err().code, "protocol");
            let requests = server.join().unwrap();
            assert_eq!(requests.last().unwrap()["method"], "profiles.import.preview.release");
        }
    }
}
impl Drop for Worker {
    fn drop(&mut self) {
        self.cancel.store(true, Ordering::Release);
        self.requests.take();
        if let Some(join) = self.join.take() { let _ = join.join(); }
    }
}
