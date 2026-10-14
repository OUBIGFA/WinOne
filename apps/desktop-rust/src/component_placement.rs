//! Local settings-window geometry. It is deliberately outside portable settings.
use serde::{Deserialize,Serialize};
use std::{fs::OpenOptions,io::Write,os::windows::fs::OpenOptionsExt,path::{Path,PathBuf}};
use windows_sys::Win32::{Foundation::*,Storage::FileSystem::*,UI::WindowsAndMessaging::*};

const FILE:&str="settings-window.json";
#[derive(Clone,Debug,PartialEq,Serialize,Deserialize)]
struct Placement{version:u8,normal:[i32;4],maximized:bool}
impl Placement{
    fn valid(&self)->bool{
        self.version==1&&matches!(self.normal[2].checked_sub(self.normal[0]),Some(1..=32768))
            &&matches!(self.normal[3].checked_sub(self.normal[1]),Some(1..=32768))
    }
}
pub struct Store{directory:PathBuf,saved:Option<Placement>}
impl Store{
    pub fn new(directory:&str)->Self{Self{directory:directory.into(),saved:None}}
    pub fn restore(&mut self,window:HWND)->Result<Option<bool>,String>{
        let Some(value)=crate::windows_files::profile_json(&self.directory,Path::new(FILE),4096).map_err(|e|e.to_string())? else{return Ok(None);};
        let saved:Placement=serde_json::from_value(value).map_err(|e|e.to_string())?;
        if !saved.valid(){return Err("窗口位置记录无效".into());}
        // WINDOWPLACEMENT uses workspace coordinates and lets Windows bring a
        // window back on-screen when a monitor is removed. Keep it hidden until
        // the existing frontend is ready, then restore the saved show state.
        let [left,top,right,bottom]=saved.normal;
        let placement=WINDOWPLACEMENT{length:std::mem::size_of::<WINDOWPLACEMENT>()as u32,showCmd:SW_HIDE as u32,rcNormalPosition:RECT{left,top,right,bottom},..Default::default()};
        if unsafe{SetWindowPlacement(window,&placement)}==0{return Err(std::io::Error::last_os_error().to_string());}
        let maximized=saved.maximized;self.saved=Some(saved);Ok(Some(maximized))
    }
    pub fn save(&mut self,window:HWND)->Result<(),String>{
        let mut placement=WINDOWPLACEMENT{length:std::mem::size_of::<WINDOWPLACEMENT>()as u32,..Default::default()};
        if unsafe{GetWindowPlacement(window,&mut placement)}==0{return Err(std::io::Error::last_os_error().to_string());}
        let r=placement.rcNormalPosition;
        let saved=Placement{version:1,normal:[r.left,r.top,r.right,r.bottom],maximized:unsafe{IsZoomed(window)}!=0||(unsafe{IsIconic(window)}!=0&&placement.flags&WPF_RESTORETOMAXIMIZED!=0)};
        if !saved.valid(){return Err("无法保存无效的窗口位置".into());}
        if self.saved.as_ref()==Some(&saved){return Ok(());}
        let bytes=serde_json::to_vec(&saved).map_err(|e|e.to_string())?;
        write(&self.directory,&bytes)?;self.saved=Some(saved);Ok(())
    }
}

fn write(directory:&Path,bytes:&[u8])->Result<(),String>{
    // Pin and resolve the profile before using paths, so a parent junction
    // cannot redirect this local metadata write. Replace atomically, never
    // truncate the last usable placement if a write is interrupted.
    let root=OpenOptions::new().read(true).share_mode(FILE_SHARE_READ|FILE_SHARE_WRITE).custom_flags(FILE_FLAG_BACKUP_SEMANTICS).open(directory).map_err(|e|e.to_string())?;
    let directory=PathBuf::from(crate::windows_files::final_path(&root).map_err(|e|e.to_string())?);
    let temporary=directory.join(format!(".settings-window-{}.tmp",crate::dialogs::new_program_id()?));
    let result=(||->std::io::Result<()>{
        let mut file=OpenOptions::new().write(true).create_new(true).open(&temporary)?;
        file.write_all(bytes)?;file.sync_all()?;drop(file);
        std::fs::rename(&temporary,directory.join(FILE))
    })();
    if let Err(error)=result{
        if let Err(cleanup)=std::fs::remove_file(&temporary){if cleanup.kind()!=std::io::ErrorKind::NotFound{return Err(format!("{error}；临时记录清理失败：{cleanup}"));}}
        return Err(error.to_string());
    }
    Ok(())
}

#[cfg(test)]
mod tests{
    use super::*;
    #[test]
    fn placement_accepts_negative_monitor_coordinates_but_rejects_invalid_extents(){
        assert!(Placement{version:1,normal:[-1800,-200,-840,440],maximized:true}.valid());
        for normal in [[0,0,0,640],[0,0,960,-1],[i32::MIN,0,i32::MAX,640],[0,0,40000,640]]{
            assert!(!Placement{version:1,normal,maximized:false}.valid());
        }
        assert!(!Placement{version:2,normal:[180,140,1140,780],maximized:false}.valid());
    }
}
