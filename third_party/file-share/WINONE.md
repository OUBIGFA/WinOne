# FileShare in WinOne

Source: https://github.com/tri5m/file-share/tree/7228df100f687cd33332331e9c590606623408e4
Version: 1.0.13. License: MIT (original LICENSE retained).

The Rust HTTP engine, tests, localization, network discovery and browser assets
are copied from upstream. The Tauri desktop shell and its updater are replaced
by WinOne management and lifecycle adapters; no transfer protocol is rewritten.

WinOne boundary changes: explicit isolated data/download directories, selected
IPv4 listener, capability-protected browser access, same-origin requests, native
management snapshots, and bounded shutdown that waits for the server task.
The full browser assets remain available as upstream reference; admin commands
are never exposed on the network listener.

Dependencies are locked in src-tauri/Cargo.lock. Original dependency notices
are recorded in third_party/interconnect-lan-notices.json. See
docs/互联助手调研与设计.md for verification status; intake is not full delivery.
