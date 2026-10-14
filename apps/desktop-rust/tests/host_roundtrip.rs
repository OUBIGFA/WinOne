#![cfg(windows)]
use serde_json::{Value, json};
use std::{sync::{Arc, atomic::AtomicBool}, time::{Duration, Instant}};
use winone_desktop::transport::Client;

// Opt-in: the runner starts a dedicated headless Host with an empty private config.
#[test]
#[ignore = "requires WINONE_RUST_TEST_PIPE from build/test-desktop-rust.ps1"]
fn real_host_reads_catalog_saves_once_and_rejects_stale_revision() {
    let pipe = std::env::var("WINONE_RUST_TEST_PIPE").unwrap();
    assert!(pipe.starts_with(r"\\.\pipe\WinOne-rust-test-"));
    let client = Client::new(&pipe, Arc::new(AtomicBool::new(false))).unwrap();
    let deadline = Instant::now() + Duration::from_secs(5);
    let snapshot = loop {
        match client.call("snapshot", Value::Null) {
            Ok(snapshot) => break snapshot,
            Err(error) if Instant::now() < deadline => { let _ = error; std::thread::sleep(Duration::from_millis(50)); },
            Err(error) => panic!("{error}"),
        }
    };
    let catalog = client.call("modules.catalog", Value::Null).unwrap();
    let updates=client.call("updates.info",Value::Null).unwrap();
    assert_eq!(updates["configured"],false);
    assert!(updates["version"].as_str().is_some_and(|version|!version.is_empty()));
    assert!(client.call("updates.check",json!({"id":"invalid-source","repository":"other/publisher"})).is_err());
    let ids: Vec<_> = catalog.as_array().unwrap().iter().map(|m| m["id"].as_str().unwrap()).collect();
    assert_eq!(ids, ["capture", "always-on-top", "wintray"]);
    let logs = client.call("diagnostics.read", json!({"module":"capture"})).unwrap();
    assert_eq!(logs["folderAvailable"], true);
    assert_eq!(logs["entries"][0]["name"], "process");
    assert_eq!(logs["entries"][0]["text"], "Rust/Host 日志 fixture");
    let folder = client.call("diagnostics.folder", json!({"module":"capture"})).unwrap();
    let path = std::path::Path::new(folder["path"].as_str().unwrap());
    assert!(path.is_absolute() && path.ends_with("modules/capture"));
    assert!(client.call("diagnostics.read", json!({"module":"../private"})).is_err());
    let original = snapshot["settings"].clone();
    let mut next = original.clone();
    next["theme"] = json!("dark");
    let saved = client.call_completing("settings.save", next.clone()).unwrap();
    assert_eq!(saved["theme"], "dark");
    assert_eq!(saved["revision"].as_u64(), original["revision"].as_u64().map(|n| n + 1));
    let error = client.call_completing("settings.save", next).unwrap_err();
    assert_eq!(error.code, "conflict");
    let refreshed = client.call("snapshot", Value::Null).unwrap();
    assert_eq!(refreshed["settings"], saved);

    // All three preference APIs are available without activating their engines.
    // Capture/pinning use numeric revisions; program management has an envelope
    // with a string revision and a completed operation result.
    for module in ["capture", "always-on-top"] {
        let original = client.call(&format!("{module}.settings.get"), Value::Null).unwrap();
        let mut request=original.clone();
        if module=="always-on-top"{request["useAccent"]=json!(false);request["frameColor"]=json!("#123456");}
        else{request["fileNameTemplate"]=json!("截图-{date}-{sequence}");}
        let saved = client.call_completing(&format!("{module}.settings.save"), request).unwrap();
        assert_eq!(saved["revision"].as_u64(), original["revision"].as_u64().map(|n| n + 1));
        if module=="always-on-top"{assert_eq!(saved["frameColor"],"#123456");assert_eq!(saved["useAccent"],false);}
        else{assert_eq!(saved["fileNameTemplate"],"截图-{date}-{sequence}");}
        assert!(client.call_completing(&format!("{module}.settings.save"), original).is_err());
        assert_eq!(client.call(&format!("{module}.settings.get"), Value::Null).unwrap(), saved);
    }
    let preview=client.call("capture.filename.preview",json!({"template":"截图-{date}-{sequence}"})).unwrap();
    let name=preview["name"].as_str().unwrap();
    assert!(name.starts_with("截图-") && name.ends_with("-01.png"));
    assert_eq!(client.call("capture.filename.preview",json!({"template":"../outside"})).unwrap_err().code,"configuration");
    let original = client.call("wintray.settings.get", Value::Null).unwrap();
    let request = json!({"revision": original["revision"], "settings": original["settings"]});
    let saved = client.call_completing("wintray.settings.save", request.clone()).unwrap();
    assert_eq!(saved["state"], "completed");
    assert_ne!(saved["result"]["revision"], original["revision"]);
    assert!(client.call_completing("wintray.settings.save", request).is_err());
    let reread = client.call("wintray.settings.get", Value::Null).unwrap();
    assert_eq!(reread["revision"], saved["result"]["revision"]);
    assert_eq!(reread["settings"], original["settings"]);
    let after = client.call("snapshot", Value::Null).unwrap();
    assert_eq!(after["modules"], snapshot["modules"], "preference reads and empty saves must not activate a module");
}
