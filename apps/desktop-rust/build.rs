use std::{env,fs,path::{Path,PathBuf}};

fn main(){
    println!("cargo:rerun-if-env-changed=WINONE_RUST_RESOURCE");
    if let Some(resource)=env::var_os("WINONE_RUST_RESOURCE"){
        println!("cargo:rerun-if-changed={}",PathBuf::from(&resource).display());
        println!("cargo:rustc-link-arg-bin=winone-desktop={}",PathBuf::from(resource).display());
    }else if env::var("PROFILE").as_deref()==Ok("release"){
        panic!("Build releases through build/build-desktop-rust.ps1 to include the project icon and Windows manifest");
    }
    println!("cargo:rerun-if-env-changed=CARGO_FEATURE_COMPONENT_UI");
    if env::var_os("CARGO_FEATURE_COMPONENT_UI").is_none(){return;}
    let root=PathBuf::from(env::var_os("CARGO_MANIFEST_DIR").unwrap()).join("../desktop/frontend/dist");
    assert!(root.join("index.html").is_file(),"Build the current frontend with the workspace build script before building the component UI");
    println!("cargo:rerun-if-changed={}",root.display());
    let mut files=Vec::new();collect(&root,&root,&mut files);files.sort();
    let mut source=String::from("pub static FILES: &[(&str,&[u8])]=&[\n");
    for (name,path) in files{source.push_str(&format!("({name:?},include_bytes!({:?})),\n",path.to_string_lossy()));}
    source.push_str("];\n");
    fs::write(PathBuf::from(env::var_os("OUT_DIR").unwrap()).join("component_assets.rs"),source).unwrap();
}
fn collect(root:&Path,directory:&Path,files:&mut Vec<(String,PathBuf)>){
    for entry in fs::read_dir(directory).unwrap(){let path=entry.unwrap().path();
        if path.is_dir(){collect(root,&path,files);}else if path.is_file(){
            files.push((format!("/{}",path.strip_prefix(root).unwrap().to_string_lossy().replace('\\',"/")),path.canonicalize().unwrap()));
        }
    }
}
