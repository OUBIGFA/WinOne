//! File metadata uses the existing icon extraction/cache; the original UI
//! expects a PNG, not the native prototype's raw BGRA pixels.
use serde_json::{Value,json};
use winone_desktop::protocol::RpcError;
use windows_sys::Win32::Storage::FileSystem::*;
use std::{path::Path,sync::Mutex};

pub fn read(directory:&str,path:&str)->Result<Value,RpcError>{
    static SHELL:Mutex<()>=Mutex::new(());
    let _shell=SHELL.lock().map_err(|_|RpcError::new("icon","无法读取程序图标"))?;
    if !std::fs::metadata(path)?.is_file(){return Err(RpcError::new("invalid","请选择程序文件"));}
    let fallback=Path::new(path).file_stem().unwrap_or_default().to_string_lossy().into_owned();
    let name=description(path).unwrap_or(fallback);
    let cache=Path::new(directory).join("cache/desktop-rust/program-icons");
    let icon=crate::icon::load(&cache.to_string_lossy(),path).and_then(|s|crate::icon::base64_decode(&s))
        .map(|pixels|encode_png(&pixels)).transpose()?;
    let mut value=json!({"name":name});if let Some(icon)=icon{value["icon"]=json!(crate::icon::base64_encode(&icon));}Ok(value)
}
fn encode_png(pixels:&[u8])->Result<Vec<u8>,RpcError>{
    if pixels.len()!=32*32*4{return Err(RpcError::new("icon","程序图标数据无效"));}
    let mut rgba=pixels.to_vec();for pixel in rgba.chunks_exact_mut(4){pixel.swap(0,2);}
    let mut output=Vec::new();
    {let mut encoder=png::Encoder::new(&mut output,32,32);encoder.set_color(png::ColorType::Rgba);encoder.set_depth(png::BitDepth::Eight);
        let mut writer=encoder.write_header().map_err(|e|RpcError::new("icon",e.to_string()))?;
        writer.write_image_data(&rgba).map_err(|e|RpcError::new("icon",e.to_string()))?;
    }Ok(output)
}
fn description(path:&str)->Option<String>{unsafe{
    let path=crate::windows_files::wide(path);let size=GetFileVersionInfoSizeW(path.as_ptr(),std::ptr::null_mut());
    if size==0||size>4<<20{return None;}
    let mut data=vec![0u8;size as usize];if GetFileVersionInfoW(path.as_ptr(),0,size,data.as_mut_ptr().cast())==0{return None;}
    let mut pairs=std::ptr::null_mut();let mut length=0;
    if VerQueryValueW(data.as_ptr().cast(),crate::windows_files::wide(r"\VarFileInfo\Translation").as_ptr(),&mut pairs,&mut length)==0||pairs.is_null()||length<4||length>size{return None;}
    for field in ["FileDescription","ProductName"]{for pair in std::slice::from_raw_parts(pairs.cast::<u16>(),length as usize/2).chunks_exact(2){
        let key=crate::windows_files::wide(&format!(r"\StringFileInfo\{:04x}{:04x}\{field}",pair[0],pair[1]));
        let mut text=std::ptr::null_mut();let mut count=0;
        if VerQueryValueW(data.as_ptr().cast(),key.as_ptr(),&mut text,&mut count)!=0&&!text.is_null()&&count>0&&count<4096{
            let value=String::from_utf16_lossy(std::slice::from_raw_parts(text.cast::<u16>(),count as usize)).trim_matches('\0').trim().to_owned();
            if !value.is_empty(){return Some(value);}
        }
    }}None
}}
#[cfg(test)]mod tests{use super::*;
    #[test]fn browser_icon_is_png_with_straight_alpha_and_correct_channels(){
        let encoded=encode_png(&[11,22,33,127].repeat(32*32)).unwrap();
        let mut decoder=png::Decoder::new(std::io::Cursor::new(encoded)).read_info().unwrap();let mut bytes=vec![0;decoder.output_buffer_size().unwrap()];
        let info=decoder.next_frame(&mut bytes).unwrap();assert_eq!((info.width,info.height),(32,32));assert_eq!(&bytes[..4],&[33,22,11,127]);
    }
}
