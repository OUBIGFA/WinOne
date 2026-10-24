# croc in WinOne

Source: https://github.com/schollz/croc/tree/8a806c36956533f9c003c55f16e31fcf14133f8e

Version: v11.5.5. License: MIT; upstream notices and module locks are retained.
WinOne directly calls the original croc and storeclient packages. Cryptography,
relay selection, transfer framing, reconnect, filesystem checks and stored
transfer verification remain upstream implementations.

Host adaptations:

- src/croc/winone_progress.go: read-only snapshot using existing locks.
- src/utils/utils.go: optional context-aware input provider for native prompts.
  RandomFileName also returns a basename on Windows, preserving the upstream
  text-artifact exception when the receiver selects the rename policy.
- modules/interconnect/transfer: one transfer per private stdio process; explicit
  task/output paths, cancellation and bounded in-memory terminal diagnostics.

The store protocol accepts regular files. Hosted transfers are opt-in and have
finite expiration/download limits. They do not imply sender-online P2P sharing.
The standalone SSH server, relay deployment and web hosting applications are
outside the requested file-transfer scope and are not launched by the adapter.
