//! Browser screenshots use WebView2's compositor, including page annotations.
use std::{path::PathBuf,rc::Rc};
use serde_json::{Value,json};
use windows::Win32::{System::Com::{STATSTG,STATFLAG_NONAME,STREAM_SEEK_SET},UI::Shell::SHCreateMemStream};
use windows_sys::Win32::{Foundation::{HWND,GlobalFree},System::{Memory::{GlobalAlloc,GlobalLock,GlobalUnlock,GMEM_MOVEABLE},DataExchange::{OpenClipboard,EmptyClipboard,SetClipboardData,CloseClipboard}}};
use wry::{WebView,WebViewExtWindows};
use webview2_com::{CapturePreviewCompletedHandler,Microsoft::Web::WebView2::Win32::COREWEBVIEW2_CAPTURE_PREVIEW_IMAGE_FORMAT_PNG};

pub fn capture(view:&WebView,owner:HWND,path:Option<PathBuf>,complete:Rc<dyn Fn(Result<Value,String>)>)->Result<(),String>{
    let stream=unsafe{SHCreateMemStream(None)}.ok_or("无法创建截图缓存")?;
    let saved=stream.clone();
    let callback=CapturePreviewCompletedHandler::create(Box::new(move|status|{
        let result=(||{
            status?;
            let mut stat=STATSTG::default();unsafe{saved.Stat(&mut stat,STATFLAG_NONAME)?;}
            if stat.cbSize==0||stat.cbSize>64*1024*1024{return Err(windows::core::Error::new(windows::core::HRESULT(0x80070057u32 as i32),"网页截图过大或为空"));}
            let mut data=vec![0u8;stat.cbSize as usize];let mut read=0;
            unsafe{saved.Seek(0,STREAM_SEEK_SET,None)?;saved.Read(data.as_mut_ptr().cast(),data.len()as u32,Some(&mut read)).ok()?;}
            if read as usize!=data.len(){return Err(windows::core::Error::new(windows::core::HRESULT(0x80004005u32 as i32),"网页截图读取不完整"));}
            Ok(data)
        })().map_err(|e:windows::core::Error|e.to_string()).and_then(|data|{
            if let Some(path)=&path{
                std::fs::create_dir_all(path.parent().ok_or("截图目录无效")?).map_err(|e|e.to_string())?;
                std::fs::write(path,&data).map_err(|e|e.to_string())?;
                Ok(json!({"screenshotPath":path,"bytes":data.len()}))
            }else{copy_png(owner,&data)?;Ok(json!({"copied":true}))}
        });
        complete(result);Ok(())
    }));
    unsafe{view.webview().CapturePreview(COREWEBVIEW2_CAPTURE_PREVIEW_IMAGE_FORMAT_PNG,&stream,&callback)}.map_err(|e|e.to_string())
}

fn clipboard_dib(data:&[u8])->Result<Vec<u8>,String>{
    let mut decoder=png::Decoder::new(std::io::Cursor::new(data));decoder.set_transformations(png::Transformations::EXPAND|png::Transformations::STRIP_16);
    let mut reader=decoder.read_info().map_err(|e|e.to_string())?;
    let size=reader.output_buffer_size().filter(|size|*size<=256*1024*1024).ok_or("截图像素过大")?;
    let mut pixels=vec![0;size];let info=reader.next_frame(&mut pixels).map_err(|e|e.to_string())?;
    let count=(info.width as usize).checked_mul(info.height as usize).ok_or("截图尺寸无效")?;
    let mut dib=vec![0u8;124+count*4];
    let put=|dib:&mut Vec<u8>,offset:usize,value:u32|dib[offset..offset+4].copy_from_slice(&value.to_le_bytes());
    put(&mut dib,0,124);put(&mut dib,4,info.width);put(&mut dib,8,(-(info.height as i32))as u32);dib[12..14].copy_from_slice(&1u16.to_le_bytes());dib[14..16].copy_from_slice(&32u16.to_le_bytes());put(&mut dib,16,3);put(&mut dib,20,(count*4)as u32);put(&mut dib,40,0x00ff0000);put(&mut dib,44,0x0000ff00);put(&mut dib,48,0x000000ff);put(&mut dib,52,0xff000000);put(&mut dib,56,0x73524742);
    let channels=match info.color_type{png::ColorType::Rgba=>4,png::ColorType::Rgb=>3,png::ColorType::Grayscale=>1,png::ColorType::GrayscaleAlpha=>2,_=>return Err("不支持的截图像素格式".into())};
    for (source,target) in pixels[..info.buffer_size()].chunks_exact(channels).zip(dib[124..].chunks_exact_mut(4)){
        let(r,g,b,a)=match channels{4=>(source[0],source[1],source[2],source[3]),3=>(source[0],source[1],source[2],255),2=>(source[0],source[0],source[0],source[1]),_=>(source[0],source[0],source[0],255)};target.copy_from_slice(&[b,g,r,a]);
    }Ok(dib)
}
pub(crate) fn copy_png(owner:HWND,data:&[u8])->Result<(),String>{
    copy_png_with_extras(owner,data,None,None)
}

