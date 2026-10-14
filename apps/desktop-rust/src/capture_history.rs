//! Reading saved-capture metadata never starts the capture runtime. The worker
//! still validates its current image ID before copying, saving or editing.
use std::path::Path;
use serde_json::{Value,json};
use winone_desktop::protocol::RpcError;

pub fn read(directory:&str)->Result<Value,RpcError> {
    let Some(value)=crate::windows_files::profile_json(Path::new(directory),Path::new("modules/capture/last-capture.json"),64<<10)? else {
        return Ok(json!({"lastCapture":null,"pins":null}));
    };
    if !value["path"].as_str().is_some_and(|s|!s.contains('\0')&&Path::new(s).is_absolute())
        || !value["width"].as_u64().is_some_and(|n|n>0) || !value["height"].as_u64().is_some_and(|n|n>0) {
        return Err(RpcError::new("invalid","最近截图记录无效，请重新截图或检查日志"));
    }
    if value["id"].is_null() || value["id"]=="" {
        // Older metadata receives an ID only in the existing module migration.
        // This exceptional case may start it once; normal reads remain local.
        return Err(RpcError::new("legacy_capture","最近截图记录需要升级"));
    }
    if !value["id"].as_str().is_some_and(|id|id.len()==32&&id.bytes().all(|b|b.is_ascii_digit()||(b'a'..=b'f').contains(&b)))
    {
        return Err(RpcError::new("invalid","最近截图记录无效，请重新截图或检查日志"));
    }
    Ok(json!({"lastCapture":value,"pins":null}))
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn history_reads_saved_identity_and_rejects_invalid_metadata() {
        // Windows reuses process IDs; each run needs a fresh profile instead of
        // reading the invalid/legacy fixture left by an earlier test process.
        let nonce=std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_nanos();
        let root=Path::new(env!("CARGO_MANIFEST_DIR")).join("../../_temp/rust-capture-history-tests").join(format!("{}-{nonce}",std::process::id()));
        std::fs::create_dir_all(root.join("modules/capture")).unwrap();let root=root.canonicalize().unwrap();
        let directory=root.to_str().unwrap();
        assert_eq!(read(directory).unwrap()["lastCapture"],Value::Null);
        let path=root.join("modules/capture/last-capture.json");
        let saved=json!({"id":"a".repeat(32),"path":"C:\\Images\\capture.png","width":200,"height":100});
        std::fs::write(&path,saved.to_string()).unwrap();assert_eq!(read(directory).unwrap()["lastCapture"],saved);
        std::fs::write(&path,"{\"id\":\"../other\"}").unwrap();assert_eq!(read(directory).unwrap_err().code,"invalid");
        std::fs::write(&path,json!({"path":"C:\\Images\\old.png","width":200,"height":100}).to_string()).unwrap();assert_eq!(read(directory).unwrap_err().code,"legacy_capture");
    }
}
