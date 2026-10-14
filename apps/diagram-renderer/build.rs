fn main() {
    println!("cargo:rerun-if-env-changed=WINONE_DIAGRAM_RESOURCE");
    if let Some(resource)=std::env::var_os("WINONE_DIAGRAM_RESOURCE") {
        println!("cargo:rerun-if-changed={}",std::path::PathBuf::from(&resource).display());
        println!("cargo:rustc-link-arg-bin=winone-diagrams={}",std::path::PathBuf::from(resource).display());
    } else if std::env::var("PROFILE").as_deref()==Ok("release") {
        panic!("Build with build/build-diagrams.ps1 to include the WinOne manifest and icon");
    }
}
