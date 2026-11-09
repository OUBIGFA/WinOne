# WinOne adapter

Complete Chitchatter source from Jeremy Kahn, commit
`708fefdcc3ac0f5fc2ef2ca9ce1cc1b6a9f46c74` (GPL-2.0-or-later).
The original LICENSE and tests are retained. Upstream repository:
https://github.com/jeremyckahn/chitchatter

WinOne bundles this application locally. Its adapters provide native window
lifetime, profile storage, downloads, language, and theme integration. The
upstream room, media, file transfer, settings, and SDK capabilities remain.
Run the repository's `build/build-flashchat.mjs` entry point; npm's original
shell-dependent build scripts are retained only as upstream reference.

Imported dependencies are locked separately from the legacy React settings
window. `build/verify-flashchat-notices.mjs` verifies their original licenses.
Generated bundles and dependencies are ignored by Git.

The browser app keeps the public deployment's room namespace and produces
portable chitchatter.im invitations. File sharing uses the original encrypted
WebTorrent path with the room's RTC and tracker configuration. It no longer uses
the previous WinOne-only file protocol. Original core, adapters, SDK and tests
remain in this directory; native host adapters live under
`apps/desktop-mygo/pages/flashchat`.

The default direct-connectivity configuration includes Google and Cloudflare
STUN. Enhanced connectivity uses the shared public TURN configuration published
by Jami at https://docs.jami.net/en_US/developer/going-further/setting-up-your-own-turn-server.html.
TCP is included because UDP was unavailable in the verification network.
The service is external and has no WinOne availability guarantee. The original
ExpressTURN configuration remains in the imported source for provenance;
it returned 401 during verification. Open Relay was also tested and timed out.
Neither result is represented as a successful cross-network test.

Media adapters report capture failures, release late capture results, retain
the working device when a replacement fails, and synchronize browser track
termination with the controls. Connection labels inspect the selected ICE pair
on both endpoints. Candidate gathering alone never proves peer connectivity.

Validation and the complete capability matrix are maintained in
`docs/mygo-parity/flash-chat.md`. The native fixture uses a private desktop and
repository-only profile; live phone verification is explicitly selected.
