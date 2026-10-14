//! Handle-based paths shared by diagnostics and installation coordination.
use std::{fs::File, os::windows::io::AsRawHandle};
use winone_desktop::protocol::RpcError;
use windows_sys::Win32::{Foundation::HANDLE, Storage::FileSystem::GetFinalPathNameByHandleW};

pub fn wide(text:&str)->Vec<u16>{text.encode_utf16().chain(Some(0)).collect()}

pub fn final_path(file: &File) -> Result<String, RpcError> {
    let mut path = vec![0u16;32768];
    let length = unsafe { GetFinalPathNameByHandleW(file.as_raw_handle() as HANDLE, path.as_mut_ptr(), path.len() as u32, 0) };
    if length == 0 { return Err(std::io::Error::last_os_error().into()); }
    if length as usize >= path.len() { return Err(RpcError::new("invalid", "路径过长")); }
    String::from_utf16(&path[..length as usize]).map_err(|_| RpcError::new("invalid", "路径无效"))
}

/// Read bounded metadata from an opened profile, without following a junction
/// outside it. Missing metadata is an empty state; other errors remain visible.
pub fn profile_json(profile: &std::path::Path, relative: &std::path::Path, limit: usize) -> Result<Option<serde_json::Value>,RpcError> {
    profile_bytes(profile,relative,limit)?.map(|bytes|serde_json::from_slice(&bytes).map_err(RpcError::from)).transpose()
}

pub fn profile_bytes(profile: &std::path::Path, relative: &std::path::Path, limit: usize) -> Result<Option<Vec<u8>>,RpcError> {
    use std::{fs::OpenOptions,io::Read,os::windows::fs::OpenOptionsExt};
    use windows_sys::Win32::Storage::FileSystem::*;
    if !profile.is_absolute() || relative.is_absolute() {return Err(RpcError::new("invalid","配置记录路径无效"));}
    let root=match OpenOptions::new().read(true).share_mode(FILE_SHARE_READ|FILE_SHARE_WRITE).custom_flags(FILE_FLAG_BACKUP_SEMANTICS).open(profile) {
        Ok(root)=>root,Err(error) if error.kind()==std::io::ErrorKind::NotFound=>return Ok(None),Err(error)=>return Err(error.into()),
    };
    if !root.metadata()?.is_dir() {return Err(RpcError::new("invalid","配置记录目录无效"));}
    let file=match OpenOptions::new().read(true).share_mode(FILE_SHARE_READ|FILE_SHARE_WRITE|FILE_SHARE_DELETE).open(profile.join(relative)) {
        Ok(file)=>file,Err(error) if error.kind()==std::io::ErrorKind::NotFound=>return Ok(None),Err(error)=>return Err(error.into()),
    };
    let base=final_path(&root)?.trim_end_matches('\\').to_lowercase()+"\\";
    if !final_path(&file)?.to_lowercase().starts_with(&base) {return Err(RpcError::new("invalid","配置记录超出当前目录"));}
    if !file.metadata()?.is_file() || file.metadata()?.len()>limit as u64 {return Err(RpcError::new("invalid","配置记录大小或类型无效"));}
    let mut bytes=Vec::new();file.take(limit as u64+1).read_to_end(&mut bytes)?;
    if bytes.len()>limit {return Err(RpcError::new("invalid","配置记录过大"));}
    Ok(Some(bytes))
}