fn unicode_clipboard(text:&str)->Vec<u8>{text.encode_utf16().chain(std::iter::once(0)).flat_map(u16::to_le_bytes).collect()}
fn file_clipboard(path:&str)->Vec<u8>{let mut data=vec![0u8;20];data[0..4].copy_from_slice(&20u32.to_le_bytes());data[16..20].copy_from_slice(&1u32.to_le_bytes());data.extend(unicode_clipboard(path));data.extend([0u8,0]);data}

// Keep bitmap, optional source text and Explorer file-paste semantics together.
// Allocate every format before replacing the clipboard; ownership transfers
// only after each successful SetClipboardData call.
pub(crate) fn copy_png_with_extras(owner:HWND,data:&[u8],plain_text:Option<&str>,file_path:Option<&str>)->Result<(),String>{
    let mut formats=vec![(17,clipboard_dib(data)?)];
    if let Some(text)=plain_text{formats.push((13,unicode_clipboard(text)));}
    if let Some(path)=file_path{formats.push((15,file_clipboard(path)));}
    unsafe{
        let mut handles=Vec::new();
        for (format,bytes) in formats{
            let memory=GlobalAlloc(GMEM_MOVEABLE,bytes.len());
            if memory.is_null(){for (_,owned)in handles{GlobalFree(owned);}return Err(std::io::Error::last_os_error().to_string());}
            let pointer=GlobalLock(memory);if pointer.is_null(){GlobalFree(memory);for (_,owned)in handles{GlobalFree(owned);}return Err(std::io::Error::last_os_error().to_string());}
            std::ptr::copy_nonoverlapping(bytes.as_ptr(),pointer.cast(),bytes.len());GlobalUnlock(memory);handles.push((format,memory));
        }
        if OpenClipboard(owner)==0{for (_,owned)in handles{GlobalFree(owned);}return Err("剪贴板正被其他程序使用，请重试".into());}
        if EmptyClipboard()==0{CloseClipboard();for (_,owned)in handles{GlobalFree(owned);}return Err(std::io::Error::last_os_error().to_string());}
        for index in 0..handles.len(){let(format,memory)=handles[index];if SetClipboardData(format,memory).is_null(){let error=std::io::Error::last_os_error().to_string();for (_,owned)in &handles[index..]{GlobalFree(*owned);}CloseClipboard();return Err(error);}}
        CloseClipboard();
    }Ok(())
}
#[cfg(test)]mod tests{
    use super::*;
    #[test]fn clipboard_text_and_file_formats_keep_unicode_and_terminators(){
        let text=unicode_clipboard("中文 diagram");assert_eq!(&text[text.len()-2..],&[0,0]);
        let file=file_clipboard("C:\\项目\\image.png");assert_eq!(u32::from_le_bytes(file[0..4].try_into().unwrap()),20);assert_eq!(u32::from_le_bytes(file[16..20].try_into().unwrap()),1);assert_eq!(&file[file.len()-4..],&[0,0,0,0]);
    }
    #[test]fn screenshot_clipboard_keeps_straight_alpha_and_uses_top_down_pixels(){
        let mut encoded=Vec::new();{let mut png=png::Encoder::new(&mut encoded,1,1);png.set_color(png::ColorType::Rgba);png.set_depth(png::BitDepth::Eight);png.write_header().unwrap().write_image_data(&[20,40,60,128]).unwrap();}
        let dib=clipboard_dib(&encoded).unwrap();assert_eq!(&dib[124..],[60,40,20,128]);assert_eq!(i32::from_le_bytes(dib[8..12].try_into().unwrap()),-1);assert_eq!(u32::from_le_bytes(dib[52..56].try_into().unwrap()),0xff000000);
    }
}
