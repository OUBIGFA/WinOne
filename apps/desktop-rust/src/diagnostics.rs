//! Bounded, local diagnostics remain available without a working Host. Only
//! known logs are readable; opened handles are checked against the selected
//! profile so junctions and symlinks cannot expose another directory.
use serde_json::{Value, json};
use std::{fs::{File, OpenOptions}, io::{Read, Seek, SeekFrom}, os::windows::fs::OpenOptionsExt, path::Path};
use crate::windows_files::final_path;
use winone_desktop::protocol::RpcError;
use windows_sys::Win32::Storage::FileSystem::*;

const LIMIT: usize = 64 << 10;

fn files(module: &str) -> Result<(&str, &'static [(&'static str, &'static str)]), RpcError> {
    match module {
        // Older module IDs remain aliases; all current UI uses the host entry.
        "host" | "capture" | "always-on-top" | "wintray" | "selection" => Ok(("logs", &[("process","host.log")])),
        _ => Err(RpcError::new("invalid", "日志模块无效")),
    }
}

fn open(path: &Path, directory: bool) -> Result<File, std::io::Error> {
    // Keep directories pinned while checking their children. Do not request
    // delete sharing, so a concurrent rename cannot redirect the profile root.
    OpenOptions::new().read(true).share_mode(FILE_SHARE_READ | FILE_SHARE_WRITE)
        .custom_flags(if directory { FILE_FLAG_BACKUP_SEMANTICS } else { 0 }).open(path)
}

fn within(root: &File, path: &Path, directory: bool) -> Result<File, RpcError> {
    let file = open(path, directory)?;
    let root_path = final_path(root)?.trim_end_matches('\\').to_lowercase() + "\\";
    if !final_path(&file)?.to_lowercase().starts_with(&root_path) { return Err(RpcError::new("invalid", "日志路径超出配置目录")); }
    let metadata = file.metadata()?;
    if (directory && !metadata.is_dir()) || (!directory && !metadata.is_file()) { return Err(RpcError::new("invalid", "日志路径类型无效")); }
    Ok(file)
}

fn root(directory: &str) -> Result<File, RpcError> {
    if !Path::new(directory).is_absolute() { return Err(RpcError::new("invalid", "日志目录必须是绝对路径")); }
    let file = open(Path::new(directory), true)?;
    if !file.metadata()?.is_dir() { return Err(RpcError::new("invalid", "配置目录类型无效")); }
    Ok(file)
}

pub fn read(directory: &str, module: &str) -> Result<Value, RpcError> {
    let (relative, specs) = files(module)?;
    let mut result = json!({"folderAvailable":false,"entries":specs.iter().map(|(name,_)| json!({"name":name,"available":false,"truncated":false,"text":""})).collect::<Vec<_>>()});
    // Absence is an empty state; access errors and containment failures remain
    // visible errors, rather than being mistaken for a lack of logs.
    if Path::new(directory).is_absolute() && !Path::new(directory).try_exists()? { return Ok(result); }
    let root = root(directory)?;
    let folder_path = Path::new(directory).join(relative);
    if !folder_path.try_exists()? { return Ok(result); }
    let _folder = within(&root, &folder_path, true)?;
    result["folderAvailable"] = json!(true);
    for (index, (_, name)) in specs.iter().enumerate() {
        let path = folder_path.join(name);
        if !path.try_exists()? { continue; }
        let mut file = within(&root, &path, false)?;
        let size = file.metadata()?.len();
        let offset = size.saturating_sub(LIMIT as u64);
        file.seek(SeekFrom::Start(offset))?;
        let mut bytes = Vec::with_capacity(LIMIT);
        file.take(LIMIT as u64).read_to_end(&mut bytes)?;
        let start = if offset > 0 { bytes.iter().position(|byte| byte & 0xc0 != 0x80).unwrap_or(bytes.len()) } else { 0 };
        result["entries"][index]["available"] = json!(true);
        result["entries"][index]["truncated"] = json!(offset > 0);
        result["entries"][index]["text"] = json!(log_text(&bytes[start..]));
    }
    Ok(result)
}

