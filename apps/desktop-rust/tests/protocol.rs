use serde_json::json;
use std::io::Cursor;
use winone_desktop::protocol::{decode_response, read_frame, write_frame};

#[test]
fn frames_match_the_go_host_wire_format() {
    let mut bytes = Vec::new();
    write_frame(&mut bytes, b"{\"id\":\"1\"}").unwrap();
    assert_eq!(bytes, b"\x0a\x00\x00\x00{\"id\":\"1\"}");
    assert_eq!(read_frame(&mut Cursor::new(bytes)).unwrap(), b"{\"id\":\"1\"}");
}

#[test]
fn rejects_empty_oversized_and_truncated_frames_before_allocation() {
    for bytes in [vec![0, 0, 0, 0], vec![1, 0, 16, 0], vec![9, 0, 0, 0, 123]] {
        assert!(read_frame(&mut Cursor::new(bytes)).is_err());
    }
    assert!(write_frame(&mut Vec::new(), &[]).is_err());
}

#[test]
fn mismatched_ids_and_ambiguous_results_never_look_successful() {
    for response in [
        json!({"id":"other","result":{}}),
        json!({"id":"save","result":{},"error":{"code":"conflict","message":"设置已变更"}}),
        json!({"id":"save"}),
    ] {
        assert!(decode_response("save", &serde_json::to_vec(&response).unwrap()).is_err());
    }
    let error = decode_response("save", br#"{"id":"save","error":{"code":"conflict","message":"refresh"}}"#).unwrap_err();
    assert_eq!(error.code, "conflict");
    assert_eq!(error.message, "refresh");
}
#[test]
fn module_jobs_retain_their_ticket_and_require_a_terminal_state() {
    use winone_desktop::protocol::{module_job, ModuleJob};
    use serde_json::json;
    assert_eq!(module_job(None,&json!({"id":"job-1","state":"running"})).unwrap(),ModuleJob::Running("job-1".into()));
    assert_eq!(module_job(Some("job-1"),&json!({"id":"job-1","state":"completed"})).unwrap(),ModuleJob::Completed(json!(null)));
    assert_eq!(module_job(Some("job-1"),&json!({"id":"job-1","state":"cancelled"})).unwrap(),ModuleJob::Cancelled);
    assert!(module_job(None,&json!({"state":"running"})).is_err());
    assert!(module_job(Some("job-1"),&json!({"id":"job-2","state":"completed"})).is_err());
    assert!(module_job(None,&json!({"state":"unknown"})).is_err());
    assert_eq!(module_job(Some("job-1"),&json!({"id":"job-1","state":"failed","error":"launch failed"})).unwrap_err().message,"launch failed");
}

#[test]
fn csharp_module_errors_keep_the_original_message_with_a_null_result(){
    let error=decode_response("capture",br#"{"id":"capture","result":null,"error":{"code":"capture","message":"draft is unavailable"}}"#).unwrap_err();
    assert_eq!(error.code,"capture");
    assert_eq!(error.message,"draft is unavailable");
}
