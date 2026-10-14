//! Build-time sampling of the locally pinned brand SVGs. This tool
//! is not packaged or run by the application; both UIs reuse its small images.
#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]
use resvg::{tiny_skia, usvg};
use std::path::PathBuf;

fn run() -> Result<usize,String> {
    let args:Vec<_>=std::env::args_os().skip(1).collect();
    if args.len()!=2 {return Err("Pass the source and output directories".into());}
    let source=PathBuf::from(&args[0]);let output=PathBuf::from(&args[1]);
    std::fs::create_dir_all(&output).map_err(|e|e.to_string())?;
    let mut paths=std::fs::read_dir(&source).map_err(|e|e.to_string())?.map(|entry|entry.map(|e|e.path())).collect::<Result<Vec<_>,_>>().map_err(|e|e.to_string())?;
    paths.sort();let mut count=0;
    for path in paths {
        if path.extension().is_none_or(|ext|ext!="svg") {continue;}
        let original=std::fs::read(&path).map_err(|e|e.to_string())?;
        // LobeHub's Kimi.Color has a white K and a blue accent. Its original
        // SVG remains pinned unchanged; sample a dark K for transparent light
        // surfaces. Both UIs use this image and derive their dark-mode mask
        // from the unchanged alpha, retaining the official paths and blue dot.
        let svg=if path.file_stem().is_some_and(|name|name=="kimi-color") {
            let text=std::str::from_utf8(&original).map_err(|e|e.to_string())?;
            if text.matches("fill=\"#fff\"").count()!=1 {return Err("Kimi source changed; review its theme palette".into());}
            text.replace("fill=\"#fff\"","fill=\"#1f1f1f\"").into_bytes()
        } else {original};
        if svg.len()>1_048_576 {return Err("Icon source is too large".into());}
        let options=usvg::Options{image_href_resolver:usvg::ImageHrefResolver{resolve_data:Box::new(|_,_,_|None),resolve_string:Box::new(|_,_|None)},..Default::default()};
        let tree=usvg::Tree::from_data(&svg,&options).map_err(|e|e.to_string())?;
        let size=tree.size();let scale=(128.0/size.width()).min(128.0/size.height());
        let transform=tiny_skia::Transform::from_row(scale,0.0,0.0,scale,(128.0-size.width()*scale)/2.0,(128.0-size.height()*scale)/2.0);
        let mut pixels=tiny_skia::Pixmap::new(128,128).ok_or("Icon allocation failed")?;
        resvg::render(&tree,transform,&mut pixels.as_mut());
        if !pixels.pixels().iter().any(|pixel|pixel.alpha()!=0) {return Err(format!("Empty icon: {}",path.display()));}
        let target=output.join(path.file_stem().ok_or("Invalid icon file")?).with_extension("png");
        pixels.save_png(target).map_err(|e|e.to_string())?;count+=1;
    }
    Ok(count)
}
fn main(){match run(){Ok(count)=>println!("Prepared {count} icon images"),Err(error)=>{eprintln!("{error}");std::process::exit(1);}}}