// Match bytes.ToValidUTF8 in the Host: replace each consecutive invalid byte
// sequence with one '?', while preserving a valid literal replacement glyph.
fn log_text(mut bytes: &[u8]) -> String {
    let mut text = String::new();
    let mut invalid_run = false;
    while !bytes.is_empty() {
        match std::str::from_utf8(bytes) {
            Ok(valid) => { text.push_str(valid); break; }
            Err(error) => {
                let valid = error.valid_up_to();
                if valid > 0 {
                    text.push_str(std::str::from_utf8(&bytes[..valid]).expect("validated UTF-8 prefix"));
                    invalid_run = false;
                }
                if !invalid_run { text.push('?'); }
                invalid_run = true;
                match error.error_len() {
                    Some(length) => bytes = &bytes[valid+length..],
                    None => break,
                }
            }
        }
    }
    text
}

pub fn folder(directory: &str, module: &str) -> Result<Value, RpcError> {
    let (relative, _) = files(module)?;
    let root = root(directory)?;
    let file = within(&root, &Path::new(directory).join(relative), true)?;
    let path = final_path(&file)?;
    let path = if let Some(unc) = path.strip_prefix(r"\\?\UNC\") { format!(r"\\{unc}") }
        else { path.strip_prefix(r"\\?\").unwrap_or(&path).into() };
    Ok(json!({"path":path}))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::{fs, path::PathBuf, sync::atomic::{AtomicU64, Ordering}};
    fn fixture() -> PathBuf {
        static SEQUENCE: AtomicU64 = AtomicU64::new(0);
        let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../_temp/rust-local-diagnostics-tests")
            .join(format!("{}-{}",std::process::id(),SEQUENCE.fetch_add(1,Ordering::Relaxed)));
        fs::create_dir_all(&root).unwrap(); root.canonicalize().unwrap()
    }
    #[test]
    fn local_logs_preserve_unicode_empty_states_and_the_read_limit() {
        let root = fixture(); let directory = root.to_str().unwrap();
        assert_eq!(read(directory,"wintray").unwrap()["folderAvailable"], false);
        fs::create_dir_all(root.join("logs")).unwrap();
        fs::write(root.join("logs/host.log"), "日志".repeat(20000)).unwrap();
        let logs = read(directory,"capture").unwrap();
        assert_eq!(logs["folderAvailable"], true);
        assert_eq!(logs["entries"][0]["truncated"], true);
        let text = logs["entries"][0]["text"].as_str().unwrap();
        assert!(text.len() <= LIMIT && text.ends_with("日志") && !text.contains('\u{fffd}'));
        assert!(folder(directory,"capture").unwrap()["path"].as_str().unwrap().ends_with("logs"));
        assert!(read(directory,"../private").is_err());
        assert!(read("relative-profile","host").is_err());
    }
    #[test]
    fn a_directory_cannot_be_read_as_a_log() {
        let root = fixture(); fs::create_dir_all(root.join("logs/host.log")).unwrap();
        assert!(read(root.to_str().unwrap(),"host").is_err());
    }
    #[test]
    fn opened_log_handles_outside_the_profile_are_rejected() {
        let directory = fixture(); let outside = fixture();
        fs::write(outside.join("worker.log"), "private").unwrap();
        let root = root(directory.to_str().unwrap()).unwrap();
        assert!(within(&root,&outside.join("worker.log"),false).is_err());
    }
    #[test]
    fn invalid_utf8_runs_match_the_host_without_replacing_valid_glyphs() {
        assert_eq!(log_text(b"a\xff\xffb\xfe"),"a?b?");
        assert_eq!(log_text("日志\u{fffd}".as_bytes()),"日志\u{fffd}");
        assert_eq!(log_text(&[0xe6,0x97]),"?");
    }
}
