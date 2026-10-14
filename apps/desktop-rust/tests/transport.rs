#![cfg(windows)]
use std::sync::{Arc, atomic::AtomicBool};
use std::time::{Duration, Instant};
use serde_json::Value;
use winone_desktop::transport::Client;
#[path = "support/pipe.rs"]
mod pipe;

#[test]
fn rejects_remote_pipe_names_and_cancelled_requests() {
    let cancel = Arc::new(AtomicBool::new(false));
    assert!(Client::new(r"\\server\pipe\WinOne", cancel.clone()).is_err());
    let client = Client::new(r"\\.\pipe\WinOne-rust-test-no-server", Arc::new(AtomicBool::new(true))).unwrap();
    let started = Instant::now();
    assert_eq!(client.call("snapshot", Value::Null).unwrap_err().code, "cancelled");
    assert!(started.elapsed() < Duration::from_millis(250));
}

#[test]
fn absent_host_reports_connection_error_without_retrying_a_mutation() {
    let client = Client::new(&format!(r"\\.\pipe\WinOne-rust-absent-{}", std::process::id()), Arc::new(AtomicBool::new(false))).unwrap();
    let started = Instant::now();
    let error = client.call("settings.save", serde_json::json!({"revision":0})).unwrap_err();
    assert_eq!(error.code, "transport");
    assert!(started.elapsed() < Duration::from_secs(1));
}

#[test]
fn launch_polls_only_its_admitted_ticket_until_the_real_pipe_result_arrives() {
    use serde_json::json;
    let (client, server) = pipe::serve(vec![
        Some(json!({"id":"launch-17","state":"running"})),
        Some(json!({"id":"launch-17","state":"running"})),
        Some(json!({"id":"launch-17","state":"completed","result":{"Managed":true,"Message":"started"}})),
    ]);
    let result = client.call_job("wintray", "launch", json!({"id":"saved-app"}));
    let requests = server.join().unwrap();
    assert_eq!(result.unwrap(), json!({"Managed":true,"Message":"started"}));
    assert_eq!(requests.len(), 3);
    assert_eq!(requests[0]["method"], "wintray.launch");
    assert_eq!(requests[0]["params"], json!({"id":"saved-app"}));
    for request in &requests[1..] {
        assert_eq!(request["method"], "wintray.jobs.get");
        assert_eq!(request["params"], json!({"id":"launch-17"}));
    }
}

#[test]
fn failed_changed_or_disconnected_jobs_never_report_success_or_resubmit_launch() {
    use serde_json::json;
    for (response, code) in [
        (Some(json!({"id":"launch-17","state":"failed","error":"fixture refused"})), "operation_failed"),
        (Some(json!({"id":"unrelated","state":"completed"})), "protocol"),
        (None, "transport"),
    ] {
        let (client, server) = pipe::serve(vec![Some(json!({"id":"launch-17","state":"running"})), response]);
        let result = client.call_job("wintray", "launch", json!({"id":"saved-app"}));
        let requests = server.join().unwrap();
        let error = result.unwrap_err();
        assert_eq!(error.code, code);
        if code == "operation_failed" { assert_eq!(error.message, "fixture refused"); }
        assert_eq!(requests.len(), 2);
        assert_eq!(requests[0]["method"], "wintray.launch");
        assert_eq!(requests[1]["method"], "wintray.jobs.get");
        assert_eq!(requests[1]["params"], json!({"id":"launch-17"}));
    }
}

#[test]
fn cancelling_an_interactive_capture_keeps_its_ticket_and_reports_cancellation() {
    use serde_json::json;
    let (client,server)=pipe::serve(vec![Some(json!({"id":"capture-1","state":"running"})),Some(json!({"id":"capture-1","state":"cancelled"}))]);
    assert_eq!(client.call_interactive_job("capture","draft.resume",json!({"expectedId":"a".repeat(32)})).unwrap_err().code,"cancelled");
    let requests=server.join().unwrap();
    assert_eq!(requests[0]["method"],"capture.draft.resume");
    assert_eq!(requests[1]["method"],"capture.jobs.get");
    assert_eq!(requests[1]["params"]["id"],"capture-1");
}
