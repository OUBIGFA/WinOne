fn main() {
    println!("cargo:rerun-if-env-changed=WINONE_SELECTION_RESOURCE");
    if let Some(resource) = std::env::var_os("WINONE_SELECTION_RESOURCE") {
        println!("cargo:rerun-if-changed={}", std::path::PathBuf::from(&resource).display());
        println!("cargo:rustc-link-arg-bin=winone-selection-reader={}", std::path::PathBuf::from(resource).display());
    } else if std::env::var("PROFILE").as_deref() == Ok("release") {
        panic!("Use build/build-selection.ps1 to include WinOne resources");
    }
}
