//! The existing capture UI's image URLs are bound to current saved/draft IDs.
use std::{path::Path,borrow::Cow,io::Cursor};
use serde_json::Value;
use wry::http::Response;
use winone_desktop::protocol::RpcError;

pub fn response(profile:&Path,path:&str)->Response<Cow<'static,[u8]>>{
    match image(profile,path){
        Ok(bytes)=>Response::builder().status(200).header("Content-Type","image/png").header("Cache-Control","private, no-store").header("X-Content-Type-Options","nosniff").body(Cow::Owned(bytes)).unwrap(),
        Err(_)=>Response::builder().status(404).body(Cow::Borrowed(b"Capture image unavailable" as &[u8])).unwrap(),
    }
}
fn invalid()->RpcError{RpcError::new("capture","截图图像不可用")}
fn image(profile:&Path,path:&str)->Result<Vec<u8>,RpcError>{
    let (kind,file)=path.trim_start_matches('/').split_once('/').ok_or_else(invalid)?;
    if !matches!(kind,"capture-preview"|"capture-edit-preview"|"capture-source"){return Err(invalid());}
    let id=file.strip_suffix(".png").filter(|id|crate::component_capture::valid_id(id)).ok_or_else(invalid)?;
    let metadata=|name:&str,limit|crate::windows_files::profile_json(profile,&Path::new("modules/capture").join(name),limit).map(|v|v.unwrap_or(Value::Null));
    let saved=metadata("last-capture.json",256<<10)?;
    let mut draft=metadata("capture-draft.json",768<<10)?;
    let retired=metadata("retired-drafts.json",64<<10)?;
    if retired.as_array().is_some_and(|items|items.len()>64){return Err(invalid());}
    if retired.as_array().is_some_and(|items|items.iter().any(|item|item["id"]==draft["id"]&&!item["completedRevision"].is_null()&&item["completedRevision"]==draft["revision"])){draft=Value::Null;}
    let source=kind=="capture-source";let editing=kind=="capture-edit-preview";
    let allowed=if source{draft["id"]==id&&draft["width"].as_u64().is_some_and(|n|n>0)&&draft["height"].as_u64().is_some_and(|n|n>0)}
        else if editing{let edit=metadata("edit-preview.json",256<<10)?;edit["id"]==id&&edit["sourceId"].as_str().is_some_and(|s|!s.is_empty())&&(edit["sourceId"]==saved["id"]||edit["sourceId"]==draft["id"])}
        else{saved["id"]==id&&saved["previewReady"]==true||draft["previewReady"]==true&&(draft["id"]==id||draft["desktopPreviewId"]==id)};
    if !allowed{return Err(invalid());}
    let relative=Path::new("modules/capture").join(if source{"drafts"}else{"previews"}).join(file);
    let bytes=crate::windows_files::profile_bytes(profile,&relative,if source||editing{512<<20}else{16<<20})?.ok_or_else(invalid)?;
    let decoder=png::Decoder::new(Cursor::new(&bytes)).read_info().map_err(|_|invalid())?;
    let (width,height)=(decoder.info().width,decoder.info().height);
    if source&&(draft["width"]!=width||draft["height"]!=height){return Err(invalid());}
    if source||editing{if width as u64*height as u64>128*1024*1024{return Err(invalid());}}
    else if width>1600||height>1600{return Err(invalid());}
    drop(decoder);Ok(bytes)
}

#[cfg(test)]mod tests{use super::*;
    #[test]fn image_requests_cannot_choose_a_file_path_or_arbitrary_identifier(){
        let root=Path::new(r"C:\missing-profile");
        for path in ["/capture-source/../../secret.png","/capture-preview/abc.png","/other/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png"]{assert!(image(root,path).is_err());}
    }
}
