#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]
use base64::{Engine, engine::general_purpose::STANDARD};
use serde::Deserialize;
use std::io::{self, Read};

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Request { source: String, dark: bool }

fn main() {
    let mut bytes = Vec::new();
    let result = (|| -> Result<Vec<u8>, String> {
        io::stdin().take(70_000).read_to_end(&mut bytes).map_err(|_| "Input could not be read")?;
        let request: Request = serde_json::from_slice(&bytes).map_err(|_| "Invalid diagram request")?;
        winone_diagrams::render_diagram(&request.source, request.dark)
    })();
    // No user diagram text is written to logs or command-line arguments.
    let response = match result {
        Ok(png) => serde_json::json!({"png": STANDARD.encode(png)}),
        Err(error) => serde_json::json!({"error": error}),
    };
    if serde_json::to_writer(io::stdout(), &response).is_err() { std::process::exit(1); }
}
