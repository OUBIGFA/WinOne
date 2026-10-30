// Declarations the upstream shell normally owns, loaded before the extracted
// app.js closure and the redact plugin page. This document only shows Privacy:
// "Try it" and Insights › Masking, never another application view.
// The authenticated plugin bridge already declares params, mode, view and
// state. Reuse that binding; redeclaring it prevents every later script running.
view = 'masking';
const setShown = 'privacy';
function fit() { window.winOnePublishMasking?.(); }
function show() { return Promise.resolve(false); }
function openMaskRecord(key) { if (key) magpie.call('masking', 'focus', key); }
// The shell's first state read; the plugin page only asks whether it is done.
function load() { return Promise.resolve(); }
load.done = false;
